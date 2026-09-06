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
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ec2helper "github.com/konfig-io/konfig-konector/internal/aws/ec2"
)

// VPCEndpointServiceAWSAPI is the subset of the EC2 API used by this controller.
type VPCEndpointServiceAWSAPI interface {
	CreateVpcEndpointServiceConfiguration(ctx context.Context, params *awsec2.CreateVpcEndpointServiceConfigurationInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateVpcEndpointServiceConfigurationOutput, error)
	DescribeVpcEndpointServiceConfigurations(ctx context.Context, params *awsec2.DescribeVpcEndpointServiceConfigurationsInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeVpcEndpointServiceConfigurationsOutput, error)
	ModifyVpcEndpointServiceConfiguration(ctx context.Context, params *awsec2.ModifyVpcEndpointServiceConfigurationInput, optFns ...func(*awsec2.Options)) (*awsec2.ModifyVpcEndpointServiceConfigurationOutput, error)
	DeleteVpcEndpointServiceConfigurations(ctx context.Context, params *awsec2.DeleteVpcEndpointServiceConfigurationsInput, optFns ...func(*awsec2.Options)) (*awsec2.DeleteVpcEndpointServiceConfigurationsOutput, error)
	DescribeVpcEndpointServicePermissions(ctx context.Context, params *awsec2.DescribeVpcEndpointServicePermissionsInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeVpcEndpointServicePermissionsOutput, error)
	ModifyVpcEndpointServicePermissions(ctx context.Context, params *awsec2.ModifyVpcEndpointServicePermissionsInput, optFns ...func(*awsec2.Options)) (*awsec2.ModifyVpcEndpointServicePermissionsOutput, error)
	DescribeVpcEndpointConnections(ctx context.Context, params *awsec2.DescribeVpcEndpointConnectionsInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeVpcEndpointConnectionsOutput, error)
	AcceptVpcEndpointConnections(ctx context.Context, params *awsec2.AcceptVpcEndpointConnectionsInput, optFns ...func(*awsec2.Options)) (*awsec2.AcceptVpcEndpointConnectionsOutput, error)
	CreateTags(ctx context.Context, params *awsec2.CreateTagsInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateTagsOutput, error)
}

// VPCEndpointServiceReconciler manages PrivateLink endpoint services.
type VPCEndpointServiceReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client VPCEndpointServiceAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpcendpointservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpcendpointservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpcendpointservices/finalizers,verbs=update

func (r *VPCEndpointServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	obj := &awsv1alpha1.VPCEndpointService{}
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
			if err := r.deleteService(ctx, obj); err != nil {
				logger.Error(err, "failed to delete VPC endpoint service")
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

	if err := r.reconcileService(ctx, obj); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		if errors.Is(err, errPendingAcceptance) {
			return requeuePending, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *VPCEndpointServiceReconciler) resolveNLBs(ctx context.Context, obj *awsv1alpha1.VPCEndpointService) ([]string, error) {
	var arns []string
	for _, ref := range obj.Spec.NetworkLoadBalancerRefs {
		if ref.ARN != "" {
			arns = append(arns, ref.ARN)
			continue
		}
		lb := &awsv1alpha1.LoadBalancer{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: obj.Namespace}, lb); err != nil {
			return nil, err
		}
		if lb.Status.ARN == "" {
			return nil, &dependencyNotReady{msg: fmt.Sprintf("LoadBalancer %s/%s has no ARN yet", obj.Namespace, ref.Name)}
		}
		arns = append(arns, lb.Status.ARN)
	}
	sort.Strings(arns)
	return arns, nil
}

func (r *VPCEndpointServiceReconciler) reconcileService(ctx context.Context, obj *awsv1alpha1.VPCEndpointService) error {
	nlbs, err := r.resolveNLBs(ctx, obj)
	if err != nil {
		return err
	}
	if len(nlbs) == 0 && len(obj.Spec.GatewayLoadBalancerArns) == 0 {
		return fmt.Errorf("at least one networkLoadBalancerRef or gatewayLoadBalancerArn is required")
	}

	var cfg *ec2types.ServiceConfiguration
	if obj.Status.ServiceID != "" {
		out, err := r.EC2Client.DescribeVpcEndpointServiceConfigurations(ctx, &awsec2.DescribeVpcEndpointServiceConfigurationsInput{
			ServiceIds: []string{obj.Status.ServiceID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return fmt.Errorf("describe endpoint service: %w", err)
		}
		if err == nil && len(out.ServiceConfigurations) > 0 && out.ServiceConfigurations[0].ServiceState != ec2types.ServiceStateDeleted {
			cfg = &out.ServiceConfigurations[0]
		} else {
			obj.Status.ServiceID = ""
		}
	}

	if cfg == nil {
		in := &awsec2.CreateVpcEndpointServiceConfigurationInput{
			AcceptanceRequired:      aws.Bool(obj.Spec.AcceptanceRequired),
			NetworkLoadBalancerArns: nlbs,
			GatewayLoadBalancerArns: obj.Spec.GatewayLoadBalancerArns,
			TagSpecifications: []ec2types.TagSpecification{{
				ResourceType: ec2types.ResourceTypeVpcEndpointService,
				Tags:         ec2helper.TagsFromMap(obj.Spec.Tags),
			}},
		}
		if obj.Spec.PrivateDNSName != "" {
			in.PrivateDnsName = aws.String(obj.Spec.PrivateDNSName)
		}
		for _, t := range obj.Spec.SupportedIPAddressTypes {
			in.SupportedIpAddressTypes = append(in.SupportedIpAddressTypes, t)
		}
		out, err := r.EC2Client.CreateVpcEndpointServiceConfiguration(ctx, in)
		if err != nil {
			return fmt.Errorf("create endpoint service: %w", err)
		}
		cfg = out.ServiceConfiguration
		obj.Status.ServiceID = aws.ToString(cfg.ServiceId)
		obj.Status.ServiceName = aws.ToString(cfg.ServiceName)
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return fmt.Errorf("persist service ID after create: %w", err)
		}
	} else if obj.Status.ObservedGeneration != obj.Generation {
		mod := &awsec2.ModifyVpcEndpointServiceConfigurationInput{
			ServiceId:          aws.String(obj.Status.ServiceID),
			AcceptanceRequired: aws.Bool(obj.Spec.AcceptanceRequired),
		}
		add, remove := diffStrings(cfg.NetworkLoadBalancerArns, nlbs)
		mod.AddNetworkLoadBalancerArns, mod.RemoveNetworkLoadBalancerArns = add, remove
		gadd, gremove := diffStrings(cfg.GatewayLoadBalancerArns, obj.Spec.GatewayLoadBalancerArns)
		mod.AddGatewayLoadBalancerArns, mod.RemoveGatewayLoadBalancerArns = gadd, gremove
		if obj.Spec.PrivateDNSName != "" && obj.Spec.PrivateDNSName != aws.ToString(cfg.PrivateDnsName) {
			mod.PrivateDnsName = aws.String(obj.Spec.PrivateDNSName)
		} else if obj.Spec.PrivateDNSName == "" && aws.ToString(cfg.PrivateDnsName) != "" {
			mod.RemovePrivateDnsName = aws.Bool(true)
		}
		if _, err := r.EC2Client.ModifyVpcEndpointServiceConfiguration(ctx, mod); err != nil {
			return fmt.Errorf("modify endpoint service: %w", err)
		}
		if len(obj.Spec.Tags) > 0 {
			if _, err := r.EC2Client.CreateTags(ctx, &awsec2.CreateTagsInput{
				Resources: []string{obj.Status.ServiceID}, Tags: ec2helper.TagsFromMap(obj.Spec.Tags),
			}); err != nil {
				return fmt.Errorf("tag endpoint service: %w", err)
			}
		}
	}

	// Allowed principals (always reconciled: this is the cross-account gate).
	permOut, err := r.EC2Client.DescribeVpcEndpointServicePermissions(ctx, &awsec2.DescribeVpcEndpointServicePermissionsInput{
		ServiceId: aws.String(obj.Status.ServiceID),
	})
	if err != nil {
		return fmt.Errorf("describe endpoint service permissions: %w", err)
	}
	var current []string
	for _, p := range permOut.AllowedPrincipals {
		current = append(current, aws.ToString(p.Principal))
	}
	addP, removeP := diffStrings(current, obj.Spec.AllowedPrincipals)
	if len(addP) > 0 || len(removeP) > 0 {
		if _, err := r.EC2Client.ModifyVpcEndpointServicePermissions(ctx, &awsec2.ModifyVpcEndpointServicePermissionsInput{
			ServiceId:               aws.String(obj.Status.ServiceID),
			AddAllowedPrincipals:    addP,
			RemoveAllowedPrincipals: removeP,
		}); err != nil {
			return fmt.Errorf("modify endpoint service permissions: %w", err)
		}
	}

	// Consumer connections awaiting acceptance.
	connOut, err := r.EC2Client.DescribeVpcEndpointConnections(ctx, &awsec2.DescribeVpcEndpointConnectionsInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("service-id"), Values: []string{obj.Status.ServiceID}},
			{Name: aws.String("vpc-endpoint-state"), Values: []string{"pendingAcceptance"}},
		},
	})
	if err != nil {
		return fmt.Errorf("describe endpoint connections: %w", err)
	}
	var pending []string
	for _, c := range connOut.VpcEndpointConnections {
		pending = append(pending, aws.ToString(c.VpcEndpointId))
	}
	if len(pending) > 0 && obj.Spec.AutoAcceptConnections {
		if _, err := r.EC2Client.AcceptVpcEndpointConnections(ctx, &awsec2.AcceptVpcEndpointConnectionsInput{
			ServiceId: aws.String(obj.Status.ServiceID), VpcEndpointIds: pending,
		}); err != nil {
			return fmt.Errorf("accept endpoint connections: %w", err)
		}
		pending = nil
	}
	obj.Status.PendingConnections = int32(len(pending))

	obj.Status.ServiceName = aws.ToString(cfg.ServiceName)
	obj.Status.State = string(cfg.ServiceState)
	obj.Status.AvailabilityZones = cfg.AvailabilityZones
	if cfg.PrivateDnsNameConfiguration != nil {
		obj.Status.PrivateDNSNameVerificationState = string(cfg.PrivateDnsNameConfiguration.State)
		obj.Status.PrivateDNSVerificationRecord = aws.ToString(cfg.PrivateDnsNameConfiguration.Name) + "=" + aws.ToString(cfg.PrivateDnsNameConfiguration.Value)
	}
	now := metav1.Now()
	obj.Status.LastSyncTime = &now

	switch cfg.ServiceState {
	case ec2types.ServiceStateAvailable:
		obj.Status.ObservedGeneration = obj.Generation
		msg := "endpoint service available"
		if len(pending) > 0 {
			msg = fmt.Sprintf("endpoint service available; %d consumer connection(s) pending acceptance", len(pending))
		}
		return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, msg)
	case ec2types.ServiceStatePending:
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonCreated, "endpoint service pending")
		return errPendingAcceptance
	default:
		return fmt.Errorf("endpoint service in state %s", cfg.ServiceState)
	}
}

func (r *VPCEndpointServiceReconciler) deleteService(ctx context.Context, obj *awsv1alpha1.VPCEndpointService) error {
	if obj.Status.ServiceID == "" {
		return nil
	}
	out, err := r.EC2Client.DeleteVpcEndpointServiceConfigurations(ctx, &awsec2.DeleteVpcEndpointServiceConfigurationsInput{
		ServiceIds: []string{obj.Status.ServiceID},
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, u := range out.Unsuccessful {
		if u.Error != nil && ec2helper.IsNotFoundCode(aws.ToString(u.Error.Code)) {
			continue
		}
		return fmt.Errorf("delete endpoint service %s: %s", aws.ToString(u.ResourceId), aws.ToString(u.Error.Message))
	}
	return nil
}

// diffStrings returns the elements to add (in desired, not current) and to
// remove (in current, not desired).
func diffStrings(current, desired []string) (add, remove []string) {
	cur := map[string]bool{}
	for _, c := range current {
		cur[c] = true
	}
	des := map[string]bool{}
	for _, d := range desired {
		des[d] = true
		if !cur[d] {
			add = append(add, d)
		}
	}
	for _, c := range current {
		if !des[c] {
			remove = append(remove, c)
		}
	}
	return add, remove
}

func (r *VPCEndpointServiceReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.VPCEndpointService, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
		Type: condType, Status: status, ObservedGeneration: obj.Generation, Reason: reason, Message: message,
	})
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *VPCEndpointServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.VPCEndpointService{}).
		Complete(r)
}
