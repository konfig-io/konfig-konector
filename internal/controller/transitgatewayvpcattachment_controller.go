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

// TransitGatewayVpcAttachmentReconciler reconciles TransitGatewayVpcAttachment objects.
type TransitGatewayVpcAttachmentReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client *awsec2.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=transitgatewayvpcattachments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=transitgatewayvpcattachments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=transitgatewayvpcattachments/finalizers,verbs=update

func (r *TransitGatewayVpcAttachmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	att := &awsv1alpha1.TransitGatewayVpcAttachment{}
	if err := r.Get(ctx, req.NamespacedName, att); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !att.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(att, awsv1alpha1.FinalizerName) {
			if shouldAbandon(att) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(att, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, att)
			}
			if err := r.deleteAttachment(ctx, att); err != nil {
				logger.Error(err, "failed to delete TransitGatewayVpcAttachment")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(att, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, att)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(att, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(att, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, att); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileAttachment(ctx, att); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionTGWAtt(ctx, att, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *TransitGatewayVpcAttachmentReconciler) reconcileAttachment(ctx context.Context, att *awsv1alpha1.TransitGatewayVpcAttachment) error {
	tgwID, err := r.resolveTGWID(ctx, att)
	if err != nil {
		return err
	}
	vpcID, err := r.resolveVPCIDTGW(ctx, att.Namespace, &att.Spec.VPCRef)
	if err != nil {
		return err
	}
	subnetIDs, err := resolveSubnetIDs(ctx, r.Client, att.Namespace, att.Spec.SubnetRefs)
	if err != nil {
		return err
	}

	if att.Status.AttachmentID != "" {
		out, err := r.EC2Client.DescribeTransitGatewayVpcAttachments(ctx, &awsec2.DescribeTransitGatewayVpcAttachmentsInput{
			TransitGatewayAttachmentIds: []string{att.Status.AttachmentID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return fmt.Errorf("describe TGW attachment: %w", err)
		}
		if err == nil && len(out.TransitGatewayVpcAttachments) > 0 {
			att.Status.State = string(out.TransitGatewayVpcAttachments[0].State)
			if len(att.Spec.Tags) > 0 {
				if _, tagErr := r.EC2Client.CreateTags(ctx, &awsec2.CreateTagsInput{
					Resources: []string{att.Status.AttachmentID},
					Tags:      ec2helper.TagsFromMap(att.Spec.Tags),
				}); tagErr != nil {
					return fmt.Errorf("tag TGW attachment: %w", tagErr)
				}
			}
			att.Status.ObservedGeneration = att.Generation
			now := metav1.Now()
			att.Status.LastSyncTime = &now
			return r.setConditionTGWAtt(ctx, att, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "TransitGatewayVpcAttachment reconciled")
		}
		att.Status.AttachmentID = ""
	}

	opts := &ec2types.CreateTransitGatewayVpcAttachmentRequestOptions{}
	if att.Spec.DNSSupport {
		opts.DnsSupport = ec2types.DnsSupportValueEnable
	}
	if att.Spec.IPv6Support {
		opts.Ipv6Support = ec2types.Ipv6SupportValueEnable
	}
	if att.Spec.ApplianceModeSupport {
		opts.ApplianceModeSupport = ec2types.ApplianceModeSupportValueEnable
	}

	createOut, err := r.EC2Client.CreateTransitGatewayVpcAttachment(ctx, &awsec2.CreateTransitGatewayVpcAttachmentInput{
		TransitGatewayId: aws.String(tgwID),
		VpcId:            aws.String(vpcID),
		SubnetIds:        subnetIDs,
		Options:          opts,
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeTransitGatewayAttachment,
				Tags:         ec2helper.TagsFromMap(att.Spec.Tags),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("create TGW VPC attachment: %w", err)
	}

	att.Status.AttachmentID = aws.ToString(createOut.TransitGatewayVpcAttachment.TransitGatewayAttachmentId)
	att.Status.State = string(createOut.TransitGatewayVpcAttachment.State)
	if err := persistStatus(ctx, r.Client, att); err != nil {
		return fmt.Errorf("persist TGW attachment ID after create: %w", err)
	}
	att.Status.ObservedGeneration = att.Generation
	now := metav1.Now()
	att.Status.LastSyncTime = &now
	return r.setConditionTGWAtt(ctx, att, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "TransitGatewayVpcAttachment created")
}

func (r *TransitGatewayVpcAttachmentReconciler) resolveTGWID(ctx context.Context, att *awsv1alpha1.TransitGatewayVpcAttachment) (string, error) {
	ref := att.Spec.TransitGatewayRef
	if ref.TransitGatewayID != "" {
		return ref.TransitGatewayID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("transitGatewayRef requires name or transitGatewayId")
	}
	tgw := &awsv1alpha1.TransitGateway{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: att.Namespace}, tgw); err != nil {
		return "", err
	}
	if tgw.Status.TransitGatewayID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("TransitGateway %s/%s has no ID yet", att.Namespace, ref.Name)}
	}
	return tgw.Status.TransitGatewayID, nil
}

func (r *TransitGatewayVpcAttachmentReconciler) resolveVPCIDTGW(ctx context.Context, namespace string, ref *awsv1alpha1.VPCResourceRef) (string, error) {
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

func (r *TransitGatewayVpcAttachmentReconciler) deleteAttachment(ctx context.Context, att *awsv1alpha1.TransitGatewayVpcAttachment) error {
	attachmentID := att.Status.AttachmentID
	if attachmentID == "" {
		// Fallback: the status write may have been lost after create.
		// The TGW + VPC pair is deterministic from spec, so look up the
		// attachment by those filters when the refs still resolve.
		tgwID, err := r.resolveTGWID(ctx, att)
		if err != nil {
			return nil
		}
		vpcID, err := r.resolveVPCIDTGW(ctx, att.Namespace, &att.Spec.VPCRef)
		if err != nil {
			return nil
		}
		out, err := r.EC2Client.DescribeTransitGatewayVpcAttachments(ctx, &awsec2.DescribeTransitGatewayVpcAttachmentsInput{
			Filters: []ec2types.Filter{
				{Name: aws.String("transit-gateway-id"), Values: []string{tgwID}},
				{Name: aws.String("vpc-id"), Values: []string{vpcID}},
				{Name: aws.String("state"), Values: []string{"pending", "available", "modifying"}},
			},
		})
		if err != nil {
			return fmt.Errorf("lookup TGW attachment by tgw/vpc: %w", err)
		}
		if len(out.TransitGatewayVpcAttachments) != 1 {
			return nil
		}
		attachmentID = aws.ToString(out.TransitGatewayVpcAttachments[0].TransitGatewayAttachmentId)
	}
	_, err := r.EC2Client.DeleteTransitGatewayVpcAttachment(ctx, &awsec2.DeleteTransitGatewayVpcAttachmentInput{
		TransitGatewayAttachmentId: aws.String(attachmentID),
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *TransitGatewayVpcAttachmentReconciler) setConditionTGWAtt(ctx context.Context, att *awsv1alpha1.TransitGatewayVpcAttachment, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&att.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: att.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, att); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *TransitGatewayVpcAttachmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.TransitGatewayVpcAttachment{}).
		Complete(r)
}
