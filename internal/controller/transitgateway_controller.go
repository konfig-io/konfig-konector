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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ec2helper "github.com/konfig-io/konfig-konector/internal/aws/ec2"
)

// TransitGatewayReconciler reconciles TransitGateway objects.
type TransitGatewayReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client *awsec2.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=transitgateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=transitgateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=transitgateways/finalizers,verbs=update

func (r *TransitGatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	tgw := &awsv1alpha1.TransitGateway{}
	if err := r.Get(ctx, req.NamespacedName, tgw); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !tgw.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(tgw, awsv1alpha1.FinalizerName) {
			if shouldAbandon(tgw) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(tgw, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, tgw)
			}
			if err := r.deleteTransitGateway(ctx, tgw); err != nil {
				logger.Error(err, "failed to delete TransitGateway")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(tgw, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, tgw)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(tgw, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(tgw, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, tgw); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileTransitGateway(ctx, tgw); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionTGW(ctx, tgw, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *TransitGatewayReconciler) reconcileTransitGateway(ctx context.Context, tgw *awsv1alpha1.TransitGateway) error {
	if tgw.Status.TransitGatewayID != "" {
		out, err := r.EC2Client.DescribeTransitGateways(ctx, &awsec2.DescribeTransitGatewaysInput{
			TransitGatewayIds: []string{tgw.Status.TransitGatewayID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return fmt.Errorf("describe transit gateway: %w", err)
		}
		if err == nil && len(out.TransitGateways) > 0 {
			existing := out.TransitGateways[0]
			tgw.Status.State = string(existing.State)
			tgw.Status.ARN = aws.ToString(existing.TransitGatewayArn)
			// Sync tags.
			if len(tgw.Spec.Tags) > 0 {
				if _, tagErr := r.EC2Client.CreateTags(ctx, &awsec2.CreateTagsInput{
					Resources: []string{tgw.Status.TransitGatewayID},
					Tags:      ec2helper.TagsFromMap(tgw.Spec.Tags),
				}); tagErr != nil {
					return fmt.Errorf("tag transit gateway: %w", tagErr)
				}
			}
			tgw.Status.ObservedGeneration = tgw.Generation
			now := metav1.Now()
			tgw.Status.LastSyncTime = &now
			return r.setConditionTGW(ctx, tgw, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "TransitGateway reconciled")
		}
		tgw.Status.TransitGatewayID = ""
	}

	// Build options.
	opts := &ec2types.TransitGatewayRequestOptions{}
	if tgw.Spec.AmazonSideASN > 0 {
		opts.AmazonSideAsn = aws.Int64(tgw.Spec.AmazonSideASN)
	}
	if tgw.Spec.AutoAcceptSharedAttachments {
		opts.AutoAcceptSharedAttachments = ec2types.AutoAcceptSharedAttachmentsValueEnable
	}
	if tgw.Spec.DefaultRouteTableAssociation {
		opts.DefaultRouteTableAssociation = ec2types.DefaultRouteTableAssociationValueEnable
	}
	if tgw.Spec.DefaultRouteTablePropagation {
		opts.DefaultRouteTablePropagation = ec2types.DefaultRouteTablePropagationValueEnable
	}
	if tgw.Spec.DNSSupport {
		opts.DnsSupport = ec2types.DnsSupportValueEnable
	}
	if tgw.Spec.VPNECMPSupport {
		opts.VpnEcmpSupport = ec2types.VpnEcmpSupportValueEnable
	}

	createOut, err := r.EC2Client.CreateTransitGateway(ctx, &awsec2.CreateTransitGatewayInput{
		Description: aws.String(tgw.Spec.Description),
		Options:     opts,
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeTransitGateway,
				Tags:         ec2helper.TagsFromMap(tgw.Spec.Tags),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("create transit gateway: %w", err)
	}

	tgw.Status.TransitGatewayID = aws.ToString(createOut.TransitGateway.TransitGatewayId)
	tgw.Status.ARN = aws.ToString(createOut.TransitGateway.TransitGatewayArn)
	tgw.Status.State = string(createOut.TransitGateway.State)
	if err := persistStatus(ctx, r.Client, tgw); err != nil {
		return fmt.Errorf("persist TransitGateway ID after create: %w", err)
	}
	tgw.Status.ObservedGeneration = tgw.Generation
	now := metav1.Now()
	tgw.Status.LastSyncTime = &now
	return r.setConditionTGW(ctx, tgw, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "TransitGateway created")
}

func (r *TransitGatewayReconciler) deleteTransitGateway(ctx context.Context, tgw *awsv1alpha1.TransitGateway) error {
	tgwID := tgw.Status.TransitGatewayID
	if tgwID == "" {
		// Fallback: the status write may have been lost after create.
		// Look up by the tags the create path applied; without tags the
		// gateway is indistinguishable from others, so give up.
		if len(tgw.Spec.Tags) == 0 {
			return nil
		}
		filters := []ec2types.Filter{
			{Name: aws.String("state"), Values: []string{"pending", "available", "modifying"}},
		}
		for k, v := range tgw.Spec.Tags {
			filters = append(filters, ec2types.Filter{Name: aws.String("tag:" + k), Values: []string{v}})
		}
		out, err := r.EC2Client.DescribeTransitGateways(ctx, &awsec2.DescribeTransitGatewaysInput{Filters: filters})
		if err != nil {
			return fmt.Errorf("lookup transit gateway by tags: %w", err)
		}
		if len(out.TransitGateways) != 1 {
			return nil
		}
		tgwID = aws.ToString(out.TransitGateways[0].TransitGatewayId)
	}
	_, err := r.EC2Client.DeleteTransitGateway(ctx, &awsec2.DeleteTransitGatewayInput{
		TransitGatewayId: aws.String(tgwID),
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *TransitGatewayReconciler) setConditionTGW(ctx context.Context, tgw *awsv1alpha1.TransitGateway, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&tgw.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: tgw.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, tgw); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *TransitGatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.TransitGateway{}).
		Complete(r)
}
