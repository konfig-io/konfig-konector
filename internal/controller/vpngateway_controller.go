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

// VPNGatewayAWSAPI is the subset of the EC2 API used by this controller.
type VPNGatewayAWSAPI interface {
	DescribeVpnGateways(ctx context.Context, params *awsec2.DescribeVpnGatewaysInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeVpnGatewaysOutput, error)
	CreateVpnGateway(ctx context.Context, params *awsec2.CreateVpnGatewayInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateVpnGatewayOutput, error)
	DeleteVpnGateway(ctx context.Context, params *awsec2.DeleteVpnGatewayInput, optFns ...func(*awsec2.Options)) (*awsec2.DeleteVpnGatewayOutput, error)
	AttachVpnGateway(ctx context.Context, params *awsec2.AttachVpnGatewayInput, optFns ...func(*awsec2.Options)) (*awsec2.AttachVpnGatewayOutput, error)
	DetachVpnGateway(ctx context.Context, params *awsec2.DetachVpnGatewayInput, optFns ...func(*awsec2.Options)) (*awsec2.DetachVpnGatewayOutput, error)
	CreateTags(ctx context.Context, params *awsec2.CreateTagsInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateTagsOutput, error)
}

// VPNGatewayReconciler reconciles VPNGateway objects.
type VPNGatewayReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client VPNGatewayAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpngateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpngateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpngateways/finalizers,verbs=update

func (r *VPNGatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.VPNGateway{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
			if shouldAbandon(obj) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, obj)
			}
			if err := r.deleteVPNGateway(ctx, obj); err != nil {
				logger.Error(err, "failed to delete VPNGateway")
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

	if err := r.reconcileVPNGateway(ctx, obj); err != nil {
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

func (r *VPNGatewayReconciler) reconcileVPNGateway(ctx context.Context, obj *awsv1alpha1.VPNGateway) error {
	// Resolve the desired VPC (if any) before mutating anything.
	var desiredVPCID string
	if obj.Spec.VPCRef != nil {
		id, err := r.resolveVPCIDVGW(ctx, obj.Namespace, obj.Spec.VPCRef)
		if err != nil {
			return err
		}
		desiredVPCID = id
	}

	var gw *ec2types.VpnGateway
	if obj.Status.VPNGatewayID != "" {
		out, err := r.EC2Client.DescribeVpnGateways(ctx, &awsec2.DescribeVpnGatewaysInput{
			VpnGatewayIds: []string{obj.Status.VPNGatewayID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return fmt.Errorf("describe VPN gateway: %w", err)
		}
		if err == nil && len(out.VpnGateways) > 0 && out.VpnGateways[0].State != ec2types.VpnStateDeleted {
			gw = &out.VpnGateways[0]
		} else {
			obj.Status.VPNGatewayID = ""
			obj.Status.AttachedVPCID = ""
		}
	}

	if gw == nil {
		gwType := obj.Spec.Type
		if gwType == "" {
			gwType = "ipsec.1"
		}
		input := &awsec2.CreateVpnGatewayInput{
			Type: ec2types.GatewayType(gwType),
			TagSpecifications: []ec2types.TagSpecification{
				{ResourceType: ec2types.ResourceTypeVpnGateway, Tags: ec2helper.TagsFromMap(obj.Spec.Tags)},
			},
		}
		if obj.Spec.AmazonSideASN > 0 {
			input.AmazonSideAsn = aws.Int64(obj.Spec.AmazonSideASN)
		}
		out, err := r.EC2Client.CreateVpnGateway(ctx, input)
		if err != nil {
			return fmt.Errorf("create VPN gateway: %w", err)
		}
		obj.Status.VPNGatewayID = aws.ToString(out.VpnGateway.VpnGatewayId)
		obj.Status.State = string(out.VpnGateway.State)
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return fmt.Errorf("persist VPN gateway ID after create: %w", err)
		}
		gw = out.VpnGateway
	} else {
		obj.Status.State = string(gw.State)
		if len(obj.Spec.Tags) > 0 {
			if _, err := r.EC2Client.CreateTags(ctx, &awsec2.CreateTagsInput{
				Resources: []string{obj.Status.VPNGatewayID},
				Tags:      ec2helper.TagsFromMap(obj.Spec.Tags),
			}); err != nil {
				return fmt.Errorf("tag VPN gateway: %w", err)
			}
		}
	}

	// Reconcile the VPC attachment.
	currentVPCID := ""
	for _, att := range gw.VpcAttachments {
		if att.State == ec2types.AttachmentStatusAttached || att.State == ec2types.AttachmentStatusAttaching {
			currentVPCID = aws.ToString(att.VpcId)
			break
		}
	}
	if desiredVPCID != currentVPCID {
		if currentVPCID != "" {
			if _, err := r.EC2Client.DetachVpnGateway(ctx, &awsec2.DetachVpnGatewayInput{
				VpnGatewayId: aws.String(obj.Status.VPNGatewayID),
				VpcId:        aws.String(currentVPCID),
			}); err != nil && !ec2helper.IsNotFound(err) {
				return fmt.Errorf("detach VPN gateway: %w", err)
			}
		}
		if desiredVPCID != "" {
			if _, err := r.EC2Client.AttachVpnGateway(ctx, &awsec2.AttachVpnGatewayInput{
				VpnGatewayId: aws.String(obj.Status.VPNGatewayID),
				VpcId:        aws.String(desiredVPCID),
			}); err != nil {
				return fmt.Errorf("attach VPN gateway: %w", err)
			}
		}
	}
	obj.Status.AttachedVPCID = desiredVPCID

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "VPNGateway reconciled")
}

func (r *VPNGatewayReconciler) resolveVPCIDVGW(ctx context.Context, namespace string, ref *awsv1alpha1.VPCResourceRef) (string, error) {
	if ref.ID != "" {
		return ref.ID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("vpcRef requires name or id")
	}
	vpc := &awsv1alpha1.VPC{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, vpc); err != nil {
		return "", err
	}
	if vpc.Status.VPCID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("VPC %s/%s has no vpcId yet", namespace, ref.Name)}
	}
	return vpc.Status.VPCID, nil
}

func (r *VPNGatewayReconciler) deleteVPNGateway(ctx context.Context, obj *awsv1alpha1.VPNGateway) error {
	if obj.Status.VPNGatewayID == "" {
		return nil
	}
	// Detach from the VPC first if attached.
	out, err := r.EC2Client.DescribeVpnGateways(ctx, &awsec2.DescribeVpnGatewaysInput{
		VpnGatewayIds: []string{obj.Status.VPNGatewayID},
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(out.VpnGateways) > 0 {
		for _, att := range out.VpnGateways[0].VpcAttachments {
			if att.State == ec2types.AttachmentStatusAttached {
				if _, err := r.EC2Client.DetachVpnGateway(ctx, &awsec2.DetachVpnGatewayInput{
					VpnGatewayId: aws.String(obj.Status.VPNGatewayID),
					VpcId:        att.VpcId,
				}); err != nil && !ec2helper.IsNotFound(err) {
					return fmt.Errorf("detach VPN gateway before delete: %w", err)
				}
			}
		}
	}
	_, err = r.EC2Client.DeleteVpnGateway(ctx, &awsec2.DeleteVpnGatewayInput{
		VpnGatewayId: aws.String(obj.Status.VPNGatewayID),
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *VPNGatewayReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.VPNGateway, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *VPNGatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.VPNGateway{}).
		Complete(r)
}
