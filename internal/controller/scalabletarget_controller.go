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
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsaas "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	aastypes "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	aashelper "github.com/konfig-io/konfig-konector/internal/aws/applicationautoscaling"
)

// ScalableTargetAWSAPI is the subset of the Application Auto Scaling API used
// by this controller.
type ScalableTargetAWSAPI interface {
	RegisterScalableTarget(ctx context.Context, params *awsaas.RegisterScalableTargetInput, optFns ...func(*awsaas.Options)) (*awsaas.RegisterScalableTargetOutput, error)
	DeregisterScalableTarget(ctx context.Context, params *awsaas.DeregisterScalableTargetInput, optFns ...func(*awsaas.Options)) (*awsaas.DeregisterScalableTargetOutput, error)
}

// ScalableTargetReconciler reconciles ScalableTarget objects.
type ScalableTargetReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	AASClient ScalableTargetAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=scalabletargets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=scalabletargets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=scalabletargets/finalizers,verbs=update

func (r *ScalableTargetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	st := &awsv1alpha1.ScalableTarget{}
	if err := r.Get(ctx, req.NamespacedName, st); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, st); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !st.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(st, awsv1alpha1.FinalizerName) {
			if shouldAbandon(st) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(st, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, st)
			}
			if err := r.deleteTarget(ctx, st); err != nil {
				logger.Error(err, "failed to deregister scalable target")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(st, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, st)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(st, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(st, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, st); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileTarget(ctx, st); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, st, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ScalableTargetReconciler) reconcileTarget(ctx context.Context, st *awsv1alpha1.ScalableTarget) error {
	// RegisterScalableTarget is an idempotent upsert: it creates or updates
	// min/max capacity, so no separate create/update paths are needed.
	input := &awsaas.RegisterScalableTargetInput{
		ServiceNamespace:  aastypes.ServiceNamespace(st.Spec.ServiceNamespace),
		ResourceId:        aws.String(st.Spec.ResourceID),
		ScalableDimension: aastypes.ScalableDimension(st.Spec.ScalableDimension),
		MinCapacity:       aws.Int32(st.Spec.MinCapacity),
		MaxCapacity:       aws.Int32(st.Spec.MaxCapacity),
	}
	if st.Spec.RoleARN != "" {
		input.RoleARN = aws.String(st.Spec.RoleARN)
	}
	if len(st.Spec.Tags) > 0 {
		input.Tags = st.Spec.Tags
	}
	out, err := r.AASClient.RegisterScalableTarget(ctx, input)
	if err != nil {
		return fmt.Errorf("register scalable target: %w", err)
	}
	st.Status.ScalableTargetARN = aws.ToString(out.ScalableTargetARN)
	// Persist the identifier immediately: the AWS resource now exists and
	// losing the identifier would orphan it on delete.
	if err := persistStatus(ctx, r.Client, st); err != nil {
		return fmt.Errorf("persist scalable target ARN after register: %w", err)
	}

	st.Status.ObservedGeneration = st.Generation
	now := metav1.Now()
	st.Status.LastSyncTime = &now
	return r.setCondition(ctx, st, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "scalable target reconciled")
}

func (r *ScalableTargetReconciler) deleteTarget(ctx context.Context, st *awsv1alpha1.ScalableTarget) error {
	// The target is fully identified by spec fields; no status identifier needed.
	_, err := r.AASClient.DeregisterScalableTarget(ctx, &awsaas.DeregisterScalableTargetInput{
		ServiceNamespace:  aastypes.ServiceNamespace(st.Spec.ServiceNamespace),
		ResourceId:        aws.String(st.Spec.ResourceID),
		ScalableDimension: aastypes.ScalableDimension(st.Spec.ScalableDimension),
	})
	if aashelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ScalableTargetReconciler) setCondition(ctx context.Context, st *awsv1alpha1.ScalableTarget, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&st.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: st.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, st); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *ScalableTargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ScalableTarget{}).
		Complete(r)
}
