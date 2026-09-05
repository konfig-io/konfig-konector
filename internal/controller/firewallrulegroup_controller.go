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
	awsnfw "github.com/aws/aws-sdk-go-v2/service/networkfirewall"
	nfwtypes "github.com/aws/aws-sdk-go-v2/service/networkfirewall/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	nfwhelper "github.com/konfig-io/konfig-konector/internal/aws/networkfirewall"
)

// FirewallRuleGroupAWSAPI is the subset of the Network Firewall API used by this controller.
type FirewallRuleGroupAWSAPI interface {
	DescribeRuleGroup(ctx context.Context, params *awsnfw.DescribeRuleGroupInput, optFns ...func(*awsnfw.Options)) (*awsnfw.DescribeRuleGroupOutput, error)
	CreateRuleGroup(ctx context.Context, params *awsnfw.CreateRuleGroupInput, optFns ...func(*awsnfw.Options)) (*awsnfw.CreateRuleGroupOutput, error)
	UpdateRuleGroup(ctx context.Context, params *awsnfw.UpdateRuleGroupInput, optFns ...func(*awsnfw.Options)) (*awsnfw.UpdateRuleGroupOutput, error)
	DeleteRuleGroup(ctx context.Context, params *awsnfw.DeleteRuleGroupInput, optFns ...func(*awsnfw.Options)) (*awsnfw.DeleteRuleGroupOutput, error)
}

// FirewallRuleGroupReconciler reconciles FirewallRuleGroup objects.
type FirewallRuleGroupReconciler struct {
	client.Client
	Scheme                *runtime.Scheme
	NetworkFirewallClient FirewallRuleGroupAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=firewallrulegroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=firewallrulegroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=firewallrulegroups/finalizers,verbs=update

func (r *FirewallRuleGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.FirewallRuleGroup{}
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
			if err := r.deleteRuleGroup(ctx, obj); err != nil {
				logger.Error(err, "failed to delete FirewallRuleGroup")
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

	if err := r.reconcileRuleGroup(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *FirewallRuleGroupReconciler) reconcileRuleGroup(ctx context.Context, obj *awsv1alpha1.FirewallRuleGroup) error {
	descOut, err := r.NetworkFirewallClient.DescribeRuleGroup(ctx, &awsnfw.DescribeRuleGroupInput{
		RuleGroupName: aws.String(obj.Spec.Name),
		Type:          nfwtypes.RuleGroupType(obj.Spec.Type),
	})
	if err != nil && !nfwhelper.IsNotFound(err) {
		return fmt.Errorf("describe rule group: %w", err)
	}

	if nfwhelper.IsNotFound(err) {
		input := &awsnfw.CreateRuleGroupInput{
			RuleGroupName: aws.String(obj.Spec.Name),
			Type:          nfwtypes.RuleGroupType(obj.Spec.Type),
			Capacity:      aws.Int32(obj.Spec.Capacity),
		}
		if obj.Spec.RulesString != "" {
			input.Rules = aws.String(obj.Spec.RulesString)
		}
		if obj.Spec.Description != "" {
			input.Description = aws.String(obj.Spec.Description)
		}
		input.Tags = nfwTagsFromMap(obj.Spec.Tags)
		out, err := r.NetworkFirewallClient.CreateRuleGroup(ctx, input)
		if err != nil {
			return fmt.Errorf("create rule group: %w", err)
		}
		obj.Status.ARN = aws.ToString(out.RuleGroupResponse.RuleGroupArn)
		obj.Status.ID = aws.ToString(out.RuleGroupResponse.RuleGroupId)
		obj.Status.UpdateToken = aws.ToString(out.UpdateToken)
		// Persist identifier immediately: the AWS resource now exists.
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return fmt.Errorf("persist rule group ARN after create: %w", err)
		}
		obj.Status.ObservedGeneration = obj.Generation
		now := metav1.Now()
		obj.Status.LastSyncTime = &now
		return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "FirewallRuleGroup created")
	}

	obj.Status.ARN = aws.ToString(descOut.RuleGroupResponse.RuleGroupArn)
	obj.Status.ID = aws.ToString(descOut.RuleGroupResponse.RuleGroupId)
	obj.Status.UpdateToken = aws.ToString(descOut.UpdateToken)

	// Only mutate when the spec changed since the last sync.
	if obj.Status.ObservedGeneration != obj.Generation {
		input := &awsnfw.UpdateRuleGroupInput{
			RuleGroupArn: descOut.RuleGroupResponse.RuleGroupArn,
			Type:         nfwtypes.RuleGroupType(obj.Spec.Type),
			UpdateToken:  descOut.UpdateToken,
		}
		if obj.Spec.RulesString != "" {
			input.Rules = aws.String(obj.Spec.RulesString)
		}
		if obj.Spec.Description != "" {
			input.Description = aws.String(obj.Spec.Description)
		}
		out, err := r.NetworkFirewallClient.UpdateRuleGroup(ctx, input)
		if err != nil {
			return fmt.Errorf("update rule group: %w", err)
		}
		obj.Status.UpdateToken = aws.ToString(out.UpdateToken)
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "FirewallRuleGroup reconciled")
}

func (r *FirewallRuleGroupReconciler) deleteRuleGroup(ctx context.Context, obj *awsv1alpha1.FirewallRuleGroup) error {
	input := &awsnfw.DeleteRuleGroupInput{}
	if obj.Status.ARN != "" {
		input.RuleGroupArn = aws.String(obj.Status.ARN)
	} else {
		// Name + type is a deterministic identifier for rule groups.
		input.RuleGroupName = aws.String(obj.Spec.Name)
		input.Type = nfwtypes.RuleGroupType(obj.Spec.Type)
	}
	_, err := r.NetworkFirewallClient.DeleteRuleGroup(ctx, input)
	if nfwhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *FirewallRuleGroupReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.FirewallRuleGroup, condType string, status metav1.ConditionStatus, reason, message string) error {
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

// nfwTagsFromMap converts a tag map to Network Firewall tag structs.
func nfwTagsFromMap(m map[string]string) []nfwtypes.Tag {
	if len(m) == 0 {
		return nil
	}
	tags := make([]nfwtypes.Tag, 0, len(m))
	for k, v := range m {
		k, v := k, v
		tags = append(tags, nfwtypes.Tag{Key: &k, Value: &v})
	}
	return tags
}

func (r *FirewallRuleGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.FirewallRuleGroup{}).
		Complete(r)
}
