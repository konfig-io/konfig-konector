/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awskms "github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	kmshelper "github.com/konfig-io/konfig-konector/internal/aws/kms"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// KMSGrantReconciler reconciles KMSGrant objects.
type KMSGrantReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	KMSClient *multi.KMS
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=kmsgrants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=kmsgrants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=kmsgrants/finalizers,verbs=update

func (r *KMSGrantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.KMSGrant{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, obj); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !obj.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
			if shouldAbandon(obj) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, obj)
			}
			if err := r.deleteGrant(ctx, obj); err != nil {
				logger.Error(err, "failed to delete KMSGrant")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, obj)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(obj, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileGrant(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionKG(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *KMSGrantReconciler) reconcileGrant(ctx context.Context, obj *awsv1alpha1.KMSGrant) error {
	// KMS grants are immutable (except retire/revoke). Only create if not already created.
	if obj.Status.GrantID != "" {
		obj.Status.ObservedGeneration = obj.Generation
		now := metav1.Now()
		obj.Status.LastSyncTime = &now
		return r.setConditionKG(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "KMSGrant already exists")
	}

	ops := make([]kmstypes.GrantOperation, 0, len(obj.Spec.Operations))
	for _, op := range obj.Spec.Operations {
		ops = append(ops, kmstypes.GrantOperation(op))
	}

	createInput := &awskms.CreateGrantInput{
		KeyId:            aws.String(obj.Spec.KeyID),
		GranteePrincipal: aws.String(obj.Spec.GranteePrincipalARN),
		Operations:       ops,
	}
	if obj.Spec.Name != "" {
		createInput.Name = aws.String(obj.Spec.Name)
	}
	if obj.Spec.RetiringPrincipalARN != "" {
		createInput.RetiringPrincipal = aws.String(obj.Spec.RetiringPrincipalARN)
	}

	out, err := r.KMSClient.CreateGrant(ctx, createInput)
	if err != nil {
		return fmt.Errorf("create kms grant: %w", err)
	}

	obj.Status.GrantID = aws.ToString(out.GrantId)
	obj.Status.GrantToken = aws.ToString(out.GrantToken)
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionKG(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "KMSGrant created")
}

func (r *KMSGrantReconciler) deleteGrant(ctx context.Context, obj *awsv1alpha1.KMSGrant) error {
	if obj.Status.GrantID == "" {
		return nil
	}
	_, err := r.KMSClient.RevokeGrant(ctx, &awskms.RevokeGrantInput{
		KeyId:   aws.String(obj.Spec.KeyID),
		GrantId: aws.String(obj.Status.GrantID),
	})
	if kmshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *KMSGrantReconciler) setConditionKG(ctx context.Context, obj *awsv1alpha1.KMSGrant, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: obj.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *KMSGrantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.KMSGrant{}).
		Complete(r)
}
