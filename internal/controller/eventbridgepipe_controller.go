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
	awspipes "github.com/aws/aws-sdk-go-v2/service/pipes"
	pipetypes "github.com/aws/aws-sdk-go-v2/service/pipes/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
	pipeshelper "github.com/konfig-io/konfig-konector/internal/aws/pipes"
)

// EventBridgePipeReconciler reconciles EventBridgePipe objects.
type EventBridgePipeReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	PipesClient *multi.Pipes
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=eventbridgepipes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eventbridgepipes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eventbridgepipes/finalizers,verbs=update

func (r *EventBridgePipeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.EventBridgePipe{}
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
			if err := r.deletePipe(ctx, obj); err != nil {
				logger.Error(err, "failed to delete EventBridgePipe")
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

	if err := r.reconcilePipe(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionEBP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *EventBridgePipeReconciler) reconcilePipe(ctx context.Context, obj *awsv1alpha1.EventBridgePipe) error {
	existing, err := r.PipesClient.DescribePipe(ctx, &awspipes.DescribePipeInput{
		Name: aws.String(obj.Spec.Name),
	})
	if err != nil && !pipeshelper.IsNotFound(err) {
		return fmt.Errorf("describe eventbridge pipe: %w", err)
	}

	if err == nil && existing != nil {
		obj.Status.PipeARN = aws.ToString(existing.Arn)
		obj.Status.PipeState = string(existing.CurrentState)

		updateInput := &awspipes.UpdatePipeInput{
			Name:    aws.String(obj.Spec.Name),
			RoleArn: aws.String(obj.Spec.RoleARN),
		}
		if obj.Spec.Description != "" {
			updateInput.Description = aws.String(obj.Spec.Description)
		}
		if obj.Spec.DesiredState != "" {
			updateInput.DesiredState = pipetypes.RequestedPipeState(obj.Spec.DesiredState)
		}
		if obj.Spec.Enrichment != "" {
			updateInput.Enrichment = aws.String(obj.Spec.Enrichment)
		}
		out, err := r.PipesClient.UpdatePipe(ctx, updateInput)
		if err != nil {
			return fmt.Errorf("update eventbridge pipe: %w", err)
		}
		obj.Status.PipeARN = aws.ToString(out.Arn)
		obj.Status.PipeState = string(out.CurrentState)
	} else {
		createInput := &awspipes.CreatePipeInput{
			Name:    aws.String(obj.Spec.Name),
			RoleArn: aws.String(obj.Spec.RoleARN),
			Source:  aws.String(obj.Spec.Source),
			Target:  aws.String(obj.Spec.Target),
		}
		if obj.Spec.Description != "" {
			createInput.Description = aws.String(obj.Spec.Description)
		}
		if obj.Spec.DesiredState != "" {
			createInput.DesiredState = pipetypes.RequestedPipeState(obj.Spec.DesiredState)
		}
		if obj.Spec.Enrichment != "" {
			createInput.Enrichment = aws.String(obj.Spec.Enrichment)
		}
		if len(obj.Spec.Tags) > 0 {
			createInput.Tags = obj.Spec.Tags
		}
		out, err := r.PipesClient.CreatePipe(ctx, createInput)
		if err != nil {
			return fmt.Errorf("create eventbridge pipe: %w", err)
		}
		obj.Status.PipeARN = aws.ToString(out.Arn)
		obj.Status.PipeState = string(out.CurrentState)
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionEBP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "EventBridgePipe reconciled")
}

func (r *EventBridgePipeReconciler) deletePipe(ctx context.Context, obj *awsv1alpha1.EventBridgePipe) error {
	_, err := r.PipesClient.DeletePipe(ctx, &awspipes.DeletePipeInput{
		Name: aws.String(obj.Spec.Name),
	})
	if pipeshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *EventBridgePipeReconciler) setConditionEBP(ctx context.Context, obj *awsv1alpha1.EventBridgePipe, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *EventBridgePipeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.EventBridgePipe{}).
		Complete(r)
}
