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
	awsshield "github.com/aws/aws-sdk-go-v2/service/shield"
	shieldtypes "github.com/aws/aws-sdk-go-v2/service/shield/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
	shieldhelper "github.com/konfig-io/konfig-konector/internal/aws/shield"
)

// ShieldProtectionReconciler reconciles ShieldProtection objects.
type ShieldProtectionReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	ShieldClient *multi.Shield
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=shieldprotections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=shieldprotections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=shieldprotections/finalizers,verbs=update

func (r *ShieldProtectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.ShieldProtection{}
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
			if err := r.deleteProtection(ctx, obj); err != nil {
				logger.Error(err, "failed to delete ShieldProtection")
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
	}

	if err := r.reconcileProtection(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionSP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ShieldProtectionReconciler) reconcileProtection(ctx context.Context, obj *awsv1alpha1.ShieldProtection) error {
	// If already created, describe to verify it still exists.
	if obj.Status.ProtectionID != "" {
		descOut, err := r.ShieldClient.DescribeProtection(ctx, &awsshield.DescribeProtectionInput{
			ProtectionId: aws.String(obj.Status.ProtectionID),
		})
		if err != nil && !shieldhelper.IsNotFound(err) {
			return fmt.Errorf("describe shield protection: %w", err)
		}
		if err == nil && descOut.Protection != nil {
			obj.Status.ProtectionARN = aws.ToString(descOut.Protection.ProtectionArn)
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionSP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ShieldProtection reconciled")
		}
		obj.Status.ProtectionID = ""
		obj.Status.ProtectionARN = ""
	}

	input := &awsshield.CreateProtectionInput{
		Name:        aws.String(obj.Spec.Name),
		ResourceArn: aws.String(obj.Spec.ResourceARN),
	}
	if len(obj.Spec.Tags) > 0 {
		tags := make([]shieldtypes.Tag, 0, len(obj.Spec.Tags))
		for k, v := range obj.Spec.Tags {
			k, v := k, v
			tags = append(tags, shieldtypes.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.ShieldClient.CreateProtection(ctx, input)
	if err != nil {
		return fmt.Errorf("create shield protection: %w", err)
	}

	obj.Status.ProtectionID = aws.ToString(out.ProtectionId)
	// The AWS resource now exists; losing the ID would orphan it.
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist status after create: %w", err)
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionSP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "ShieldProtection created")
}

func (r *ShieldProtectionReconciler) deleteProtection(ctx context.Context, obj *awsv1alpha1.ShieldProtection) error {
	if obj.Status.ProtectionID == "" {
		// Status may have been lost after a successful create; the create
		// path always uses spec.name and spec.resourceARN, so look the
		// protection up by both before giving up.
		listOut, err := r.ShieldClient.ListProtections(ctx, &awsshield.ListProtectionsInput{
			InclusionFilters: &shieldtypes.InclusionProtectionFilters{
				ProtectionNames: []string{obj.Spec.Name},
			},
		})
		if err != nil && !shieldhelper.IsNotFound(err) {
			return fmt.Errorf("list shield protections: %w", err)
		}
		if err == nil {
			for _, p := range listOut.Protections {
				if aws.ToString(p.Name) == obj.Spec.Name && aws.ToString(p.ResourceArn) == obj.Spec.ResourceARN {
					obj.Status.ProtectionID = aws.ToString(p.Id)
					break
				}
			}
		}
		if obj.Status.ProtectionID == "" {
			return nil
		}
	}
	_, err := r.ShieldClient.DeleteProtection(ctx, &awsshield.DeleteProtectionInput{
		ProtectionId: aws.String(obj.Status.ProtectionID),
	})
	if shieldhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ShieldProtectionReconciler) setConditionSP(ctx context.Context, obj *awsv1alpha1.ShieldProtection, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *ShieldProtectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ShieldProtection{}).
		Complete(r)
}
