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

// SubnetAWSAPI is the subset of the EC2 client used by the Subnet controller.
// *ec2.Client satisfies it.
type SubnetAWSAPI interface {
	CreateSubnet(ctx context.Context, params *awsec2.CreateSubnetInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateSubnetOutput, error)
	DeleteSubnet(ctx context.Context, params *awsec2.DeleteSubnetInput, optFns ...func(*awsec2.Options)) (*awsec2.DeleteSubnetOutput, error)
	DescribeSubnets(ctx context.Context, params *awsec2.DescribeSubnetsInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeSubnetsOutput, error)
	ModifySubnetAttribute(ctx context.Context, params *awsec2.ModifySubnetAttributeInput, optFns ...func(*awsec2.Options)) (*awsec2.ModifySubnetAttributeOutput, error)
	CreateTags(ctx context.Context, params *awsec2.CreateTagsInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateTagsOutput, error)
	DeleteTags(ctx context.Context, params *awsec2.DeleteTagsInput, optFns ...func(*awsec2.Options)) (*awsec2.DeleteTagsOutput, error)
	DescribeTags(ctx context.Context, params *awsec2.DescribeTagsInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeTagsOutput, error)
}

// SubnetReconciler reconciles Subnet objects.
type SubnetReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client SubnetAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=subnets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=subnets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=subnets/finalizers,verbs=update

func (r *SubnetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	sn := &awsv1alpha1.Subnet{}
	if err := r.Get(ctx, req.NamespacedName, sn); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, sn); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !sn.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(sn, awsv1alpha1.FinalizerName) {
			if shouldAbandon(sn) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(sn, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, sn)
			}
			subnetID := sn.Status.SubnetID
			if subnetID == "" {
				// Fallback: the status write may have been lost after create.
				// Look up by VPC + CIDR block (plus tags the create path set).
				id, err := r.findSubnetID(ctx, sn)
				if err != nil {
					return ctrl.Result{}, err
				}
				subnetID = id
			}
			if subnetID != "" {
				if _, err := r.EC2Client.DeleteSubnet(ctx, &awsec2.DeleteSubnetInput{
					SubnetId: aws.String(subnetID),
				}); err != nil && !ec2helper.IsNotFound(err) {
					logger.Error(err, "failed to delete subnet", "subnetId", subnetID)
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(sn, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, sn)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(sn, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(sn, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, sn); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileSubnet(ctx, sn); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, sn, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *SubnetReconciler) reconcileSubnet(ctx context.Context, sn *awsv1alpha1.Subnet) error {
	vpcID, err := r.resolveVPCID(ctx, sn.Namespace, &sn.Spec.VPCRef)
	if err != nil {
		return err
	}

	var subnetID string
	if sn.Status.SubnetID != "" {
		out, err := r.EC2Client.DescribeSubnets(ctx, &awsec2.DescribeSubnetsInput{
			SubnetIds: []string{sn.Status.SubnetID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return err
		}
		if err == nil && len(out.Subnets) > 0 {
			subnetID = sn.Status.SubnetID
			sn.Status.AvailableIPAddressCount = aws.ToInt32(out.Subnets[0].AvailableIpAddressCount)
		}
	}

	if subnetID == "" {
		input := &awsec2.CreateSubnetInput{
			VpcId:     aws.String(vpcID),
			CidrBlock: aws.String(sn.Spec.CIDRBlock),
			TagSpecifications: []types.TagSpecification{
				{ResourceType: types.ResourceTypeSubnet, Tags: ec2helper.TagsFromMap(sn.Spec.Tags)},
			},
		}
		if sn.Spec.AvailabilityZone != "" {
			input.AvailabilityZone = aws.String(sn.Spec.AvailabilityZone)
		}
		out, err := r.EC2Client.CreateSubnet(ctx, input)
		if err != nil {
			return fmt.Errorf("create subnet: %w", err)
		}
		subnetID = aws.ToString(out.Subnet.SubnetId)
		sn.Status.AvailableIPAddressCount = aws.ToInt32(out.Subnet.AvailableIpAddressCount)
		sn.Status.SubnetID = subnetID
		if err := persistStatus(ctx, r.Client, sn); err != nil {
			return fmt.Errorf("persist subnet ID after create: %w", err)
		}
	}

	sn.Status.SubnetID = subnetID

	// MapPublicIPOnLaunch attribute.
	if _, err := r.EC2Client.ModifySubnetAttribute(ctx, &awsec2.ModifySubnetAttributeInput{
		SubnetId:            aws.String(subnetID),
		MapPublicIpOnLaunch: &types.AttributeBooleanValue{Value: aws.Bool(sn.Spec.MapPublicIPOnLaunch)},
	}); err != nil {
		return fmt.Errorf("modify subnet attribute: %w", err)
	}

	if err := ec2helper.SyncResourceTags(ctx, r.EC2Client, subnetID, "subnet", sn.Spec.Tags); err != nil {
		return fmt.Errorf("sync tags: %w", err)
	}

	sn.Status.ObservedGeneration = sn.Generation
	now := metav1.Now()
	sn.Status.LastSyncTime = &now
	return r.setCondition(ctx, sn, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "subnet reconciled")
}

// findSubnetID looks up a subnet created by this CR when the status ID was
// lost (e.g. a failed status write after CreateSubnet). It matches on the
// spec CIDR block plus all tags the create path applied, scoped to the VPC
// when it can still be resolved. Returns "" unless there is exactly one match.
func (r *SubnetReconciler) findSubnetID(ctx context.Context, sn *awsv1alpha1.Subnet) (string, error) {
	filters := []types.Filter{
		{Name: aws.String("cidr-block"), Values: []string{sn.Spec.CIDRBlock}},
	}
	// Best-effort VPC scoping: the referenced VPC CR may already be gone.
	if vpcID, err := r.resolveVPCID(ctx, sn.Namespace, &sn.Spec.VPCRef); err == nil && vpcID != "" {
		filters = append(filters, types.Filter{Name: aws.String("vpc-id"), Values: []string{vpcID}})
	}
	for k, v := range sn.Spec.Tags {
		filters = append(filters, types.Filter{Name: aws.String("tag:" + k), Values: []string{v}})
	}
	out, err := r.EC2Client.DescribeSubnets(ctx, &awsec2.DescribeSubnetsInput{Filters: filters})
	if err != nil {
		return "", fmt.Errorf("lookup subnet by CIDR/tags: %w", err)
	}
	if len(out.Subnets) != 1 {
		return "", nil
	}
	return aws.ToString(out.Subnets[0].SubnetId), nil
}

func (r *SubnetReconciler) resolveVPCID(ctx context.Context, namespace string, ref *awsv1alpha1.VPCResourceRef) (string, error) {
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

func (r *SubnetReconciler) setCondition(ctx context.Context, sn *awsv1alpha1.Subnet, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&sn.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: sn.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, sn); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *SubnetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.Subnet{}).
		Complete(r)
}
