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
	awselbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	elbv2helper "github.com/konfig-io/konfig-konector/internal/aws/elbv2"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// ListenerRuleReconciler reconciles ListenerRule objects.
type ListenerRuleReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	ELBv2Client *multi.ELBv2
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=listenerrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=listenerrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=listenerrules/finalizers,verbs=update

func (r *ListenerRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	lr := &awsv1alpha1.ListenerRule{}
	if err := r.Get(ctx, req.NamespacedName, lr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, lr); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !lr.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(lr, awsv1alpha1.FinalizerName) {
			if shouldAbandon(lr) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(lr, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, lr)
			}
			if err := r.deleteListenerRule(ctx, lr); err != nil {
				logger.Error(err, "failed to delete ListenerRule")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(lr, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, lr)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(lr, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(lr, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, lr); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileListenerRule(ctx, lr); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionLR(ctx, lr, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ListenerRuleReconciler) reconcileListenerRule(ctx context.Context, lr *awsv1alpha1.ListenerRule) error {
	if lr.Status.ARN != "" {
		out, err := r.ELBv2Client.DescribeRules(ctx, &awselbv2.DescribeRulesInput{
			RuleArns: []string{lr.Status.ARN},
		})
		if err != nil && !elbv2helper.IsNotFound(err) {
			return fmt.Errorf("describe listener rule: %w", err)
		}
		if err == nil && len(out.Rules) > 0 {
			lr.Status.ObservedGeneration = lr.Generation
			now := metav1.Now()
			lr.Status.LastSyncTime = &now
			return r.setConditionLR(ctx, lr, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ListenerRule reconciled")
		}
		lr.Status.ARN = ""
	}

	listenerARN, err := r.resolveListenerARN(ctx, lr)
	if err != nil {
		return err
	}

	actions, err := r.buildRuleActions(ctx, lr.Spec.Actions, lr.Namespace)
	if err != nil {
		return err
	}

	conditions := make([]elbv2types.RuleCondition, 0, len(lr.Spec.Conditions))
	for _, c := range lr.Spec.Conditions {
		c := c
		cond := elbv2types.RuleCondition{
			Field: aws.String(c.Field),
		}
		if len(c.Values) > 0 {
			cond.Values = c.Values
		}
		conditions = append(conditions, cond)
	}

	input := &awselbv2.CreateRuleInput{
		ListenerArn: aws.String(listenerARN),
		Priority:    aws.Int32(lr.Spec.Priority),
		Conditions:  conditions,
		Actions:     actions,
	}
	if len(lr.Spec.Tags) > 0 {
		tags := make([]elbv2types.Tag, 0, len(lr.Spec.Tags))
		for k, v := range lr.Spec.Tags {
			k, v := k, v
			tags = append(tags, elbv2types.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.ELBv2Client.CreateRule(ctx, input)
	if err != nil {
		return fmt.Errorf("create listener rule: %w", err)
	}
	if len(out.Rules) == 0 {
		return fmt.Errorf("create listener rule: empty response")
	}

	lr.Status.ARN = aws.ToString(out.Rules[0].RuleArn)
	// The AWS resource now exists; losing the ARN would orphan it.
	if err := persistStatus(ctx, r.Client, lr); err != nil {
		return fmt.Errorf("persist status after create: %w", err)
	}
	lr.Status.ObservedGeneration = lr.Generation
	now := metav1.Now()
	lr.Status.LastSyncTime = &now
	return r.setConditionLR(ctx, lr, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "ListenerRule created")
}

func (r *ListenerRuleReconciler) resolveListenerARN(ctx context.Context, lr *awsv1alpha1.ListenerRule) (string, error) {
	ref := lr.Spec.ListenerRef
	if ref.ARN != "" {
		return ref.ARN, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("listenerRef requires name or arn")
	}
	lCR := &awsv1alpha1.Listener{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: lr.Namespace}, lCR); err != nil {
		return "", err
	}
	if lCR.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("Listener %s/%s has no ARN yet", lr.Namespace, ref.Name)}
	}
	return lCR.Status.ARN, nil
}

func (r *ListenerRuleReconciler) buildRuleActions(ctx context.Context, specs []awsv1alpha1.ListenerDefaultAction, namespace string) ([]elbv2types.Action, error) {
	actions := make([]elbv2types.Action, 0, len(specs))
	for i, s := range specs {
		a := elbv2types.Action{
			Type:  elbv2types.ActionTypeEnum(s.Type),
			Order: aws.Int32(int32(i + 1)),
		}
		if s.TargetGroupRef != nil {
			tgARN, err := r.resolveTGARNForRule(ctx, s.TargetGroupRef, namespace)
			if err != nil {
				return nil, err
			}
			a.ForwardConfig = &elbv2types.ForwardActionConfig{
				TargetGroups: []elbv2types.TargetGroupTuple{{TargetGroupArn: aws.String(tgARN)}},
			}
		}
		if rc := s.RedirectConfig; rc != nil {
			a.RedirectConfig = &elbv2types.RedirectActionConfig{
				StatusCode: elbv2types.RedirectActionStatusCodeEnum(rc.StatusCode),
				Host:       aws.String(rc.Host),
				Path:       aws.String(rc.Path),
				Port:       aws.String(rc.Port),
				Protocol:   aws.String(rc.Protocol),
			}
		}
		if fc := s.FixedResponseConfig; fc != nil {
			a.FixedResponseConfig = &elbv2types.FixedResponseActionConfig{
				StatusCode:  aws.String(fc.StatusCode),
				ContentType: aws.String(fc.ContentType),
				MessageBody: aws.String(fc.MessageBody),
			}
		}
		actions = append(actions, a)
	}
	return actions, nil
}

func (r *ListenerRuleReconciler) resolveTGARNForRule(ctx context.Context, ref *awsv1alpha1.TargetGroupRef, namespace string) (string, error) {
	if ref.ARN != "" {
		return ref.ARN, nil
	}
	tgCR := &awsv1alpha1.TargetGroup{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, tgCR); err != nil {
		return "", err
	}
	if tgCR.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("TargetGroup %s/%s has no ARN yet", namespace, ref.Name)}
	}
	return tgCR.Status.ARN, nil
}

func (r *ListenerRuleReconciler) deleteListenerRule(ctx context.Context, lr *awsv1alpha1.ListenerRule) error {
	if lr.Status.ARN == "" {
		return nil
	}
	_, err := r.ELBv2Client.DeleteRule(ctx, &awselbv2.DeleteRuleInput{
		RuleArn: aws.String(lr.Status.ARN),
	})
	if elbv2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ListenerRuleReconciler) setConditionLR(ctx context.Context, lr *awsv1alpha1.ListenerRule, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&lr.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: lr.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, lr); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *ListenerRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ListenerRule{}).
		Complete(r)
}
