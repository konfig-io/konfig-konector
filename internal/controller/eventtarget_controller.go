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
)

// EventTargetReconciler reconciles EventTarget objects.
type EventTargetReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	EventBridgeClient *awseb.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=eventtargets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eventtargets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eventtargets/finalizers,verbs=update

func (r *EventTargetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	et := &awsv1alpha1.EventTarget{}
	if err := r.Get(ctx, req.NamespacedName, et); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !et.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(et, awsv1alpha1.FinalizerName) {
			if shouldAbandon(et) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(et, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, et)
			}
			if err := r.removeEventTargets(ctx, et); err != nil {
				logger.Error(err, "failed to remove EventTargets")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(et, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, et)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(et, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(et, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, et); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileEventTarget(ctx, et); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionET(ctx, et, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *EventTargetReconciler) reconcileEventTarget(ctx context.Context, et *awsv1alpha1.EventTarget) error {
	ruleName, busName, err := r.resolveRuleAndBus(ctx, et)
	if err != nil {
		return err
	}

	targets := make([]ebtypes.Target, 0, len(et.Spec.Targets))
	for _, t := range et.Spec.Targets {
		t := t
		tgt := ebtypes.Target{
			Id:  aws.String(t.ID),
			Arn: aws.String(t.ARN),
		}
		if t.RoleARN != "" {
			tgt.RoleArn = aws.String(t.RoleARN)
		}
		if t.Input != "" {
			tgt.Input = aws.String(t.Input)
		}
		if t.InputPath != "" {
			tgt.InputPath = aws.String(t.InputPath)
		}
		targets = append(targets, tgt)
	}

	input := &awseb.PutTargetsInput{
		Rule:    aws.String(ruleName),
		Targets: targets,
	}
	if busName != "" {
		input.EventBusName = aws.String(busName)
	}

	_, err = r.EventBridgeClient.PutTargets(ctx, input)
	if err != nil {
		return fmt.Errorf("put targets: %w", err)
	}

	et.Status.ObservedGeneration = et.Generation
	now := metav1.Now()
	et.Status.LastSyncTime = &now
	return r.setConditionET(ctx, et, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "EventTarget reconciled")
}

func (r *EventTargetReconciler) resolveRuleAndBus(ctx context.Context, et *awsv1alpha1.EventTarget) (string, string, error) {
	ruleRef := et.Spec.EventRuleRef
	ruleName := ruleRef.RuleName
	if ruleName == "" && ruleRef.Name != "" {
		erCR := &awsv1alpha1.EventRule{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: ruleRef.Name, Namespace: et.Namespace}, erCR); err != nil {
			return "", "", err
		}
		if erCR.Status.ARN == "" {
			return "", "", &dependencyNotReady{msg: fmt.Sprintf("EventRule %s/%s has no ARN yet", et.Namespace, ruleRef.Name)}
		}
		ruleName = erCR.Spec.RuleName
	}
	if ruleName == "" {
		return "", "", fmt.Errorf("eventRuleRef requires name or ruleName")
	}

	busName := ""
	if et.Spec.EventBusRef != nil {
		busRef := et.Spec.EventBusRef
		if busRef.EventBusName != "" {
			busName = busRef.EventBusName
		} else if busRef.Name != "" {
			ebCR := &awsv1alpha1.EventBus{}
			if err := r.Get(ctx, k8stypes.NamespacedName{Name: busRef.Name, Namespace: et.Namespace}, ebCR); err != nil {
				return "", "", err
			}
			if ebCR.Status.ARN == "" {
				return "", "", &dependencyNotReady{msg: fmt.Sprintf("EventBus %s/%s has no ARN yet", et.Namespace, busRef.Name)}
			}
			busName = ebCR.Spec.EventBusName
		}
	}
	return ruleName, busName, nil
}

func (r *EventTargetReconciler) removeEventTargets(ctx context.Context, et *awsv1alpha1.EventTarget) error {
	ruleName, busName, err := r.resolveRuleAndBus(ctx, et)
	if err != nil {
		return nil
	}
	ids := make([]string, 0, len(et.Spec.Targets))
	for _, t := range et.Spec.Targets {
		ids = append(ids, t.ID)
	}
	input := &awseb.RemoveTargetsInput{
		Rule: aws.String(ruleName),
		Ids:  ids,
	}
	if busName != "" {
		input.EventBusName = aws.String(busName)
	}
	_, err = r.EventBridgeClient.RemoveTargets(ctx, input)
	if ebhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *EventTargetReconciler) setConditionET(ctx context.Context, et *awsv1alpha1.EventTarget, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&et.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: et.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, et); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *EventTargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.EventTarget{}).
		Complete(r)
}
