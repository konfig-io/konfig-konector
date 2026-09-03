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

// VPCPeeringConnectionReconciler reconciles VPCPeeringConnection objects.
type VPCPeeringConnectionReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client *awsec2.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpcpeeringconnections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpcpeeringconnections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpcpeeringconnections/finalizers,verbs=update

func (r *VPCPeeringConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	vpc := &awsv1alpha1.VPCPeeringConnection{}
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
			if err := r.deleteVPCPeeringConnection(ctx, vpc); err != nil {
				logger.Error(err, "failed to delete VPCPeeringConnection")
				return ctrl.Result{}, err
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

	if err := r.reconcileVPCPeeringConnection(ctx, vpc); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionVPC(ctx, vpc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *VPCPeeringConnectionReconciler) reconcileVPCPeeringConnection(ctx context.Context, obj *awsv1alpha1.VPCPeeringConnection) error {
	vpcID, err := r.resolveVPCIDForPeering(ctx, obj.Namespace, &obj.Spec.VPCRef)
	if err != nil {
		return err
	}

	// If we have an existing peering ID, verify it still exists.
	if obj.Status.PeeringID != "" {
		out, err := r.EC2Client.DescribeVpcPeeringConnections(ctx, &awsec2.DescribeVpcPeeringConnectionsInput{
			VpcPeeringConnectionIds: []string{obj.Status.PeeringID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return fmt.Errorf("describe vpc peering connection: %w", err)
		}
		if err == nil && len(out.VpcPeeringConnections) > 0 {
			state := string(out.VpcPeeringConnections[0].Status.Code)
			obj.Status.Status = state
			// Tag sync
			if len(obj.Spec.Tags) > 0 {
				_, _ = r.EC2Client.CreateTags(ctx, &awsec2.CreateTagsInput{
					Resources: []string{obj.Status.PeeringID},
					Tags:      ec2helper.TagsFromMap(obj.Spec.Tags),
				})
			}
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionVPC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "VPCPeeringConnection reconciled")
		}
		obj.Status.PeeringID = ""
	}

	input := &awsec2.CreateVpcPeeringConnectionInput{
		VpcId:     aws.String(vpcID),
		PeerVpcId: aws.String(obj.Spec.PeerVPCID),
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeVpcPeeringConnection,
				Tags:         ec2helper.TagsFromMap(obj.Spec.Tags),
			},
		},
	}
	if obj.Spec.PeerOwnerID != "" {
		input.PeerOwnerId = aws.String(obj.Spec.PeerOwnerID)
	}
	if obj.Spec.PeerRegion != "" {
		input.PeerRegion = aws.String(obj.Spec.PeerRegion)
	}

	out, err := r.EC2Client.CreateVpcPeeringConnection(ctx, input)
	if err != nil {
		return fmt.Errorf("create vpc peering connection: %w", err)
	}

	peeringID := aws.ToString(out.VpcPeeringConnection.VpcPeeringConnectionId)
	obj.Status.PeeringID = peeringID
	obj.Status.Status = string(out.VpcPeeringConnection.Status.Code)
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist peering connection ID after create: %w", err)
	}

	// Auto-accept for same-account peering.
	if obj.Spec.AutoAccept {
		_, err = r.EC2Client.AcceptVpcPeeringConnection(ctx, &awsec2.AcceptVpcPeeringConnectionInput{
			VpcPeeringConnectionId: aws.String(peeringID),
		})
		if err != nil {
			return fmt.Errorf("accept vpc peering connection: %w", err)
		}
		obj.Status.Status = "active"
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionVPC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "VPCPeeringConnection created")
}

func (r *VPCPeeringConnectionReconciler) resolveVPCIDForPeering(ctx context.Context, namespace string, ref *awsv1alpha1.VPCResourceRef) (string, error) {
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

func (r *VPCPeeringConnectionReconciler) deleteVPCPeeringConnection(ctx context.Context, obj *awsv1alpha1.VPCPeeringConnection) error {
	if obj.Status.PeeringID == "" {
		// No fallback lookup: peering connections are genuinely ambiguous to
		// identify (cross-account/cross-region peers, no unique name). The
		// persist immediately after create keeps this window tiny.
		return nil
	}
	_, err := r.EC2Client.DeleteVpcPeeringConnection(ctx, &awsec2.DeleteVpcPeeringConnectionInput{
		VpcPeeringConnectionId: aws.String(obj.Status.PeeringID),
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *VPCPeeringConnectionReconciler) setConditionVPC(ctx context.Context, obj *awsv1alpha1.VPCPeeringConnection, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *VPCPeeringConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.VPCPeeringConnection{}).
		Complete(r)
}
