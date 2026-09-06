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
	awseb "github.com/aws/aws-sdk-go-v2/service/eventbridge"
	ebtypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ebhelper "github.com/konfig-io/konfig-konector/internal/aws/eventbridge"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// EventBusReconciler reconciles EventBus objects.
type EventBusReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	EventBridgeClient *multi.EventBridge
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=eventbuses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eventbuses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eventbuses/finalizers,verbs=update

func (r *EventBusReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	eb := &awsv1alpha1.EventBus{}
	if err := r.Get(ctx, req.NamespacedName, eb); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, eb); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !eb.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(eb, awsv1alpha1.FinalizerName) {
			if shouldAbandon(eb) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(eb, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, eb)
			}
			if err := r.deleteEventBus(ctx, eb); err != nil {
				logger.Error(err, "failed to delete EventBus")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(eb, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, eb)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(eb, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(eb, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, eb); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileEventBus(ctx, eb); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionEB(ctx, eb, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *EventBusReconciler) reconcileEventBus(ctx context.Context, eb *awsv1alpha1.EventBus) error {
	// Always look the bus up by name first so an existing one is adopted
	// instead of failing with ResourceAlreadyExistsException.
	{
		out, err := r.EventBridgeClient.DescribeEventBus(ctx, &awseb.DescribeEventBusInput{
			Name: aws.String(eb.Spec.EventBusName),
		})
		if err != nil && !ebhelper.IsNotFound(err) {
			return fmt.Errorf("describe event bus: %w", err)
		}
		if err == nil {
			eb.Status.ARN = aws.ToString(out.Arn)
			eb.Status.ObservedGeneration = eb.Generation
			now := metav1.Now()
			eb.Status.LastSyncTime = &now
			return r.setConditionEB(ctx, eb, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "EventBus reconciled")
		}
		eb.Status.ARN = ""
	}

	input := &awseb.CreateEventBusInput{
		Name: aws.String(eb.Spec.EventBusName),
	}
	if len(eb.Spec.Tags) > 0 {
		tags := make([]ebtypes.Tag, 0, len(eb.Spec.Tags))
		for k, v := range eb.Spec.Tags {
			k, v := k, v
			tags = append(tags, ebtypes.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.EventBridgeClient.CreateEventBus(ctx, input)
	if err != nil {
		return fmt.Errorf("create event bus: %w", err)
	}

	eb.Status.ARN = aws.ToString(out.EventBusArn)
	eb.Status.ObservedGeneration = eb.Generation
	now := metav1.Now()
	eb.Status.LastSyncTime = &now
	return r.setConditionEB(ctx, eb, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "EventBus created")
}

func (r *EventBusReconciler) deleteEventBus(ctx context.Context, eb *awsv1alpha1.EventBus) error {
	if eb.Spec.EventBusName == "" {
		return nil
	}
	_, err := r.EventBridgeClient.DeleteEventBus(ctx, &awseb.DeleteEventBusInput{
		Name: aws.String(eb.Spec.EventBusName),
	})
	if ebhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *EventBusReconciler) setConditionEB(ctx context.Context, eb *awsv1alpha1.EventBus, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&eb.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: eb.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, eb); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *EventBusReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.EventBus{}).
		Complete(r)
}
