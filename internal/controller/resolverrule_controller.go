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
	awsr53r "github.com/aws/aws-sdk-go-v2/service/route53resolver"
	r53rtypes "github.com/aws/aws-sdk-go-v2/service/route53resolver/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
	r53rhelper "github.com/konfig-io/konfig-konector/internal/aws/route53resolver"
)

// ResolverRuleReconciler reconciles ResolverRule objects.
type ResolverRuleReconciler struct {
	client.Client
	Scheme                *runtime.Scheme
	Route53ResolverClient *multi.Route53Resolver
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=resolverrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=resolverrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=resolverrules/finalizers,verbs=update

func (r *ResolverRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.ResolverRule{}
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
			if err := r.deleteRule(ctx, obj); err != nil {
				logger.Error(err, "failed to delete ResolverRule")
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

	if err := r.reconcileRule(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionRR(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ResolverRuleReconciler) reconcileRule(ctx context.Context, obj *awsv1alpha1.ResolverRule) error {
	targetIPs := buildTargetIPs(obj.Spec.TargetIPs)

	if obj.Status.RuleID != "" {
		getOut, err := r.Route53ResolverClient.GetResolverRule(ctx, &awsr53r.GetResolverRuleInput{
			ResolverRuleId: aws.String(obj.Status.RuleID),
		})
		if err != nil && !r53rhelper.IsNotFound(err) {
			return fmt.Errorf("get resolver rule: %w", err)
		}
		if err == nil && getOut.ResolverRule != nil {
			rr := getOut.ResolverRule
			obj.Status.Status = string(rr.Status)
			obj.Status.RuleARN = aws.ToString(rr.Arn)

			updateConfig := &r53rtypes.ResolverRuleConfig{
				Name:      aws.String(obj.Spec.Name),
				TargetIps: targetIPs,
			}
			if obj.Spec.ResolverEndpointID != "" {
				updateConfig.ResolverEndpointId = aws.String(obj.Spec.ResolverEndpointID)
			}
			if _, err := r.Route53ResolverClient.UpdateResolverRule(ctx, &awsr53r.UpdateResolverRuleInput{
				ResolverRuleId: aws.String(obj.Status.RuleID),
				Config:         updateConfig,
			}); err != nil {
				return fmt.Errorf("update resolver rule: %w", err)
			}
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionRR(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ResolverRule reconciled")
		}
		obj.Status.RuleID = ""
		obj.Status.RuleARN = ""
	}

	input := &awsr53r.CreateResolverRuleInput{
		CreatorRequestId: aws.String(string(obj.UID)),
		DomainName:       aws.String(obj.Spec.DomainName),
		RuleType:         r53rtypes.RuleTypeOption(obj.Spec.RuleType),
		Name:             aws.String(obj.Spec.Name),
		TargetIps:        targetIPs,
	}
	if obj.Spec.ResolverEndpointID != "" {
		input.ResolverEndpointId = aws.String(obj.Spec.ResolverEndpointID)
	}
	if len(obj.Spec.Tags) > 0 {
		tags := make([]r53rtypes.Tag, 0, len(obj.Spec.Tags))
		for k, v := range obj.Spec.Tags {
			k, v := k, v
			tags = append(tags, r53rtypes.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.Route53ResolverClient.CreateResolverRule(ctx, input)
	if err != nil {
		return fmt.Errorf("create resolver rule: %w", err)
	}

	if out.ResolverRule != nil {
		obj.Status.RuleID = aws.ToString(out.ResolverRule.Id)
		obj.Status.RuleARN = aws.ToString(out.ResolverRule.Arn)
		obj.Status.Status = string(out.ResolverRule.Status)
		// The AWS resource now exists; losing the ID would orphan it.
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return fmt.Errorf("persist status after create: %w", err)
		}
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionRR(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "ResolverRule created")
}

func buildTargetIPs(tips []awsv1alpha1.ResolverTargetIP) []r53rtypes.TargetAddress {
	out := make([]r53rtypes.TargetAddress, 0, len(tips))
	for _, t := range tips {
		ta := r53rtypes.TargetAddress{Ip: aws.String(t.IP)}
		if t.Port != nil {
			ta.Port = t.Port
		}
		out = append(out, ta)
	}
	return out
}

func (r *ResolverRuleReconciler) deleteRule(ctx context.Context, obj *awsv1alpha1.ResolverRule) error {
	if obj.Status.RuleID == "" {
		// Status may have been lost after a successful create; the create
		// path sets CreatorRequestId to this CR's UID, so filter on it to
		// find only the rule this CR created before giving up.
		paginator := awsr53r.NewListResolverRulesPaginator(r.Route53ResolverClient, &awsr53r.ListResolverRulesInput{
			Filters: []r53rtypes.Filter{{
				Name:   aws.String("CreatorRequestId"),
				Values: []string{string(obj.UID)},
			}},
		})
		for paginator.HasMorePages() && obj.Status.RuleID == "" {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				return fmt.Errorf("list resolver rules: %w", err)
			}
			for _, rr := range page.ResolverRules {
				if aws.ToString(rr.CreatorRequestId) == string(obj.UID) {
					obj.Status.RuleID = aws.ToString(rr.Id)
					break
				}
			}
		}
		if obj.Status.RuleID == "" {
			return nil
		}
	}
	_, err := r.Route53ResolverClient.DeleteResolverRule(ctx, &awsr53r.DeleteResolverRuleInput{
		ResolverRuleId: aws.String(obj.Status.RuleID),
	})
	if r53rhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ResolverRuleReconciler) setConditionRR(ctx context.Context, obj *awsv1alpha1.ResolverRule, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *ResolverRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ResolverRule{}).
		Complete(r)
}
