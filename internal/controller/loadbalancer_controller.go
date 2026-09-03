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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	elbv2helper "github.com/konfig-io/konfig-konector/internal/aws/elbv2"
)

// LoadBalancerAWSAPI is the subset of the ELBv2 client used by the
// LoadBalancer controller. *elasticloadbalancingv2.Client satisfies it.
type LoadBalancerAWSAPI interface {
	DescribeLoadBalancers(ctx context.Context, params *awselbv2.DescribeLoadBalancersInput, optFns ...func(*awselbv2.Options)) (*awselbv2.DescribeLoadBalancersOutput, error)
	CreateLoadBalancer(ctx context.Context, params *awselbv2.CreateLoadBalancerInput, optFns ...func(*awselbv2.Options)) (*awselbv2.CreateLoadBalancerOutput, error)
	DeleteLoadBalancer(ctx context.Context, params *awselbv2.DeleteLoadBalancerInput, optFns ...func(*awselbv2.Options)) (*awselbv2.DeleteLoadBalancerOutput, error)
	SetSecurityGroups(ctx context.Context, params *awselbv2.SetSecurityGroupsInput, optFns ...func(*awselbv2.Options)) (*awselbv2.SetSecurityGroupsOutput, error)
	SetSubnets(ctx context.Context, params *awselbv2.SetSubnetsInput, optFns ...func(*awselbv2.Options)) (*awselbv2.SetSubnetsOutput, error)
	ModifyLoadBalancerAttributes(ctx context.Context, params *awselbv2.ModifyLoadBalancerAttributesInput, optFns ...func(*awselbv2.Options)) (*awselbv2.ModifyLoadBalancerAttributesOutput, error)
}

// LoadBalancerReconciler reconciles LoadBalancer objects.
type LoadBalancerReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	ELBv2Client LoadBalancerAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=loadbalancers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=loadbalancers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=loadbalancers/finalizers,verbs=update

func (r *LoadBalancerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	lb := &awsv1alpha1.LoadBalancer{}
	if err := r.Get(ctx, req.NamespacedName, lb); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !lb.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(lb, awsv1alpha1.FinalizerName) {
			if shouldAbandon(lb) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(lb, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, lb)
			}
			if err := r.deleteLoadBalancer(ctx, lb); err != nil {
				logger.Error(err, "failed to delete LoadBalancer")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(lb, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, lb)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(lb, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(lb, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, lb); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileLoadBalancer(ctx, lb); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionLB(ctx, lb, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *LoadBalancerReconciler) reconcileLoadBalancer(ctx context.Context, lb *awsv1alpha1.LoadBalancer) error {
	if lb.Status.ARN != "" {
		out, err := r.ELBv2Client.DescribeLoadBalancers(ctx, &awselbv2.DescribeLoadBalancersInput{
			LoadBalancerArns: []string{lb.Status.ARN},
		})
		if err != nil && !elbv2helper.IsNotFound(err) {
			return fmt.Errorf("describe load balancer: %w", err)
		}
		if err == nil && len(out.LoadBalancers) > 0 {
			existing := out.LoadBalancers[0]
			lb.Status.DNSName = aws.ToString(existing.DNSName)
			lb.Status.State = string(existing.State.Code)
			if lb.Status.ObservedGeneration != lb.Generation {
				if err := r.updateLoadBalancer(ctx, lb, existing); err != nil {
					return err
				}
			}
			lb.Status.ObservedGeneration = lb.Generation
			now := metav1.Now()
			lb.Status.LastSyncTime = &now
			return r.setConditionLB(ctx, lb, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "LoadBalancer reconciled")
		}
		lb.Status.ARN = ""
	}

	subnetIDs, err := resolveSubnetIDs(ctx, r.Client, lb.Namespace, lb.Spec.SubnetRefs)
	if err != nil {
		return err
	}
	sgIDs, err := resolveSGIDs(ctx, r.Client, lb.Namespace, lb.Spec.SecurityGroupRefs)
	if err != nil {
		return err
	}

	lbType := elbv2types.LoadBalancerTypeEnumApplication
	switch lb.Spec.Type {
	case "network":
		lbType = elbv2types.LoadBalancerTypeEnumNetwork
	case "gateway":
		lbType = elbv2types.LoadBalancerTypeEnumGateway
	}

	scheme := elbv2types.LoadBalancerSchemeEnumInternetFacing
	if lb.Spec.Scheme == "internal" {
		scheme = elbv2types.LoadBalancerSchemeEnumInternal
	}

	input := &awselbv2.CreateLoadBalancerInput{
		Name:    aws.String(lb.Spec.Name),
		Type:    lbType,
		Scheme:  scheme,
		Subnets: subnetIDs,
	}
	if len(sgIDs) > 0 {
		input.SecurityGroups = sgIDs
	}
	if len(lb.Spec.Tags) > 0 {
		tags := make([]elbv2types.Tag, 0, len(lb.Spec.Tags))
		for k, v := range lb.Spec.Tags {
			k, v := k, v
			tags = append(tags, elbv2types.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.ELBv2Client.CreateLoadBalancer(ctx, input)
	if err != nil {
		return fmt.Errorf("create load balancer: %w", err)
	}
	if len(out.LoadBalancers) == 0 {
		return fmt.Errorf("create load balancer: empty response")
	}

	created := out.LoadBalancers[0]
	lb.Status.ARN = aws.ToString(created.LoadBalancerArn)
	lb.Status.DNSName = aws.ToString(created.DNSName)
	lb.Status.State = string(created.State.Code)
	// The AWS resource now exists; losing the ARN would orphan it and a
	// retried create-by-name would conflict.
	if err := persistStatus(ctx, r.Client, lb); err != nil {
		return fmt.Errorf("persist status after create: %w", err)
	}
	lb.Status.ObservedGeneration = lb.Generation
	now := metav1.Now()
	lb.Status.LastSyncTime = &now
	return r.setConditionLB(ctx, lb, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "LoadBalancer created")
}

// updateLoadBalancer reconciles mutable ELBv2 settings the spec models:
// security groups, subnets, and load balancer attributes.
func (r *LoadBalancerReconciler) updateLoadBalancer(ctx context.Context, lb *awsv1alpha1.LoadBalancer, existing elbv2types.LoadBalancer) error {
	// Security groups (ALB only; the spec marks these optional).
	if len(lb.Spec.SecurityGroupRefs) > 0 {
		desiredSGs, err := resolveSGIDs(ctx, r.Client, lb.Namespace, lb.Spec.SecurityGroupRefs)
		if err != nil {
			return err
		}
		if !sameStringSet(desiredSGs, existing.SecurityGroups) {
			if _, err := r.ELBv2Client.SetSecurityGroups(ctx, &awselbv2.SetSecurityGroupsInput{
				LoadBalancerArn: aws.String(lb.Status.ARN),
				SecurityGroups:  desiredSGs,
			}); err != nil {
				return fmt.Errorf("set security groups: %w", err)
			}
		}
	}

	// Subnets.
	desiredSubnets, err := resolveSubnetIDs(ctx, r.Client, lb.Namespace, lb.Spec.SubnetRefs)
	if err != nil {
		return err
	}
	currentSubnets := make([]string, 0, len(existing.AvailabilityZones))
	for _, az := range existing.AvailabilityZones {
		currentSubnets = append(currentSubnets, aws.ToString(az.SubnetId))
	}
	if !sameStringSet(desiredSubnets, currentSubnets) {
		if _, err := r.ELBv2Client.SetSubnets(ctx, &awselbv2.SetSubnetsInput{
			LoadBalancerArn: aws.String(lb.Status.ARN),
			Subnets:         desiredSubnets,
		}); err != nil {
			return fmt.Errorf("set subnets: %w", err)
		}
	}

	// Attributes modelled by the spec.
	attrs := []elbv2types.LoadBalancerAttribute{
		{
			Key:   aws.String("deletion_protection.enabled"),
			Value: aws.String(fmt.Sprintf("%t", lb.Spec.DeletionProtection)),
		},
	}
	if lb.Spec.IdleTimeout > 0 {
		attrs = append(attrs, elbv2types.LoadBalancerAttribute{
			Key:   aws.String("idle_timeout.timeout_seconds"),
			Value: aws.String(fmt.Sprintf("%d", lb.Spec.IdleTimeout)),
		})
	}
	if _, err := r.ELBv2Client.ModifyLoadBalancerAttributes(ctx, &awselbv2.ModifyLoadBalancerAttributesInput{
		LoadBalancerArn: aws.String(lb.Status.ARN),
		Attributes:      attrs,
	}); err != nil {
		return fmt.Errorf("modify load balancer attributes: %w", err)
	}
	return nil
}

// sameStringSet reports whether two slices contain the same elements,
// ignoring order.
func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]int, len(a))
	for _, s := range a {
		set[s]++
	}
	for _, s := range b {
		set[s]--
		if set[s] < 0 {
			return false
		}
	}
	return true
}

func (r *LoadBalancerReconciler) deleteLoadBalancer(ctx context.Context, lb *awsv1alpha1.LoadBalancer) error {
	if lb.Status.ARN == "" {
		// Status may have been lost after a successful create; the create
		// path always uses spec.name, so look it up by name before giving up.
		out, err := r.ELBv2Client.DescribeLoadBalancers(ctx, &awselbv2.DescribeLoadBalancersInput{
			Names: []string{lb.Spec.Name},
		})
		if elbv2helper.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("describe load balancer by name: %w", err)
		}
		if len(out.LoadBalancers) == 0 {
			return nil
		}
		lb.Status.ARN = aws.ToString(out.LoadBalancers[0].LoadBalancerArn)
	}
	_, err := r.ELBv2Client.DeleteLoadBalancer(ctx, &awselbv2.DeleteLoadBalancerInput{
		LoadBalancerArn: aws.String(lb.Status.ARN),
	})
	if elbv2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *LoadBalancerReconciler) setConditionLB(ctx context.Context, lb *awsv1alpha1.LoadBalancer, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&lb.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: lb.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, lb); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *LoadBalancerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.LoadBalancer{}).
		Complete(r)
}
