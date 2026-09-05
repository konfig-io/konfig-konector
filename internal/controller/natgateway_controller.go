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
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// requeueNATPolling is the requeue interval while waiting for a NAT gateway to become available.
var requeueNATPolling = ctrl.Result{RequeueAfter: 15 * time.Second}

// NatGatewayReconciler reconciles NatGateway objects.
type NatGatewayReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client *multi.EC2
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=natgateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=natgateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=natgateways/finalizers,verbs=update

func (r *NatGatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ng := &awsv1alpha1.NatGateway{}
	if err := r.Get(ctx, req.NamespacedName, ng); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, ng); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !ng.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ng, awsv1alpha1.FinalizerName) {
			if shouldAbandon(ng) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(ng, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, ng)
			}
			if ng.Status.NatGatewayID == "" {
				// Fallback: the status write may have been lost after create.
				// Look up by subnet plus the tags the create path applied;
				// this also recovers the EIP allocation ID for release below.
				if err := r.adoptNatGateway(ctx, ng); err != nil {
					return ctrl.Result{}, err
				}
			}
			if ng.Status.NatGatewayID != "" {
				if _, err := r.EC2Client.DeleteNatGateway(ctx, &awsec2.DeleteNatGatewayInput{
					NatGatewayId: aws.String(ng.Status.NatGatewayID),
				}); err != nil && !ec2helper.IsNotFound(err) {
					logger.Error(err, "failed to delete NAT gateway", "natGwId", ng.Status.NatGatewayID)
					return ctrl.Result{}, err
				}
				// Release the Elastic IP after NAT gateway deletion.
				if ng.Status.ElasticIPAllocationID != "" {
					// Poll until NAT GW is deleted before releasing EIP.
					out, err := r.EC2Client.DescribeNatGateways(ctx, &awsec2.DescribeNatGatewaysInput{
						NatGatewayIds: []string{ng.Status.NatGatewayID},
					})
					if err == nil && len(out.NatGateways) > 0 {
						state := string(out.NatGateways[0].State)
						if state != "deleted" && state != "failed" {
							// Still deleting — requeue.
							ng.Status.State = state
							if err := r.Status().Update(ctx, ng); err != nil {
								logger.Error(err, "failed to update NAT gateway deletion status")
							}
							return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
						}
					}
					if _, err := r.EC2Client.ReleaseAddress(ctx, &awsec2.ReleaseAddressInput{
						AllocationId: aws.String(ng.Status.ElasticIPAllocationID),
					}); err != nil && !ec2helper.IsNotFound(err) {
						logger.Error(err, "failed to release EIP during NAT gateway deletion", "allocationId", ng.Status.ElasticIPAllocationID)
						return ctrl.Result{}, err
					}
				}
			}
			controllerutil.RemoveFinalizer(ng, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, ng)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(ng, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(ng, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, ng); err != nil {
			return ctrl.Result{}, err
		}
	}

	result, err := r.reconcileNATGateway(ctx, ng)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, ng, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *NatGatewayReconciler) reconcileNATGateway(ctx context.Context, ng *awsv1alpha1.NatGateway) (ctrl.Result, error) {
	subnetID, err := r.resolveSubnetID(ctx, ng.Namespace, &ng.Spec.SubnetRef)
	if err != nil {
		return ctrl.Result{}, err
	}

	// If we already have a NAT GW ID, check its state.
	if ng.Status.NatGatewayID != "" {
		out, err := r.EC2Client.DescribeNatGateways(ctx, &awsec2.DescribeNatGatewaysInput{
			NatGatewayIds: []string{ng.Status.NatGatewayID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err == nil && len(out.NatGateways) > 0 {
			state := string(out.NatGateways[0].State)
			ng.Status.State = state
			if state == "available" {
				if err := ec2helper.SyncResourceTags(ctx, r.EC2Client, ng.Status.NatGatewayID, "natgateway", ng.Spec.Tags); err != nil {
					return ctrl.Result{}, fmt.Errorf("sync tags: %w", err)
				}
				ng.Status.ObservedGeneration = ng.Generation
				now := metav1.Now()
				ng.Status.LastSyncTime = &now
				return requeueResult(), r.setCondition(ctx, ng, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "NAT gateway available")
			}
			if state == "pending" {
				_ = r.setCondition(ctx, ng, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Pending", "NAT gateway is pending")
				return requeueNATPolling, nil
			}
			// deleted/failed — recreate.
			ng.Status.NatGatewayID = ""
			ng.Status.ElasticIPAllocationID = ""
		}
	}

	connectivityType := types.ConnectivityTypePublic
	if ng.Spec.ConnectivityType == "private" {
		connectivityType = types.ConnectivityTypePrivate
	}

	input := &awsec2.CreateNatGatewayInput{
		SubnetId:         aws.String(subnetID),
		ConnectivityType: connectivityType,
		TagSpecifications: []types.TagSpecification{
			{ResourceType: types.ResourceTypeNatgateway, Tags: ec2helper.TagsFromMap(ng.Spec.Tags)},
		},
	}

	// Allocate EIP for public NAT.
	var allocationID string
	if connectivityType == types.ConnectivityTypePublic {
		eipOut, err := r.EC2Client.AllocateAddress(ctx, &awsec2.AllocateAddressInput{
			Domain: types.DomainTypeVpc,
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("allocate EIP: %w", err)
		}
		allocationID = aws.ToString(eipOut.AllocationId)
		input.AllocationId = eipOut.AllocationId
	}

	out, err := r.EC2Client.CreateNatGateway(ctx, input)
	if err != nil {
		// Release the EIP we just allocated to avoid a permanent leak.
		if allocationID != "" {
			if _, releaseErr := r.EC2Client.ReleaseAddress(ctx, &awsec2.ReleaseAddressInput{
				AllocationId: aws.String(allocationID),
			}); releaseErr != nil {
				log.FromContext(ctx).Error(releaseErr, "failed to release EIP after CreateNatGateway failure", "allocationId", allocationID)
			}
		}
		return ctrl.Result{}, fmt.Errorf("create NAT gateway: %w", err)
	}

	ng.Status.ElasticIPAllocationID = allocationID
	ng.Status.NatGatewayID = aws.ToString(out.NatGateway.NatGatewayId)
	ng.Status.State = string(out.NatGateway.State)
	// Persist status so allocationId and natGwId survive a crash before the next reconcile.
	if err := persistStatus(ctx, r.Client, ng); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist NAT gateway ID after create: %w", err)
	}
	_ = r.setCondition(ctx, ng, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Pending", "NAT gateway is being created")
	return requeueNATPolling, nil
}

// adoptNatGateway looks up a NAT gateway created by this CR when the status ID
// was lost (e.g. a failed status write after CreateNatGateway). It matches on
// the subnet plus all tags the create path applied, ignoring already-deleted
// gateways, and only adopts when there is exactly one match. It also recovers
// the EIP allocation ID so deletion can release the address.
func (r *NatGatewayReconciler) adoptNatGateway(ctx context.Context, ng *awsv1alpha1.NatGateway) error {
	subnetID, err := r.resolveSubnetID(ctx, ng.Namespace, &ng.Spec.SubnetRef)
	if err != nil {
		// The referenced Subnet CR may already be gone — nothing to find.
		return nil
	}
	filters := []types.Filter{
		{Name: aws.String("subnet-id"), Values: []string{subnetID}},
		{Name: aws.String("state"), Values: []string{"pending", "available", "failed", "deleting"}},
	}
	for k, v := range ng.Spec.Tags {
		filters = append(filters, types.Filter{Name: aws.String("tag:" + k), Values: []string{v}})
	}
	out, err := r.EC2Client.DescribeNatGateways(ctx, &awsec2.DescribeNatGatewaysInput{Filter: filters})
	if err != nil {
		return fmt.Errorf("lookup NAT gateway by subnet/tags: %w", err)
	}
	if len(out.NatGateways) != 1 {
		return nil
	}
	ng.Status.NatGatewayID = aws.ToString(out.NatGateways[0].NatGatewayId)
	for _, addr := range out.NatGateways[0].NatGatewayAddresses {
		if addr.AllocationId != nil {
			ng.Status.ElasticIPAllocationID = aws.ToString(addr.AllocationId)
			break
		}
	}
	return nil
}

func (r *NatGatewayReconciler) resolveSubnetID(ctx context.Context, namespace string, ref *awsv1alpha1.SubnetRef) (string, error) {
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

func (r *NatGatewayReconciler) setCondition(ctx context.Context, ng *awsv1alpha1.NatGateway, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&ng.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: ng.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, ng); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *NatGatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.NatGateway{}).
		Complete(r)
}
