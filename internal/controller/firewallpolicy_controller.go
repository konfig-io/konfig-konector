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
	awsnfw "github.com/aws/aws-sdk-go-v2/service/networkfirewall"
	nfwtypes "github.com/aws/aws-sdk-go-v2/service/networkfirewall/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	nfwhelper "github.com/konfig-io/konfig-konector/internal/aws/networkfirewall"
)

// FirewallPolicyAWSAPI is the subset of the Network Firewall API used by this controller.
type FirewallPolicyAWSAPI interface {
	DescribeFirewallPolicy(ctx context.Context, params *awsnfw.DescribeFirewallPolicyInput, optFns ...func(*awsnfw.Options)) (*awsnfw.DescribeFirewallPolicyOutput, error)
	CreateFirewallPolicy(ctx context.Context, params *awsnfw.CreateFirewallPolicyInput, optFns ...func(*awsnfw.Options)) (*awsnfw.CreateFirewallPolicyOutput, error)
	UpdateFirewallPolicy(ctx context.Context, params *awsnfw.UpdateFirewallPolicyInput, optFns ...func(*awsnfw.Options)) (*awsnfw.UpdateFirewallPolicyOutput, error)
	DeleteFirewallPolicy(ctx context.Context, params *awsnfw.DeleteFirewallPolicyInput, optFns ...func(*awsnfw.Options)) (*awsnfw.DeleteFirewallPolicyOutput, error)
}

// FirewallPolicyReconciler reconciles FirewallPolicy objects.
type FirewallPolicyReconciler struct {
	client.Client
	Scheme                *runtime.Scheme
	NetworkFirewallClient FirewallPolicyAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=firewallpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=firewallpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=firewallpolicies/finalizers,verbs=update

func (r *FirewallPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.FirewallPolicy{}
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
			if err := r.deletePolicy(ctx, obj); err != nil {
				logger.Error(err, "failed to delete FirewallPolicy")
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

	if err := r.reconcilePolicy(ctx, obj); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

// buildPolicy resolves rule group refs and assembles the AWS FirewallPolicy document.
func (r *FirewallPolicyReconciler) buildPolicy(ctx context.Context, obj *awsv1alpha1.FirewallPolicy) (*nfwtypes.FirewallPolicy, error) {
	policy := &nfwtypes.FirewallPolicy{
		StatelessDefaultActions:         obj.Spec.StatelessDefaultActions,
		StatelessFragmentDefaultActions: obj.Spec.StatelessFragmentDefaultActions,
	}
	for _, ref := range obj.Spec.StatelessRuleGroupRefs {
		arn, err := r.resolveRuleGroupARN(ctx, obj.Namespace, ref.Name, ref.ARN)
		if err != nil {
			return nil, err
		}
		ref := ref
		policy.StatelessRuleGroupReferences = append(policy.StatelessRuleGroupReferences, nfwtypes.StatelessRuleGroupReference{
			ResourceArn: aws.String(arn),
			Priority:    aws.Int32(ref.Priority),
		})
	}
	for _, ref := range obj.Spec.StatefulRuleGroupRefs {
		arn, err := r.resolveRuleGroupARN(ctx, obj.Namespace, ref.Name, ref.ARN)
		if err != nil {
			return nil, err
		}
		policy.StatefulRuleGroupReferences = append(policy.StatefulRuleGroupReferences, nfwtypes.StatefulRuleGroupReference{
			ResourceArn: aws.String(arn),
		})
	}
	return policy, nil
}

func (r *FirewallPolicyReconciler) resolveRuleGroupARN(ctx context.Context, namespace, name, directARN string) (string, error) {
	if directARN != "" {
		return directARN, nil
	}
	if name == "" {
		return "", fmt.Errorf("rule group ref requires name or arn")
	}
	rg := &awsv1alpha1.FirewallRuleGroup{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: name, Namespace: namespace}, rg); err != nil {
		return "", err
	}
	if rg.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("FirewallRuleGroup %s/%s has no ARN yet", namespace, name)}
	}
	return rg.Status.ARN, nil
}

func (r *FirewallPolicyReconciler) reconcilePolicy(ctx context.Context, obj *awsv1alpha1.FirewallPolicy) error {
	// Resolve refs up front so a missing/not-ready rule group is reported
	// before any AWS mutation.
	policy, err := r.buildPolicy(ctx, obj)
	if err != nil {
		return err
	}

	descOut, err := r.NetworkFirewallClient.DescribeFirewallPolicy(ctx, &awsnfw.DescribeFirewallPolicyInput{
		FirewallPolicyName: aws.String(obj.Spec.Name),
	})
	if err != nil && !nfwhelper.IsNotFound(err) {
		return fmt.Errorf("describe firewall policy: %w", err)
	}

	if nfwhelper.IsNotFound(err) {
		input := &awsnfw.CreateFirewallPolicyInput{
			FirewallPolicyName: aws.String(obj.Spec.Name),
			FirewallPolicy:     policy,
			Tags:               nfwTagsFromMap(obj.Spec.Tags),
		}
		if obj.Spec.Description != "" {
			input.Description = aws.String(obj.Spec.Description)
		}
		out, err := r.NetworkFirewallClient.CreateFirewallPolicy(ctx, input)
		if err != nil {
			return fmt.Errorf("create firewall policy: %w", err)
		}
		obj.Status.ARN = aws.ToString(out.FirewallPolicyResponse.FirewallPolicyArn)
		obj.Status.ID = aws.ToString(out.FirewallPolicyResponse.FirewallPolicyId)
		obj.Status.UpdateToken = aws.ToString(out.UpdateToken)
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return fmt.Errorf("persist firewall policy ARN after create: %w", err)
		}
		obj.Status.ObservedGeneration = obj.Generation
		now := metav1.Now()
		obj.Status.LastSyncTime = &now
		return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "FirewallPolicy created")
	}

	obj.Status.ARN = aws.ToString(descOut.FirewallPolicyResponse.FirewallPolicyArn)
	obj.Status.ID = aws.ToString(descOut.FirewallPolicyResponse.FirewallPolicyId)
	obj.Status.UpdateToken = aws.ToString(descOut.UpdateToken)

	if obj.Status.ObservedGeneration != obj.Generation {
		input := &awsnfw.UpdateFirewallPolicyInput{
			FirewallPolicyArn: descOut.FirewallPolicyResponse.FirewallPolicyArn,
			FirewallPolicy:    policy,
			UpdateToken:       descOut.UpdateToken,
		}
		if obj.Spec.Description != "" {
			input.Description = aws.String(obj.Spec.Description)
		}
		out, err := r.NetworkFirewallClient.UpdateFirewallPolicy(ctx, input)
		if err != nil {
			return fmt.Errorf("update firewall policy: %w", err)
		}
		obj.Status.UpdateToken = aws.ToString(out.UpdateToken)
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "FirewallPolicy reconciled")
}

func (r *FirewallPolicyReconciler) deletePolicy(ctx context.Context, obj *awsv1alpha1.FirewallPolicy) error {
	input := &awsnfw.DeleteFirewallPolicyInput{}
	if obj.Status.ARN != "" {
		input.FirewallPolicyArn = aws.String(obj.Status.ARN)
	} else {
		// Name is a deterministic identifier for firewall policies.
		input.FirewallPolicyName = aws.String(obj.Spec.Name)
	}
	_, err := r.NetworkFirewallClient.DeleteFirewallPolicy(ctx, input)
	if nfwhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *FirewallPolicyReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.FirewallPolicy, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *FirewallPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.FirewallPolicy{}).
		Complete(r)
}
