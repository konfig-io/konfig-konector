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
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// EgressOnlyIGWReconciler reconciles EgressOnlyIGW objects.
type EgressOnlyIGWReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client *multi.EC2
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=egressonlyigws,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=egressonlyigws/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=egressonlyigws/finalizers,verbs=update

func (r *EgressOnlyIGWReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	igw := &awsv1alpha1.EgressOnlyIGW{}
	if err := r.Get(ctx, req.NamespacedName, igw); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, igw); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !igw.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(igw, awsv1alpha1.FinalizerName) {
			if shouldAbandon(igw) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(igw, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, igw)
			}
			if err := r.deleteEgressOnlyIGW(ctx, igw); err != nil {
				logger.Error(err, "failed to delete EgressOnlyIGW")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(igw, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, igw)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(igw, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(igw, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, igw); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileEgressOnlyIGW(ctx, igw); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionEOIGW(ctx, igw, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *EgressOnlyIGWReconciler) reconcileEgressOnlyIGW(ctx context.Context, igw *awsv1alpha1.EgressOnlyIGW) error {
	vpcID, err := r.resolveVPCIDEOIGW(ctx, igw.Namespace, &igw.Spec.VPCRef)
	if err != nil {
		return err
	}

	if igw.Status.EgressOnlyIGWID != "" {
		out, err := r.EC2Client.DescribeEgressOnlyInternetGateways(ctx, &awsec2.DescribeEgressOnlyInternetGatewaysInput{
			EgressOnlyInternetGatewayIds: []string{igw.Status.EgressOnlyIGWID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return fmt.Errorf("describe egress-only igw: %w", err)
		}
		if err == nil && len(out.EgressOnlyInternetGateways) > 0 {
			if len(igw.Spec.Tags) > 0 {
				_, _ = r.EC2Client.CreateTags(ctx, &awsec2.CreateTagsInput{
					Resources: []string{igw.Status.EgressOnlyIGWID},
					Tags:      ec2helper.TagsFromMap(igw.Spec.Tags),
				})
			}
			igw.Status.ObservedGeneration = igw.Generation
			now := metav1.Now()
			igw.Status.LastSyncTime = &now
			return r.setConditionEOIGW(ctx, igw, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "EgressOnlyIGW reconciled")
		}
		igw.Status.EgressOnlyIGWID = ""
	}

	out, err := r.EC2Client.CreateEgressOnlyInternetGateway(ctx, &awsec2.CreateEgressOnlyInternetGatewayInput{
		VpcId: aws.String(vpcID),
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeEgressOnlyInternetGateway,
				Tags:         ec2helper.TagsFromMap(igw.Spec.Tags),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("create egress-only igw: %w", err)
	}

	igw.Status.EgressOnlyIGWID = aws.ToString(out.EgressOnlyInternetGateway.EgressOnlyInternetGatewayId)
	if err := persistStatus(ctx, r.Client, igw); err != nil {
		return fmt.Errorf("persist EgressOnlyIGW ID after create: %w", err)
	}
	igw.Status.ObservedGeneration = igw.Generation
	now := metav1.Now()
	igw.Status.LastSyncTime = &now
	return r.setConditionEOIGW(ctx, igw, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "EgressOnlyIGW created")
}

func (r *EgressOnlyIGWReconciler) resolveVPCIDEOIGW(ctx context.Context, namespace string, ref *awsv1alpha1.VPCResourceRef) (string, error) {
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

func (r *EgressOnlyIGWReconciler) deleteEgressOnlyIGW(ctx context.Context, igw *awsv1alpha1.EgressOnlyIGW) error {
	igwID := igw.Status.EgressOnlyIGWID
	if igwID == "" {
		// Fallback: the status write may have been lost after create.
		// Look up by the tags the create path applied; without tags the
		// gateway is indistinguishable from others, so give up.
		if len(igw.Spec.Tags) == 0 {
			return nil
		}
		filters := make([]ec2types.Filter, 0, len(igw.Spec.Tags))
		for k, v := range igw.Spec.Tags {
			filters = append(filters, ec2types.Filter{Name: aws.String("tag:" + k), Values: []string{v}})
		}
		out, err := r.EC2Client.DescribeEgressOnlyInternetGateways(ctx, &awsec2.DescribeEgressOnlyInternetGatewaysInput{Filters: filters})
		if err != nil {
			return fmt.Errorf("lookup egress-only igw by tags: %w", err)
		}
		if len(out.EgressOnlyInternetGateways) != 1 {
			return nil
		}
		igwID = aws.ToString(out.EgressOnlyInternetGateways[0].EgressOnlyInternetGatewayId)
	}
	_, err := r.EC2Client.DeleteEgressOnlyInternetGateway(ctx, &awsec2.DeleteEgressOnlyInternetGatewayInput{
		EgressOnlyInternetGatewayId: aws.String(igwID),
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *EgressOnlyIGWReconciler) setConditionEOIGW(ctx context.Context, igw *awsv1alpha1.EgressOnlyIGW, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&igw.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: igw.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, igw); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *EgressOnlyIGWReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.EgressOnlyIGW{}).
		Complete(r)
}
