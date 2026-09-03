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
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ec2helper "github.com/konfig-io/konfig-konector/internal/aws/ec2"
)

// VPCAWSAPI is the subset of the EC2 client used by the VPC controller.
// *ec2.Client satisfies it.
type VPCAWSAPI interface {
	CreateVpc(ctx context.Context, params *awsec2.CreateVpcInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateVpcOutput, error)
	DeleteVpc(ctx context.Context, params *awsec2.DeleteVpcInput, optFns ...func(*awsec2.Options)) (*awsec2.DeleteVpcOutput, error)
	DescribeVpcs(ctx context.Context, params *awsec2.DescribeVpcsInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeVpcsOutput, error)
	ModifyVpcAttribute(ctx context.Context, params *awsec2.ModifyVpcAttributeInput, optFns ...func(*awsec2.Options)) (*awsec2.ModifyVpcAttributeOutput, error)
	AssociateVpcCidrBlock(ctx context.Context, params *awsec2.AssociateVpcCidrBlockInput, optFns ...func(*awsec2.Options)) (*awsec2.AssociateVpcCidrBlockOutput, error)
	DisassociateVpcCidrBlock(ctx context.Context, params *awsec2.DisassociateVpcCidrBlockInput, optFns ...func(*awsec2.Options)) (*awsec2.DisassociateVpcCidrBlockOutput, error)
	CreateTags(ctx context.Context, params *awsec2.CreateTagsInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateTagsOutput, error)
	DeleteTags(ctx context.Context, params *awsec2.DeleteTagsInput, optFns ...func(*awsec2.Options)) (*awsec2.DeleteTagsOutput, error)
	DescribeTags(ctx context.Context, params *awsec2.DescribeTagsInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeTagsOutput, error)
}

// VPCReconciler reconciles VPC objects.
type VPCReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client VPCAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpcs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpcs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpcs/finalizers,verbs=update

func (r *VPCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	vpc := &awsv1alpha1.VPC{}
	if err := r.Get(ctx, req.NamespacedName, vpc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !vpc.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(vpc, awsv1alpha1.FinalizerName) {
			if shouldAbandon(vpc) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(vpc, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, vpc)
			}
			vpcID := vpc.Status.VPCID
			if vpcID == "" {
				// Fallback: the status write may have been lost after create.
				// Look up by CIDR block (plus Name tag when the create path set one).
				id, err := r.findVPCID(ctx, vpc)
				if err != nil {
					return ctrl.Result{}, err
				}
				vpcID = id
			}
			if vpcID != "" {
				if _, err := r.EC2Client.DeleteVpc(ctx, &awsec2.DeleteVpcInput{
					VpcId: aws.String(vpcID),
				}); err != nil && !ec2helper.IsNotFound(err) {
					logger.Error(err, "failed to delete VPC", "vpcId", vpcID)
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(vpc, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, vpc)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(vpc, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(vpc, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, vpc); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileVPC(ctx, vpc); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, vpc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *VPCReconciler) reconcileVPC(ctx context.Context, vpc *awsv1alpha1.VPC) error {
	var vpcID string

	if vpc.Status.VPCID != "" {
		existing, err := ec2helper.GetVPC(ctx, r.EC2Client, vpc.Status.VPCID)
		if err != nil {
			return err
		}
		if existing != nil {
			vpcID = vpc.Status.VPCID
			vpc.Status.State = string(existing.State)
		}
	}

	if vpcID == "" {
		createIn := &awsec2.CreateVpcInput{
			CidrBlock: aws.String(vpc.Spec.CIDRBlock),
			TagSpecifications: []types.TagSpecification{
				{ResourceType: types.ResourceTypeVpc, Tags: ec2helper.TagsFromMap(vpc.Spec.Tags)},
			},
		}
		if vpc.Spec.InstanceTenancy != "" {
			createIn.InstanceTenancy = types.Tenancy(vpc.Spec.InstanceTenancy)
		}
		out, err := r.EC2Client.CreateVpc(ctx, createIn)
		if err != nil {
			return fmt.Errorf("create VPC: %w", err)
		}
		vpcID = aws.ToString(out.Vpc.VpcId)
		vpc.Status.State = string(out.Vpc.State)
		vpc.Status.VPCID = vpcID
		if err := persistStatus(ctx, r.Client, vpc); err != nil {
			return fmt.Errorf("persist VPC ID after create: %w", err)
		}
	}

	vpc.Status.VPCID = vpcID

	// Sync DNS attributes.
	enableDNSSupport := true
	if vpc.Spec.EnableDNSSupport != nil {
		enableDNSSupport = *vpc.Spec.EnableDNSSupport
	}
	if _, err := r.EC2Client.ModifyVpcAttribute(ctx, &awsec2.ModifyVpcAttributeInput{
		VpcId:            aws.String(vpcID),
		EnableDnsSupport: &types.AttributeBooleanValue{Value: aws.Bool(enableDNSSupport)},
	}); err != nil {
		return fmt.Errorf("modify VPC DNS support: %w", err)
	}

	if vpc.Spec.EnableDNSHostnames != nil {
		if _, err := r.EC2Client.ModifyVpcAttribute(ctx, &awsec2.ModifyVpcAttributeInput{
			VpcId:              aws.String(vpcID),
			EnableDnsHostnames: &types.AttributeBooleanValue{Value: vpc.Spec.EnableDNSHostnames},
		}); err != nil {
			return fmt.Errorf("modify VPC DNS hostnames: %w", err)
		}
	}

	// Sync IPv6 CIDR.
	if vpc.Spec.AssignIpv6CidrBlock && vpc.Status.Ipv6CidrBlock == "" {
		out, err := r.EC2Client.AssociateVpcCidrBlock(ctx, &awsec2.AssociateVpcCidrBlockInput{
			VpcId:                       aws.String(vpcID),
			AmazonProvidedIpv6CidrBlock: aws.Bool(true),
		})
		if err != nil {
			return fmt.Errorf("associate IPv6 CIDR: %w", err)
		}
		if out.Ipv6CidrBlockAssociation != nil && out.Ipv6CidrBlockAssociation.Ipv6CidrBlock != nil {
			vpc.Status.Ipv6CidrBlock = *out.Ipv6CidrBlockAssociation.Ipv6CidrBlock
		}
	}

	// Sync secondary IPv4 CIDRs (no length guard — always run so removals are applied).
	{
		existing, err := ec2helper.GetVPC(ctx, r.EC2Client, vpcID)
		if err != nil {
			return fmt.Errorf("describe VPC for CIDR sync: %w", err)
		}
		existingCIDRs := make(map[string]string) // cidr -> associationID
		for _, assoc := range existing.CidrBlockAssociationSet {
			if assoc.CidrBlock != nil && aws.ToString(assoc.CidrBlock) != vpc.Spec.CIDRBlock {
				state := ""
				if assoc.CidrBlockState != nil {
					state = string(assoc.CidrBlockState.State)
				}
				if state != "disassociating" && state != "disassociated" {
					existingCIDRs[aws.ToString(assoc.CidrBlock)] = aws.ToString(assoc.AssociationId)
				}
			}
		}
		for _, cidr := range vpc.Spec.SecondaryIPv4CIDRs {
			if _, ok := existingCIDRs[cidr]; !ok {
				if _, err := r.EC2Client.AssociateVpcCidrBlock(ctx, &awsec2.AssociateVpcCidrBlockInput{
					VpcId:     aws.String(vpcID),
					CidrBlock: aws.String(cidr),
				}); err != nil {
					return fmt.Errorf("associate secondary CIDR %s: %w", cidr, err)
				}
			}
			delete(existingCIDRs, cidr)
		}
		for _, assocID := range existingCIDRs {
			if _, err := r.EC2Client.DisassociateVpcCidrBlock(ctx, &awsec2.DisassociateVpcCidrBlockInput{
				AssociationId: aws.String(assocID),
			}); err != nil {
				return fmt.Errorf("disassociate CIDR %s: %w", assocID, err)
			}
		}
	}

	// Sync tags.
	if err := ec2helper.SyncResourceTags(ctx, r.EC2Client, vpcID, "vpc", vpc.Spec.Tags); err != nil {
		return fmt.Errorf("sync tags: %w", err)
	}

	vpc.Status.ObservedGeneration = vpc.Generation
	now := metav1.Now()
	vpc.Status.LastSyncTime = &now
	return r.setCondition(ctx, vpc, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "VPC reconciled")
}

// findVPCID looks up a VPC created by this CR when the status ID was lost
// (e.g. a failed status write after CreateVpc). It matches on the spec CIDR
// block plus all tags the create path applied. Returns "" when there is not
// exactly one match, so ambiguous lookups never delete the wrong VPC.
func (r *VPCReconciler) findVPCID(ctx context.Context, vpc *awsv1alpha1.VPC) (string, error) {
	filters := []types.Filter{
		{Name: aws.String("cidr-block-association.cidr-block"), Values: []string{vpc.Spec.CIDRBlock}},
	}
	for k, v := range vpc.Spec.Tags {
		filters = append(filters, types.Filter{Name: aws.String("tag:" + k), Values: []string{v}})
	}
	out, err := r.EC2Client.DescribeVpcs(ctx, &awsec2.DescribeVpcsInput{Filters: filters})
	if err != nil {
		return "", fmt.Errorf("lookup VPC by CIDR/tags: %w", err)
	}
	if len(out.Vpcs) != 1 {
		return "", nil
	}
	return aws.ToString(out.Vpcs[0].VpcId), nil
}

func (r *VPCReconciler) setCondition(ctx context.Context, vpc *awsv1alpha1.VPC, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&vpc.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: vpc.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, vpc); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *VPCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.VPC{}).
		Complete(r)
}
