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
	awssesv2 "github.com/aws/aws-sdk-go-v2/service/sesv2"
	sestypes "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
	seshelper "github.com/konfig-io/konfig-konector/internal/aws/sesv2"
)

// SESEmailIdentityReconciler reconciles SESEmailIdentity objects.
type SESEmailIdentityReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	SESv2Client *multi.SESv2
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=sesemailidentities,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=sesemailidentities/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=sesemailidentities/finalizers,verbs=update

func (r *SESEmailIdentityReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.SESEmailIdentity{}
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
			if err := r.deleteEmailIdentity(ctx, obj); err != nil {
				logger.Error(err, "failed to delete SESEmailIdentity")
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

	if err := r.reconcileEmailIdentity(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionSEI(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *SESEmailIdentityReconciler) reconcileEmailIdentity(ctx context.Context, obj *awsv1alpha1.SESEmailIdentity) error {
	existing, err := r.SESv2Client.GetEmailIdentity(ctx, &awssesv2.GetEmailIdentityInput{
		EmailIdentity: aws.String(obj.Spec.EmailIdentity),
	})
	if err != nil && !seshelper.IsNotFound(err) {
		return fmt.Errorf("get ses email identity: %w", err)
	}

	if err == nil && existing != nil {
		obj.Status.IdentityType = string(existing.IdentityType)
		obj.Status.VerifiedForSendingStatus = existing.VerifiedForSendingStatus
	} else {
		tags := make([]sestypes.Tag, 0, len(obj.Spec.Tags))
		for k, v := range obj.Spec.Tags {
			k, v := k, v
			tags = append(tags, sestypes.Tag{Key: &k, Value: &v})
		}

		createInput := &awssesv2.CreateEmailIdentityInput{
			EmailIdentity: aws.String(obj.Spec.EmailIdentity),
		}
		if obj.Spec.ConfigurationSetName != "" {
			createInput.ConfigurationSetName = aws.String(obj.Spec.ConfigurationSetName)
		}
		if len(tags) > 0 {
			createInput.Tags = tags
		}
		out, err := r.SESv2Client.CreateEmailIdentity(ctx, createInput)
		if err != nil {
			return fmt.Errorf("create ses email identity: %w", err)
		}
		obj.Status.IdentityType = string(out.IdentityType)
		obj.Status.VerifiedForSendingStatus = out.VerifiedForSendingStatus
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionSEI(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "SESEmailIdentity reconciled")
}

func (r *SESEmailIdentityReconciler) deleteEmailIdentity(ctx context.Context, obj *awsv1alpha1.SESEmailIdentity) error {
	_, err := r.SESv2Client.DeleteEmailIdentity(ctx, &awssesv2.DeleteEmailIdentityInput{
		EmailIdentity: aws.String(obj.Spec.EmailIdentity),
	})
	if seshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SESEmailIdentityReconciler) setConditionSEI(ctx context.Context, obj *awsv1alpha1.SESEmailIdentity, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *SESEmailIdentityReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.SESEmailIdentity{}).
		Complete(r)
}
