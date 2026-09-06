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

	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	lambdahelper "github.com/konfig-io/konfig-konector/internal/aws/lambda"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// LambdaEventSourceMappingReconciler reconciles LambdaEventSourceMapping objects.
type LambdaEventSourceMappingReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	LambdaClient *multi.Lambda
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdaeventsourcemappings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdaeventsourcemappings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdaeventsourcemappings/finalizers,verbs=update

func (r *LambdaEventSourceMappingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	esm := &awsv1alpha1.LambdaEventSourceMapping{}
	if err := r.Get(ctx, req.NamespacedName, esm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, esm); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !esm.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(esm, awsv1alpha1.FinalizerName) {
			if shouldAbandon(esm) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(esm, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, esm)
			}
			if esm.Status.UUID != "" {
				if err := lambdahelper.DeleteEventSourceMapping(ctx, r.LambdaClient, esm.Status.UUID); err != nil && !lambdahelper.IsNotFound(err) {
					logger.Error(err, "failed to delete event source mapping")
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(esm, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, esm)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(esm, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(esm, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, esm); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	result, err := r.reconcileESM(ctx, esm)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, esm, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *LambdaEventSourceMappingReconciler) reconcileESM(ctx context.Context, esm *awsv1alpha1.LambdaEventSourceMapping) (ctrl.Result, error) {
	functionArn, err := resolveLambdaFunctionName(ctx, r.Client, esm.Namespace, esm.Spec.FunctionArn, esm.Spec.FunctionRef)
	if err != nil {
		return ctrl.Result{}, err
	}

	var filterPatterns []string
	if esm.Spec.FilterCriteria != nil {
		filterPatterns = esm.Spec.FilterCriteria.Filters
	}

	esmIn := lambdahelper.EventSourceMappingInput{
		FunctionArn:                    functionArn,
		EventSourceArn:                 esm.Spec.EventSourceArn,
		BatchSize:                      esm.Spec.BatchSize,
		Enabled:                        esm.Spec.Enabled,
		MaximumBatchingWindowInSeconds: esm.Spec.MaximumBatchingWindowInSeconds,
		FilterPatterns:                 filterPatterns,
	}
	if esm.Spec.StartingPosition != "" {
		esmIn.StartingPosition = types.EventSourcePosition(esm.Spec.StartingPosition)
	}

	if esm.Status.UUID != "" {
		existing, err := lambdahelper.GetEventSourceMapping(ctx, r.LambdaClient, esm.Status.UUID)
		if err != nil && !lambdahelper.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err == nil && existing != nil {
			esm.Status.State = aws_toString(existing.State)

			if esm.Status.ObservedGeneration != esm.Generation {
				if err := lambdahelper.UpdateEventSourceMapping(ctx, r.LambdaClient, esm.Status.UUID, esmIn); err != nil {
					return ctrl.Result{}, fmt.Errorf("update event source mapping: %w", err)
				}
			}

			esm.Status.ObservedGeneration = esm.Generation
			now := metav1.Now()
			esm.Status.LastSyncTime = &now
			return requeueResult(), r.setCondition(ctx, esm, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Event source mapping active")
		}
	}

	created, err := lambdahelper.CreateEventSourceMapping(ctx, r.LambdaClient, esmIn)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create event source mapping: %w", err)
	}
	if created.UUID != nil {
		esm.Status.UUID = *created.UUID
		// Persist the UUID immediately: the AWS resource now exists, and losing
		// the identifier would orphan it or create a duplicate on retry.
		if err := persistStatus(ctx, r.Client, esm); err != nil {
			return ctrl.Result{}, fmt.Errorf("persist event source mapping UUID after create: %w", err)
		}
	}
	esm.Status.State = aws_toString(created.State)
	esm.Status.ObservedGeneration = esm.Generation
	now := metav1.Now()
	esm.Status.LastSyncTime = &now
	return requeueResult(), r.setCondition(ctx, esm, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Event source mapping created")
}

// aws_toString safely dereferences a *string returned from the SDK.
func aws_toString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (r *LambdaEventSourceMappingReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.LambdaEventSourceMapping, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *LambdaEventSourceMappingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.LambdaEventSourceMapping{}).
		Complete(r)
}
