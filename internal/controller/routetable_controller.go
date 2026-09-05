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
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// RouteTableReconciler reconciles RouteTable objects.
type RouteTableReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client *multi.EC2
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=routetables,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=routetables/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=routetables/finalizers,verbs=update

func (r *RouteTableReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	rt := &awsv1alpha1.RouteTable{}
	if err := r.Get(ctx, req.NamespacedName, rt); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, rt); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !rt.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(rt, awsv1alpha1.FinalizerName) {
			if shouldAbandon(rt) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(rt, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, rt)
			}
			rtbID := rt.Status.RouteTableID
			if rtbID == "" {
				// Fallback: the status write may have been lost after create.
				// Look up by the tags the create path applied.
				id, err := r.findRouteTableID(ctx, rt)
				if err != nil {
					return ctrl.Result{}, err
				}
				rtbID = id
			}
			if rtbID != "" {
				// Disassociate subnets before deleting.
				if err := r.disassociateAllSubnets(ctx, rtbID); err != nil {
					logger.Error(err, "failed to disassociate subnets from route table", "rtbId", rtbID)
					return ctrl.Result{}, err
				}
				if _, err := r.EC2Client.DeleteRouteTable(ctx, &awsec2.DeleteRouteTableInput{
					RouteTableId: aws.String(rtbID),
				}); err != nil && !ec2helper.IsNotFound(err) {
					logger.Error(err, "failed to delete route table", "rtbId", rtbID)
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(rt, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, rt)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(rt, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(rt, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, rt); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileRouteTable(ctx, rt); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, rt, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *RouteTableReconciler) reconcileRouteTable(ctx context.Context, rt *awsv1alpha1.RouteTable) error {
	vpcID, err := r.resolveVPCID(ctx, rt.Namespace, &rt.Spec.VPCRef)
	if err != nil {
		return err
	}

	rtbID := rt.Status.RouteTableID
	if rtbID != "" {
		out, err := r.EC2Client.DescribeRouteTables(ctx, &awsec2.DescribeRouteTablesInput{
			RouteTableIds: []string{rtbID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return err
		}
		if err != nil || len(out.RouteTables) == 0 {
			rtbID = ""
		}
	}

	if rtbID == "" {
		out, err := r.EC2Client.CreateRouteTable(ctx, &awsec2.CreateRouteTableInput{
			VpcId: aws.String(vpcID),
			TagSpecifications: []types.TagSpecification{
				{ResourceType: types.ResourceTypeRouteTable, Tags: ec2helper.TagsFromMap(rt.Spec.Tags)},
			},
		})
		if err != nil {
			return fmt.Errorf("create route table: %w", err)
		}
		rtbID = aws.ToString(out.RouteTable.RouteTableId)
		rt.Status.RouteTableID = rtbID
		if err := persistStatus(ctx, r.Client, rt); err != nil {
			return fmt.Errorf("persist route table ID after create: %w", err)
		}
	}

	rt.Status.RouteTableID = rtbID

	// Sync routes.
	if err := r.syncRoutes(ctx, rt, rtbID); err != nil {
		return err
	}

	// Sync subnet associations.
	if err := r.syncSubnetAssociations(ctx, rt, rtbID); err != nil {
		return err
	}

	if err := ec2helper.SyncResourceTags(ctx, r.EC2Client, rtbID, "route-table", rt.Spec.Tags); err != nil {
		return fmt.Errorf("sync tags: %w", err)
	}

	rt.Status.ObservedGeneration = rt.Generation
	now := metav1.Now()
	rt.Status.LastSyncTime = &now
	return r.setCondition(ctx, rt, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "route table reconciled")
}

func (r *RouteTableReconciler) syncRoutes(ctx context.Context, rt *awsv1alpha1.RouteTable, rtbID string) error {
	out, err := r.EC2Client.DescribeRouteTables(ctx, &awsec2.DescribeRouteTablesInput{
		RouteTableIds: []string{rtbID},
	})
	if err != nil {
		return err
	}

	// Build set of desired destination CIDRs.
	desired := make(map[string]awsv1alpha1.RouteEntry, len(rt.Spec.Routes))
	for _, route := range rt.Spec.Routes {
		desired[route.DestinationCIDR] = route
	}

	// Build set of existing destination CIDRs (skip the local route).
	existing := make(map[string]bool)
	if len(out.RouteTables) > 0 {
		for _, route := range out.RouteTables[0].Routes {
			cidr := aws.ToString(route.DestinationCidrBlock)
			if aws.ToString(route.GatewayId) == "local" {
				continue
			}
			existing[cidr] = true
		}
	}

	// Add missing routes.
	for cidr, entry := range desired {
		if existing[cidr] {
			continue
		}
		input := &awsec2.CreateRouteInput{
			RouteTableId:         aws.String(rtbID),
			DestinationCidrBlock: aws.String(cidr),
		}
		if entry.GatewayID != "" {
			input.GatewayId = aws.String(entry.GatewayID)
		}
		if entry.NatGatewayRef != "" {
			natGW := &awsv1alpha1.NatGateway{}
			if err := r.Get(ctx, k8stypes.NamespacedName{Name: entry.NatGatewayRef, Namespace: rt.Namespace}, natGW); err != nil {
				return err
			}
			if natGW.Status.NatGatewayID == "" {
				return &dependencyNotReady{msg: fmt.Sprintf("NatGateway %s/%s has no ID yet", rt.Namespace, entry.NatGatewayRef)}
			}
			input.NatGatewayId = aws.String(natGW.Status.NatGatewayID)
		}
		if _, err := r.EC2Client.CreateRoute(ctx, input); err != nil {
			return fmt.Errorf("create route %s: %w", cidr, err)
		}
	}

	// Remove routes no longer in spec.
	for cidr := range existing {
		if _, ok := desired[cidr]; !ok {
			if _, err := r.EC2Client.DeleteRoute(ctx, &awsec2.DeleteRouteInput{
				RouteTableId:         aws.String(rtbID),
				DestinationCidrBlock: aws.String(cidr),
			}); err != nil {
				return fmt.Errorf("delete route %s: %w", cidr, err)
			}
		}
	}

	return nil
}

func (r *RouteTableReconciler) syncSubnetAssociations(ctx context.Context, rt *awsv1alpha1.RouteTable, rtbID string) error {
	out, err := r.EC2Client.DescribeRouteTables(ctx, &awsec2.DescribeRouteTablesInput{
		RouteTableIds: []string{rtbID},
	})
	if err != nil {
		return err
	}

	// Current association map: subnetID -> associationID
	currentAssocs := make(map[string]string)
	if len(out.RouteTables) > 0 {
		for _, assoc := range out.RouteTables[0].Associations {
			if aws.ToString(assoc.SubnetId) != "" {
				currentAssocs[aws.ToString(assoc.SubnetId)] = aws.ToString(assoc.RouteTableAssociationId)
			}
		}
	}

	// Build desired subnet IDs.
	desiredSubnets := make(map[string]bool)
	for _, subnetName := range rt.Spec.SubnetAssociations {
		sn := &awsv1alpha1.Subnet{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: subnetName, Namespace: rt.Namespace}, sn); err != nil {
			return err
		}
		if sn.Status.SubnetID == "" {
			return &dependencyNotReady{msg: fmt.Sprintf("Subnet %s/%s has no ID yet", rt.Namespace, subnetName)}
		}
		desiredSubnets[sn.Status.SubnetID] = true

		// Associate if not already.
		if _, ok := currentAssocs[sn.Status.SubnetID]; !ok {
			if _, err := r.EC2Client.AssociateRouteTable(ctx, &awsec2.AssociateRouteTableInput{
				RouteTableId: aws.String(rtbID),
				SubnetId:     aws.String(sn.Status.SubnetID),
			}); err != nil {
				return fmt.Errorf("associate subnet %s: %w", sn.Status.SubnetID, err)
			}
		}
	}

	// Disassociate subnets no longer in spec.
	for subnetID, assocID := range currentAssocs {
		if !desiredSubnets[subnetID] {
			if _, err := r.EC2Client.DisassociateRouteTable(ctx, &awsec2.DisassociateRouteTableInput{
				AssociationId: aws.String(assocID),
			}); err != nil {
				return fmt.Errorf("disassociate subnet %s: %w", subnetID, err)
			}
		}
	}

	return nil
}

// findRouteTableID looks up a route table created by this CR when the status
// ID was lost (e.g. a failed status write after CreateRouteTable). It matches
// on the tags the create path applied, scoped to the VPC when resolvable.
// Returns "" when the CR has no tags or there is not exactly one match.
func (r *RouteTableReconciler) findRouteTableID(ctx context.Context, rt *awsv1alpha1.RouteTable) (string, error) {
	if len(rt.Spec.Tags) == 0 {
		return "", nil
	}
	var filters []types.Filter
	// Best-effort VPC scoping: the referenced VPC CR may already be gone.
	if vpcID, err := r.resolveVPCID(ctx, rt.Namespace, &rt.Spec.VPCRef); err == nil && vpcID != "" {
		filters = append(filters, types.Filter{Name: aws.String("vpc-id"), Values: []string{vpcID}})
	}
	for k, v := range rt.Spec.Tags {
		filters = append(filters, types.Filter{Name: aws.String("tag:" + k), Values: []string{v}})
	}
	out, err := r.EC2Client.DescribeRouteTables(ctx, &awsec2.DescribeRouteTablesInput{Filters: filters})
	if err != nil {
		return "", fmt.Errorf("lookup route table by tags: %w", err)
	}
	if len(out.RouteTables) != 1 {
		return "", nil
	}
	return aws.ToString(out.RouteTables[0].RouteTableId), nil
}

func (r *RouteTableReconciler) disassociateAllSubnets(ctx context.Context, rtbID string) error {
	out, err := r.EC2Client.DescribeRouteTables(ctx, &awsec2.DescribeRouteTablesInput{
		RouteTableIds: []string{rtbID},
	})
	if err != nil || len(out.RouteTables) == 0 {
		return nil
	}
	for _, assoc := range out.RouteTables[0].Associations {
		if aws.ToString(assoc.SubnetId) != "" {
			if _, err := r.EC2Client.DisassociateRouteTable(ctx, &awsec2.DisassociateRouteTableInput{
				AssociationId: assoc.RouteTableAssociationId,
			}); err != nil {
				return fmt.Errorf("disassociate route table association %s: %w", aws.ToString(assoc.RouteTableAssociationId), err)
			}
		}
	}
	return nil
}

func (r *RouteTableReconciler) resolveVPCID(ctx context.Context, namespace string, ref *awsv1alpha1.VPCResourceRef) (string, error) {
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

func (r *RouteTableReconciler) setCondition(ctx context.Context, rt *awsv1alpha1.RouteTable, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&rt.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: rt.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, rt); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *RouteTableReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.RouteTable{}).
		Complete(r)
}
