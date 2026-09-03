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

// NetworkACLReconciler reconciles NetworkACL objects.
type NetworkACLReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client *awsec2.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=networkacls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=networkacls/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=networkacls/finalizers,verbs=update

func (r *NetworkACLReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	nacl := &awsv1alpha1.NetworkACL{}
	if err := r.Get(ctx, req.NamespacedName, nacl); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !nacl.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(nacl, awsv1alpha1.FinalizerName) {
			if shouldAbandon(nacl) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(nacl, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, nacl)
			}
			if err := r.deleteNetworkACL(ctx, nacl); err != nil {
				logger.Error(err, "failed to delete NetworkACL")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(nacl, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, nacl)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(nacl, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(nacl, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, nacl); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileNetworkACL(ctx, nacl); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionNACL(ctx, nacl, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *NetworkACLReconciler) reconcileNetworkACL(ctx context.Context, nacl *awsv1alpha1.NetworkACL) error {
	vpcID, err := r.resolveVPCID(ctx, nacl.Namespace, &nacl.Spec.VPCRef)
	if err != nil {
		return err
	}

	var naclID string
	if nacl.Status.NetworkACLID != "" {
		// Verify it still exists.
		out, err := r.EC2Client.DescribeNetworkAcls(ctx, &awsec2.DescribeNetworkAclsInput{
			NetworkAclIds: []string{nacl.Status.NetworkACLID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return fmt.Errorf("describe network ACL: %w", err)
		}
		if err == nil && len(out.NetworkAcls) > 0 {
			naclID = nacl.Status.NetworkACLID
		}
	}

	if naclID == "" {
		createOut, err := r.EC2Client.CreateNetworkAcl(ctx, &awsec2.CreateNetworkAclInput{
			VpcId: aws.String(vpcID),
			TagSpecifications: []ec2types.TagSpecification{
				{
					ResourceType: ec2types.ResourceTypeNetworkAcl,
					Tags:         ec2helper.TagsFromMap(nacl.Spec.Tags),
				},
			},
		})
		if err != nil {
			return fmt.Errorf("create network ACL: %w", err)
		}
		naclID = aws.ToString(createOut.NetworkAcl.NetworkAclId)
		nacl.Status.NetworkACLID = naclID
		if err := persistStatus(ctx, r.Client, nacl); err != nil {
			return fmt.Errorf("persist network ACL ID after create: %w", err)
		}
	} else {
		// Sync tags.
		if len(nacl.Spec.Tags) > 0 {
			_, err := r.EC2Client.CreateTags(ctx, &awsec2.CreateTagsInput{
				Resources: []string{naclID},
				Tags:      ec2helper.TagsFromMap(nacl.Spec.Tags),
			})
			if err != nil {
				return fmt.Errorf("tag network ACL: %w", err)
			}
		}
	}

	// Sync entries: delete existing non-default entries then re-create.
	if err := r.syncNACLEntries(ctx, naclID, nacl.Spec.Entries); err != nil {
		return err
	}

	nacl.Status.NetworkACLID = naclID
	nacl.Status.ObservedGeneration = nacl.Generation
	now := metav1.Now()
	nacl.Status.LastSyncTime = &now
	return r.setConditionNACL(ctx, nacl, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "NetworkACL reconciled")
}

func (r *NetworkACLReconciler) syncNACLEntries(ctx context.Context, naclID string, desired []awsv1alpha1.NetworkACLEntry) error {
	// Describe current entries.
	out, err := r.EC2Client.DescribeNetworkAcls(ctx, &awsec2.DescribeNetworkAclsInput{
		NetworkAclIds: []string{naclID},
	})
	if err != nil {
		return fmt.Errorf("describe NACL entries: %w", err)
	}
	if len(out.NetworkAcls) == 0 {
		return nil
	}

	// Delete all non-default entries (default rules have rule number 32767 or 32768).
	for _, entry := range out.NetworkAcls[0].Entries {
		if aws.ToInt32(entry.RuleNumber) >= 32767 {
			continue
		}
		if _, err := r.EC2Client.DeleteNetworkAclEntry(ctx, &awsec2.DeleteNetworkAclEntryInput{
			NetworkAclId: aws.String(naclID),
			RuleNumber:   entry.RuleNumber,
			Egress:       entry.Egress,
		}); err != nil {
			return fmt.Errorf("delete NACL entry: %w", err)
		}
	}

	// Create desired entries.
	for _, e := range desired {
		input := &awsec2.CreateNetworkAclEntryInput{
			NetworkAclId: aws.String(naclID),
			RuleNumber:   aws.Int32(e.RuleNumber),
			Protocol:     aws.String(e.Protocol),
			RuleAction:   ec2types.RuleAction(e.RuleAction),
			Egress:       aws.Bool(e.Egress),
		}
		if e.CIDRBlock != "" {
			input.CidrBlock = aws.String(e.CIDRBlock)
		}
		if e.IPv6CIDRBlock != "" {
			input.Ipv6CidrBlock = aws.String(e.IPv6CIDRBlock)
		}
		if e.PortRange != nil {
			input.PortRange = &ec2types.PortRange{
				From: aws.Int32(e.PortRange.From),
				To:   aws.Int32(e.PortRange.To),
			}
		}
		if e.ICMPTypeCode != nil {
			input.IcmpTypeCode = &ec2types.IcmpTypeCode{
				Type: aws.Int32(e.ICMPTypeCode.Type),
				Code: aws.Int32(e.ICMPTypeCode.Code),
			}
		}
		if _, err := r.EC2Client.CreateNetworkAclEntry(ctx, input); err != nil {
			return fmt.Errorf("create NACL entry rule %d: %w", e.RuleNumber, err)
		}
	}
	return nil
}

func (r *NetworkACLReconciler) deleteNetworkACL(ctx context.Context, nacl *awsv1alpha1.NetworkACL) error {
	naclID := nacl.Status.NetworkACLID
	if naclID == "" {
		// Fallback: the status write may have been lost after create.
		// Look up by the tags the create path applied, scoped to the VPC
		// when resolvable; without tags the ACL is indistinguishable, so give up.
		if len(nacl.Spec.Tags) == 0 {
			return nil
		}
		filters := make([]ec2types.Filter, 0, len(nacl.Spec.Tags)+1)
		if vpcID, err := r.resolveVPCID(ctx, nacl.Namespace, &nacl.Spec.VPCRef); err == nil && vpcID != "" {
			filters = append(filters, ec2types.Filter{Name: aws.String("vpc-id"), Values: []string{vpcID}})
		}
		for k, v := range nacl.Spec.Tags {
			filters = append(filters, ec2types.Filter{Name: aws.String("tag:" + k), Values: []string{v}})
		}
		out, err := r.EC2Client.DescribeNetworkAcls(ctx, &awsec2.DescribeNetworkAclsInput{Filters: filters})
		if err != nil {
			return fmt.Errorf("lookup network ACL by tags: %w", err)
		}
		if len(out.NetworkAcls) != 1 {
			return nil
		}
		naclID = aws.ToString(out.NetworkAcls[0].NetworkAclId)
	}
	_, err := r.EC2Client.DeleteNetworkAcl(ctx, &awsec2.DeleteNetworkAclInput{
		NetworkAclId: aws.String(naclID),
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *NetworkACLReconciler) resolveVPCID(ctx context.Context, namespace string, ref *awsv1alpha1.VPCResourceRef) (string, error) {
	if ref.ID != "" {
		return ref.ID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("vpcRef requires either name or id")
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

func (r *NetworkACLReconciler) setConditionNACL(ctx context.Context, nacl *awsv1alpha1.NetworkACL, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&nacl.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: nacl.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, nacl); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *NetworkACLReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.NetworkACL{}).
		Complete(r)
}
