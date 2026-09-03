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

// VPCEndpointReconciler reconciles VPCEndpoint objects.
type VPCEndpointReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client *awsec2.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpcendpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpcendpoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpcendpoints/finalizers,verbs=update

func (r *VPCEndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ep := &awsv1alpha1.VPCEndpoint{}
	if err := r.Get(ctx, req.NamespacedName, ep); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !ep.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ep, awsv1alpha1.FinalizerName) {
			if shouldAbandon(ep) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(ep, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, ep)
			}
			endpointID := ep.Status.EndpointID
			if endpointID == "" {
				// Fallback: the status write may have been lost after create.
				// Look up by service name (plus tags), scoped to the VPC.
				id, err := r.findEndpointID(ctx, ep)
				if err != nil {
					return ctrl.Result{}, err
				}
				endpointID = id
			}
			if endpointID != "" {
				if _, err := r.EC2Client.DeleteVpcEndpoints(ctx, &awsec2.DeleteVpcEndpointsInput{
					VpcEndpointIds: []string{endpointID},
				}); err != nil && !ec2helper.IsNotFound(err) {
					logger.Error(err, "failed to delete VPC endpoint", "endpointId", endpointID)
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(ep, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, ep)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(ep, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(ep, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, ep); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileEndpoint(ctx, ep); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, ep, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *VPCEndpointReconciler) reconcileEndpoint(ctx context.Context, ep *awsv1alpha1.VPCEndpoint) error {
	vpcID, err := r.resolveVPCID(ctx, ep.Namespace, &ep.Spec.VPCRef)
	if err != nil {
		return err
	}

	endpointID := ep.Status.EndpointID
	if endpointID != "" {
		out, err := r.EC2Client.DescribeVpcEndpoints(ctx, &awsec2.DescribeVpcEndpointsInput{
			VpcEndpointIds: []string{endpointID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return err
		}
		if err != nil || len(out.VpcEndpoints) == 0 {
			endpointID = ""
		} else {
			ep.Status.State = string(out.VpcEndpoints[0].State)
		}
	}

	if endpointID == "" {
		// Resolve subnet IDs for Interface endpoints.
		var subnetIDs []string
		for i := range ep.Spec.SubnetRefs {
			sid, err := r.resolveSubnetIDFromRef(ctx, ep.Namespace, &ep.Spec.SubnetRefs[i])
			if err != nil {
				return err
			}
			subnetIDs = append(subnetIDs, sid)
		}

		// Resolve security group IDs.
		var sgIDs []string
		for i := range ep.Spec.SecurityGroupRefs {
			sgID, err := r.resolveSGID(ctx, ep.Namespace, &ep.Spec.SecurityGroupRefs[i])
			if err != nil {
				return err
			}
			sgIDs = append(sgIDs, sgID)
		}

		// Resolve route table IDs.
		var rtbIDs []string
		for _, rtbName := range ep.Spec.RouteTableRefs {
			rt := &awsv1alpha1.RouteTable{}
			if err := r.Get(ctx, k8stypes.NamespacedName{Name: rtbName, Namespace: ep.Namespace}, rt); err != nil {
				return err
			}
			if rt.Status.RouteTableID == "" {
				return &dependencyNotReady{msg: fmt.Sprintf("RouteTable %s/%s has no ID yet", ep.Namespace, rtbName)}
			}
			rtbIDs = append(rtbIDs, rt.Status.RouteTableID)
		}

		input := &awsec2.CreateVpcEndpointInput{
			VpcId:           aws.String(vpcID),
			ServiceName:     aws.String(ep.Spec.ServiceName),
			VpcEndpointType: types.VpcEndpointType(ep.Spec.EndpointType),
			TagSpecifications: []types.TagSpecification{
				{ResourceType: types.ResourceTypeVpcEndpoint, Tags: ec2helper.TagsFromMap(ep.Spec.Tags)},
			},
		}
		if len(subnetIDs) > 0 {
			input.SubnetIds = subnetIDs
		}
		if len(sgIDs) > 0 {
			for _, id := range sgIDs {
				id := id
				input.SecurityGroupIds = append(input.SecurityGroupIds, id)
			}
		}
		if len(rtbIDs) > 0 {
			input.RouteTableIds = rtbIDs
		}

		out, err := r.EC2Client.CreateVpcEndpoint(ctx, input)
		if err != nil {
			return fmt.Errorf("create VPC endpoint: %w", err)
		}
		endpointID = aws.ToString(out.VpcEndpoint.VpcEndpointId)
		ep.Status.State = string(out.VpcEndpoint.State)
		ep.Status.EndpointID = endpointID
		if err := persistStatus(ctx, r.Client, ep); err != nil {
			return fmt.Errorf("persist VPC endpoint ID after create: %w", err)
		}
	}

	ep.Status.EndpointID = endpointID

	if err := ec2helper.SyncResourceTags(ctx, r.EC2Client, endpointID, "vpc-endpoint", ep.Spec.Tags); err != nil {
		return fmt.Errorf("sync tags: %w", err)
	}

	ep.Status.ObservedGeneration = ep.Generation
	now := metav1.Now()
	ep.Status.LastSyncTime = &now
	return r.setCondition(ctx, ep, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "VPC endpoint reconciled")
}

// findEndpointID looks up a VPC endpoint created by this CR when the status
// ID was lost (e.g. a failed status write after CreateVpcEndpoint). It matches
// on the service name plus tags the create path applied, scoped to the VPC
// when it can still be resolved. Returns "" unless there is exactly one match.
func (r *VPCEndpointReconciler) findEndpointID(ctx context.Context, ep *awsv1alpha1.VPCEndpoint) (string, error) {
	filters := []types.Filter{
		{Name: aws.String("service-name"), Values: []string{ep.Spec.ServiceName}},
	}
	// Best-effort VPC scoping: the referenced VPC CR may already be gone.
	if vpcID, err := r.resolveVPCID(ctx, ep.Namespace, &ep.Spec.VPCRef); err == nil && vpcID != "" {
		filters = append(filters, types.Filter{Name: aws.String("vpc-id"), Values: []string{vpcID}})
	}
	for k, v := range ep.Spec.Tags {
		filters = append(filters, types.Filter{Name: aws.String("tag:" + k), Values: []string{v}})
	}
	out, err := r.EC2Client.DescribeVpcEndpoints(ctx, &awsec2.DescribeVpcEndpointsInput{Filters: filters})
	if err != nil {
		return "", fmt.Errorf("lookup VPC endpoint by service/tags: %w", err)
	}
	if len(out.VpcEndpoints) != 1 {
		return "", nil
	}
	return aws.ToString(out.VpcEndpoints[0].VpcEndpointId), nil
}

func (r *VPCEndpointReconciler) resolveVPCID(ctx context.Context, namespace string, ref *awsv1alpha1.VPCResourceRef) (string, error) {
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

func (r *VPCEndpointReconciler) resolveSubnetIDFromRef(ctx context.Context, namespace string, ref *awsv1alpha1.SubnetRef) (string, error) {
	if ref.ID != "" {
		return ref.ID, nil
	}
	if ref.Name != "" {
		sn := &awsv1alpha1.Subnet{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, sn); err != nil {
			return "", err
		}
		if sn.Status.SubnetID == "" {
			return "", &dependencyNotReady{msg: fmt.Sprintf("Subnet %s/%s has no ID yet", namespace, ref.Name)}
		}
		return sn.Status.SubnetID, nil
	}
	return "", fmt.Errorf("subnetRef requires either name or id")
}

func (r *VPCEndpointReconciler) resolveSGID(ctx context.Context, namespace string, ref *awsv1alpha1.SecurityGroupRef) (string, error) {
	if ref.ID != "" {
		return ref.ID, nil
	}
	if ref.Name != "" {
		sgCR := &awsv1alpha1.SecurityGroup{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, sgCR); err != nil {
			return "", err
		}
		if sgCR.Status.GroupID == "" {
			return "", &dependencyNotReady{msg: fmt.Sprintf("SecurityGroup %s/%s has no ID yet", namespace, ref.Name)}
		}
		return sgCR.Status.GroupID, nil
	}
	return "", fmt.Errorf("securityGroupRef requires either name or id")
}

func (r *VPCEndpointReconciler) setCondition(ctx context.Context, ep *awsv1alpha1.VPCEndpoint, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&ep.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: ep.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, ep); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *VPCEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.VPCEndpoint{}).
		Complete(r)
}
