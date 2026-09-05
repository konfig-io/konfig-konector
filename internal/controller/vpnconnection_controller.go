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
	"time"

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

// requeueVPNPolling is the requeue interval while a VPN connection is pending.
var requeueVPNPolling = ctrl.Result{RequeueAfter: 30 * time.Second}

// VPNConnectionAWSAPI is the subset of the EC2 API used by this controller.
type VPNConnectionAWSAPI interface {
	DescribeVpnConnections(ctx context.Context, params *awsec2.DescribeVpnConnectionsInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeVpnConnectionsOutput, error)
	CreateVpnConnection(ctx context.Context, params *awsec2.CreateVpnConnectionInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateVpnConnectionOutput, error)
	DeleteVpnConnection(ctx context.Context, params *awsec2.DeleteVpnConnectionInput, optFns ...func(*awsec2.Options)) (*awsec2.DeleteVpnConnectionOutput, error)
	CreateTags(ctx context.Context, params *awsec2.CreateTagsInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateTagsOutput, error)
}

// VPNConnectionReconciler reconciles VPNConnection objects.
type VPNConnectionReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client VPNConnectionAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpnconnections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpnconnections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpnconnections/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *VPNConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.VPNConnection{}
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
			if err := r.deleteVPNConnection(ctx, obj); err != nil {
				logger.Error(err, "failed to delete VPNConnection")
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

	result, err := r.reconcileVPNConnection(ctx, obj)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *VPNConnectionReconciler) reconcileVPNConnection(ctx context.Context, obj *awsv1alpha1.VPNConnection) (ctrl.Result, error) {
	if obj.Status.VPNConnectionID != "" {
		out, err := r.EC2Client.DescribeVpnConnections(ctx, &awsec2.DescribeVpnConnectionsInput{
			VpnConnectionIds: []string{obj.Status.VPNConnectionID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("describe VPN connection: %w", err)
		}
		if err == nil && len(out.VpnConnections) > 0 {
			state := out.VpnConnections[0].State
			obj.Status.State = string(state)
			switch state {
			case ec2types.VpnStateAvailable:
				if len(obj.Spec.Tags) > 0 {
					if _, tagErr := r.EC2Client.CreateTags(ctx, &awsec2.CreateTagsInput{
						Resources: []string{obj.Status.VPNConnectionID},
						Tags:      ec2helper.TagsFromMap(obj.Spec.Tags),
					}); tagErr != nil {
						return ctrl.Result{}, fmt.Errorf("tag VPN connection: %w", tagErr)
					}
				}
				obj.Status.ObservedGeneration = obj.Generation
				now := metav1.Now()
				obj.Status.LastSyncTime = &now
				if err := r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "VPN connection available"); err != nil {
					return ctrl.Result{}, err
				}
				return requeueResult(), nil
			case ec2types.VpnStatePending:
				_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Pending", "VPN connection is pending")
				return requeueVPNPolling, nil
			default:
				// deleted/deleting — recreate on the next pass.
				obj.Status.VPNConnectionID = ""
			}
		} else {
			obj.Status.VPNConnectionID = ""
		}
	}

	cgwID, err := r.resolveCustomerGatewayID(ctx, obj)
	if err != nil {
		return ctrl.Result{}, err
	}

	connType := obj.Spec.Type
	if connType == "" {
		connType = "ipsec.1"
	}
	input := &awsec2.CreateVpnConnectionInput{
		CustomerGatewayId: aws.String(cgwID),
		Type:              aws.String(connType),
		TagSpecifications: []ec2types.TagSpecification{
			{ResourceType: ec2types.ResourceTypeVpnConnection, Tags: ec2helper.TagsFromMap(obj.Spec.Tags)},
		},
	}

	switch {
	case obj.Spec.VPNGatewayRef != nil:
		vgwID, err := r.resolveVPNGatewayID(ctx, obj.Namespace, obj.Spec.VPNGatewayRef)
		if err != nil {
			return ctrl.Result{}, err
		}
		input.VpnGatewayId = aws.String(vgwID)
	case obj.Spec.TransitGatewayRef != nil:
		tgwID, err := r.resolveTransitGatewayIDVPN(ctx, obj.Namespace, obj.Spec.TransitGatewayRef)
		if err != nil {
			return ctrl.Result{}, err
		}
		input.TransitGatewayId = aws.String(tgwID)
	default:
		return ctrl.Result{}, fmt.Errorf("either vpnGatewayRef or transitGatewayRef must be set")
	}

	opts := &ec2types.VpnConnectionOptionsSpecification{}
	optsSet := false
	if obj.Spec.StaticRoutesOnly {
		opts.StaticRoutesOnly = aws.Bool(true)
		optsSet = true
	}
	for _, to := range obj.Spec.TunnelOptions {
		spec := ec2types.VpnTunnelOptionsSpecification{}
		if to.InsideCIDR != "" {
			spec.TunnelInsideCidr = aws.String(to.InsideCIDR)
		}
		if to.PreSharedKeyRef != nil {
			// SECURITY: the PSK is read from the Secret and passed only to the
			// AWS API. It must never appear in status, conditions, or logs.
			psk, err := resolveSecretValue(ctx, r.Client, obj.Namespace, *to.PreSharedKeyRef)
			if err != nil {
				return ctrl.Result{}, err
			}
			spec.PreSharedKey = aws.String(psk)
		}
		opts.TunnelOptions = append(opts.TunnelOptions, spec)
		optsSet = true
	}
	if optsSet {
		input.Options = opts
	}

	out, err := r.EC2Client.CreateVpnConnection(ctx, input)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create VPN connection: %w", err)
	}
	obj.Status.VPNConnectionID = aws.ToString(out.VpnConnection.VpnConnectionId)
	obj.Status.State = string(out.VpnConnection.State)
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist VPN connection ID after create: %w", err)
	}
	_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Pending", "VPN connection is being created")
	return requeueVPNPolling, nil
}

func (r *VPNConnectionReconciler) resolveCustomerGatewayID(ctx context.Context, obj *awsv1alpha1.VPNConnection) (string, error) {
	ref := obj.Spec.CustomerGatewayRef
	if ref.ID != "" {
		return ref.ID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("customerGatewayRef requires name or id")
	}
	cgw := &awsv1alpha1.CustomerGateway{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: obj.Namespace}, cgw); err != nil {
		return "", err
	}
	if cgw.Status.CustomerGatewayID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("CustomerGateway %s/%s has no ID yet", obj.Namespace, ref.Name)}
	}
	return cgw.Status.CustomerGatewayID, nil
}

func (r *VPNConnectionReconciler) resolveVPNGatewayID(ctx context.Context, namespace string, ref *awsv1alpha1.VPNGatewayRef) (string, error) {
	if ref.ID != "" {
		return ref.ID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("vpnGatewayRef requires name or id")
	}
	vgw := &awsv1alpha1.VPNGateway{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, vgw); err != nil {
		return "", err
	}
	if vgw.Status.VPNGatewayID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("VPNGateway %s/%s has no ID yet", namespace, ref.Name)}
	}
	return vgw.Status.VPNGatewayID, nil
}

func (r *VPNConnectionReconciler) resolveTransitGatewayIDVPN(ctx context.Context, namespace string, ref *awsv1alpha1.TransitGatewayRef) (string, error) {
	if ref.TransitGatewayID != "" {
		return ref.TransitGatewayID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("transitGatewayRef requires name or transitGatewayId")
	}
	tgw := &awsv1alpha1.TransitGateway{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, tgw); err != nil {
		return "", err
	}
	if tgw.Status.TransitGatewayID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("TransitGateway %s/%s has no ID yet", namespace, ref.Name)}
	}
	return tgw.Status.TransitGatewayID, nil
}

func (r *VPNConnectionReconciler) deleteVPNConnection(ctx context.Context, obj *awsv1alpha1.VPNConnection) error {
	if obj.Status.VPNConnectionID == "" {
		return nil
	}
	_, err := r.EC2Client.DeleteVpnConnection(ctx, &awsec2.DeleteVpnConnectionInput{
		VpnConnectionId: aws.String(obj.Status.VPNConnectionID),
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *VPNConnectionReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.VPNConnection, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *VPNConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.VPNConnection{}).
		Complete(r)
}
