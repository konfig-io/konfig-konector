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
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ebhelper "github.com/konfig-io/konfig-konector/internal/aws/eventbridge"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// EventRuleReconciler reconciles EventRule objects.
type EventRuleReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	EventBridgeClient *multi.EventBridge
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=eventrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eventrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eventrules/finalizers,verbs=update

func (r *EventRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	er := &awsv1alpha1.EventRule{}
	if err := r.Get(ctx, req.NamespacedName, er); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, er); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !er.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(er, awsv1alpha1.FinalizerName) {
			if shouldAbandon(er) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(er, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, er)
			}
			if err := r.deleteEventRule(ctx, er); err != nil {
				logger.Error(err, "failed to delete EventRule")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(er, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, er)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(er, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(er, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, er); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileEventRule(ctx, er); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionER(ctx, er, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *EventRuleReconciler) reconcileEventRule(ctx context.Context, er *awsv1alpha1.EventRule) error {
	busName, err := r.resolveEventBusName(ctx, er)
	if err != nil {
		return err
	}

	input := &awseb.PutRuleInput{
		Name: aws.String(er.Spec.RuleName),
	}
	if busName != "" {
		input.EventBusName = aws.String(busName)
	}
	if er.Spec.Description != "" {
		input.Description = aws.String(er.Spec.Description)
	}
	if er.Spec.EventPattern != "" {
		input.EventPattern = aws.String(er.Spec.EventPattern)
	}
	if er.Spec.ScheduleExpression != "" {
		input.ScheduleExpression = aws.String(er.Spec.ScheduleExpression)
	}
	if er.Spec.State != "" {
		input.State = ebtypes.RuleState(er.Spec.State)
	}
	if er.Spec.RoleARN != "" {
		input.RoleArn = aws.String(er.Spec.RoleARN)
	}
	if len(er.Spec.Tags) > 0 {
		tags := make([]ebtypes.Tag, 0, len(er.Spec.Tags))
		for k, v := range er.Spec.Tags {
			k, v := k, v
			tags = append(tags, ebtypes.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.EventBridgeClient.PutRule(ctx, input)
	if err != nil {
		return fmt.Errorf("put rule: %w", err)
	}

	er.Status.ARN = aws.ToString(out.RuleArn)
	er.Status.ObservedGeneration = er.Generation
	now := metav1.Now()
	er.Status.LastSyncTime = &now
	return r.setConditionER(ctx, er, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "EventRule reconciled")
}

func (r *EventRuleReconciler) resolveEventBusName(ctx context.Context, er *awsv1alpha1.EventRule) (string, error) {
	if er.Spec.EventBusRef == nil {
		return "", nil
	}
	ref := er.Spec.EventBusRef
	if ref.EventBusName != "" {
		return ref.EventBusName, nil
	}
	if ref.Name == "" {
		return "", nil
	}
	ebCR := &awsv1alpha1.EventBus{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: er.Namespace}, ebCR); err != nil {
		return "", err
	}
	if ebCR.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("EventBus %s/%s has no ARN yet", er.Namespace, ref.Name)}
	}
	return ebCR.Spec.EventBusName, nil
}

func (r *EventRuleReconciler) deleteEventRule(ctx context.Context, er *awsv1alpha1.EventRule) error {
	if er.Spec.RuleName == "" {
		return nil
	}
	input := &awseb.DeleteRuleInput{
		Name: aws.String(er.Spec.RuleName),
	}
	if er.Spec.EventBusRef != nil && er.Spec.EventBusRef.EventBusName != "" {
		input.EventBusName = aws.String(er.Spec.EventBusRef.EventBusName)
	}
	_, err := r.EventBridgeClient.DeleteRule(ctx, input)
	if ebhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *EventRuleReconciler) setConditionER(ctx context.Context, er *awsv1alpha1.EventRule, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&er.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: er.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, er); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *EventRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.EventRule{}).
		Complete(r)
}
