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
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
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

// SecurityGroupAWSAPI is the subset of the EC2 client used by the
// SecurityGroup controller. *ec2.Client satisfies it.
type SecurityGroupAWSAPI interface {
	CreateSecurityGroup(ctx context.Context, params *awsec2.CreateSecurityGroupInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateSecurityGroupOutput, error)
	DeleteSecurityGroup(ctx context.Context, params *awsec2.DeleteSecurityGroupInput, optFns ...func(*awsec2.Options)) (*awsec2.DeleteSecurityGroupOutput, error)
	DescribeSecurityGroups(ctx context.Context, params *awsec2.DescribeSecurityGroupsInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeSecurityGroupsOutput, error)
	AuthorizeSecurityGroupIngress(ctx context.Context, params *awsec2.AuthorizeSecurityGroupIngressInput, optFns ...func(*awsec2.Options)) (*awsec2.AuthorizeSecurityGroupIngressOutput, error)
	RevokeSecurityGroupIngress(ctx context.Context, params *awsec2.RevokeSecurityGroupIngressInput, optFns ...func(*awsec2.Options)) (*awsec2.RevokeSecurityGroupIngressOutput, error)
	AuthorizeSecurityGroupEgress(ctx context.Context, params *awsec2.AuthorizeSecurityGroupEgressInput, optFns ...func(*awsec2.Options)) (*awsec2.AuthorizeSecurityGroupEgressOutput, error)
	RevokeSecurityGroupEgress(ctx context.Context, params *awsec2.RevokeSecurityGroupEgressInput, optFns ...func(*awsec2.Options)) (*awsec2.RevokeSecurityGroupEgressOutput, error)
	CreateTags(ctx context.Context, params *awsec2.CreateTagsInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateTagsOutput, error)
	DeleteTags(ctx context.Context, params *awsec2.DeleteTagsInput, optFns ...func(*awsec2.Options)) (*awsec2.DeleteTagsOutput, error)
	DescribeTags(ctx context.Context, params *awsec2.DescribeTagsInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeTagsOutput, error)
}

// SecurityGroupReconciler reconciles SecurityGroup objects.
type SecurityGroupReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client SecurityGroupAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=securitygroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=securitygroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=securitygroups/finalizers,verbs=update

func (r *SecurityGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	sg := &awsv1alpha1.SecurityGroup{}
	if err := r.Get(ctx, req.NamespacedName, sg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, sg); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !sg.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(sg, awsv1alpha1.FinalizerName) {
			if shouldAbandon(sg) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(sg, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, sg)
			}
			groupID := sg.Status.GroupID
			if groupID == "" {
				// Fallback: the status write may have been lost after create.
				// Look up by GroupName scoped to the VPC when resolvable.
				id, err := r.findGroupID(ctx, sg)
				if err != nil {
					return ctrl.Result{}, err
				}
				groupID = id
			}
			if groupID != "" {
				if _, err := r.EC2Client.DeleteSecurityGroup(ctx, &awsec2.DeleteSecurityGroupInput{
					GroupId: aws.String(groupID),
				}); err != nil && !ec2helper.IsNotFound(err) {
					logger.Error(err, "failed to delete security group", "sgId", groupID)
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(sg, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, sg)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(sg, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(sg, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, sg); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileSG(ctx, sg); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, sg, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *SecurityGroupReconciler) reconcileSG(ctx context.Context, sg *awsv1alpha1.SecurityGroup) error {
	vpcID, err := r.resolveVPCID(ctx, sg.Namespace, &sg.Spec.VPCRef)
	if err != nil {
		return err
	}

	groupID := sg.Status.GroupID
	if groupID != "" {
		out, err := r.EC2Client.DescribeSecurityGroups(ctx, &awsec2.DescribeSecurityGroupsInput{
			GroupIds: []string{groupID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return err
		}
		if err != nil || len(out.SecurityGroups) == 0 {
			groupID = ""
		}
	}

	created := false
	if groupID == "" {
		out, err := r.EC2Client.CreateSecurityGroup(ctx, &awsec2.CreateSecurityGroupInput{
			GroupName:   aws.String(sg.Spec.GroupName),
			Description: aws.String(sg.Spec.Description),
			VpcId:       aws.String(vpcID),
			TagSpecifications: []types.TagSpecification{
				{ResourceType: types.ResourceTypeSecurityGroup, Tags: ec2helper.TagsFromMap(sg.Spec.Tags)},
			},
		})
		if err != nil {
			return fmt.Errorf("create security group: %w", err)
		}
		groupID = aws.ToString(out.GroupId)
		sg.Status.GroupID = groupID
		if err := persistStatus(ctx, r.Client, sg); err != nil {
			return fmt.Errorf("persist security group ID after create: %w", err)
		}
		created = true
	}

	sg.Status.GroupID = groupID

	// Only churn rules when the spec changed (or the group was just created);
	// steady-state resyncs skip the revoke/reauthorize cycle so existing
	// rules aren't momentarily dropped.
	if created || sg.Status.ObservedGeneration != sg.Generation {
		// Sync ingress rules.
		if err := r.syncIngressRules(ctx, sg, groupID); err != nil {
			return err
		}

		// Sync egress rules.
		if err := r.syncEgressRules(ctx, sg, groupID); err != nil {
			return err
		}
	}

	if err := ec2helper.SyncResourceTags(ctx, r.EC2Client, groupID, "security-group", sg.Spec.Tags); err != nil {
		return fmt.Errorf("sync tags: %w", err)
	}

	sg.Status.ObservedGeneration = sg.Generation
	now := metav1.Now()
	sg.Status.LastSyncTime = &now
	return r.setCondition(ctx, sg, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "security group reconciled")
}

func (r *SecurityGroupReconciler) syncIngressRules(ctx context.Context, sg *awsv1alpha1.SecurityGroup, groupID string) error {
	desired, err := r.buildIPPermissions(ctx, sg.Namespace, sg.Spec.IngressRules)
	if err != nil {
		return err
	}

	out, err := r.EC2Client.DescribeSecurityGroups(ctx, &awsec2.DescribeSecurityGroupsInput{
		GroupIds: []string{groupID},
	})
	if err != nil {
		return err
	}
	if len(out.SecurityGroups) == 0 {
		return nil
	}

	existing := out.SecurityGroups[0].IpPermissions

	// Revoke all existing then re-authorize (simplest approach for drift correction).
	// For production, a diff would be more efficient.
	if len(existing) > 0 {
		if _, err := r.EC2Client.RevokeSecurityGroupIngress(ctx, &awsec2.RevokeSecurityGroupIngressInput{
			GroupId:       aws.String(groupID),
			IpPermissions: existing,
		}); err != nil {
			return fmt.Errorf("revoke ingress: %w", err)
		}
	}

	if len(desired) > 0 {
		if _, err := r.EC2Client.AuthorizeSecurityGroupIngress(ctx, &awsec2.AuthorizeSecurityGroupIngressInput{
			GroupId:       aws.String(groupID),
			IpPermissions: desired,
		}); err != nil {
			return fmt.Errorf("authorize ingress: %w", err)
		}
	}

	return nil
}

func (r *SecurityGroupReconciler) syncEgressRules(ctx context.Context, sg *awsv1alpha1.SecurityGroup, groupID string) error {
	desired, err := r.buildIPPermissions(ctx, sg.Namespace, sg.Spec.EgressRules)
	if err != nil {
		return err
	}

	out, err := r.EC2Client.DescribeSecurityGroups(ctx, &awsec2.DescribeSecurityGroupsInput{
		GroupIds: []string{groupID},
	})
	if err != nil {
		return err
	}
	if len(out.SecurityGroups) == 0 {
		return nil
	}

	existing := out.SecurityGroups[0].IpPermissionsEgress

	// Only modify egress if spec explicitly defines rules (otherwise leave the default all-outbound rule).
	if len(sg.Spec.EgressRules) == 0 {
		return nil
	}

	if len(existing) > 0 {
		if _, err := r.EC2Client.RevokeSecurityGroupEgress(ctx, &awsec2.RevokeSecurityGroupEgressInput{
			GroupId:       aws.String(groupID),
			IpPermissions: existing,
		}); err != nil {
			return fmt.Errorf("revoke egress: %w", err)
		}
	}

	if len(desired) > 0 {
		if _, err := r.EC2Client.AuthorizeSecurityGroupEgress(ctx, &awsec2.AuthorizeSecurityGroupEgressInput{
			GroupId:       aws.String(groupID),
			IpPermissions: desired,
		}); err != nil {
			return fmt.Errorf("authorize egress: %w", err)
		}
	}

	return nil
}

func (r *SecurityGroupReconciler) buildIPPermissions(ctx context.Context, namespace string, rules []awsv1alpha1.SGRule) ([]types.IpPermission, error) {
	perms := make([]types.IpPermission, 0, len(rules))
	for _, rule := range rules {
		perm := types.IpPermission{
			IpProtocol: aws.String(rule.Protocol),
		}
		if rule.FromPort != 0 || rule.ToPort != 0 {
			perm.FromPort = aws.Int32(rule.FromPort)
			perm.ToPort = aws.Int32(rule.ToPort)
		}
		if rule.CIDRIPv4 != "" {
			r4 := types.IpRange{CidrIp: aws.String(rule.CIDRIPv4)}
			if rule.Description != "" {
				r4.Description = aws.String(rule.Description)
			}
			perm.IpRanges = []types.IpRange{r4}
		}
		if rule.CIDRIPv6 != "" {
			r6 := types.Ipv6Range{CidrIpv6: aws.String(rule.CIDRIPv6)}
			if rule.Description != "" {
				r6.Description = aws.String(rule.Description)
			}
			perm.Ipv6Ranges = []types.Ipv6Range{r6}
		}
		if rule.PrefixListID != "" {
			pl := types.PrefixListId{PrefixListId: aws.String(rule.PrefixListID)}
			if rule.Description != "" {
				pl.Description = aws.String(rule.Description)
			}
			perm.PrefixListIds = []types.PrefixListId{pl}
		}
		if rule.SourceGroupRef != "" {
			srcSG := &awsv1alpha1.SecurityGroup{}
			if err := r.Get(ctx, k8stypes.NamespacedName{Name: rule.SourceGroupRef, Namespace: namespace}, srcSG); err != nil {
				return nil, err
			}
			if srcSG.Status.GroupID == "" {
				return nil, &dependencyNotReady{msg: fmt.Sprintf("SecurityGroup %s/%s has no ID yet", namespace, rule.SourceGroupRef)}
			}
			pair := types.UserIdGroupPair{GroupId: aws.String(srcSG.Status.GroupID)}
			if rule.Description != "" {
				pair.Description = aws.String(rule.Description)
			}
			perm.UserIdGroupPairs = []types.UserIdGroupPair{pair}
		}
		perms = append(perms, perm)
	}
	return perms, nil
}

// findGroupID looks up a security group created by this CR when the status ID
// was lost (e.g. a failed status write after CreateSecurityGroup). It matches
// on the spec GroupName, scoped to the VPC when it can still be resolved.
// Returns "" unless there is exactly one match.
func (r *SecurityGroupReconciler) findGroupID(ctx context.Context, sg *awsv1alpha1.SecurityGroup) (string, error) {
	filters := []types.Filter{
		{Name: aws.String("group-name"), Values: []string{sg.Spec.GroupName}},
	}
	// Best-effort VPC scoping: the referenced VPC CR may already be gone.
	if vpcID, err := r.resolveVPCID(ctx, sg.Namespace, &sg.Spec.VPCRef); err == nil && vpcID != "" {
		filters = append(filters, types.Filter{Name: aws.String("vpc-id"), Values: []string{vpcID}})
	}
	out, err := r.EC2Client.DescribeSecurityGroups(ctx, &awsec2.DescribeSecurityGroupsInput{Filters: filters})
	if err != nil {
		return "", fmt.Errorf("lookup security group by name: %w", err)
	}
	if len(out.SecurityGroups) != 1 {
		return "", nil
	}
	return aws.ToString(out.SecurityGroups[0].GroupId), nil
}

func (r *SecurityGroupReconciler) resolveVPCID(ctx context.Context, namespace string, ref *awsv1alpha1.VPCResourceRef) (string, error) {
	if ref.ID != "" {
		return ref.ID, nil
	}
	if ref.Name != "" {
		vpcCR := &awsv1alpha1.VPC{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, vpcCR); err != nil {
			return "", err
		}
		if vpcCR.Status.VPCID == "" {
			return "", &dependencyNotReady{msg: fmt.Sprintf("VPC %s/%s has no ID yet", namespace, ref.Name)}
		}
		return vpcCR.Status.VPCID, nil
	}
	return "", fmt.Errorf("vpcRef requires either name or id")
}

func (r *SecurityGroupReconciler) setCondition(ctx context.Context, sg *awsv1alpha1.SecurityGroup, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&sg.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: sg.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, sg); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *SecurityGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.SecurityGroup{}).
		Complete(r)
}
