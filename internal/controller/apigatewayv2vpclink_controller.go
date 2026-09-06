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
	awsapigwv2 "github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	apigwv2types "github.com/aws/aws-sdk-go-v2/service/apigatewayv2/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	apigwv2helper "github.com/konfig-io/konfig-konector/internal/aws/apigatewayv2"
)

// APIGatewayV2VpcLinkAWSAPI is the subset of the API Gateway v2 API used by this controller.
type APIGatewayV2VpcLinkAWSAPI interface {
	GetVpcLink(ctx context.Context, params *awsapigwv2.GetVpcLinkInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.GetVpcLinkOutput, error)
	CreateVpcLink(ctx context.Context, params *awsapigwv2.CreateVpcLinkInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.CreateVpcLinkOutput, error)
	DeleteVpcLink(ctx context.Context, params *awsapigwv2.DeleteVpcLinkInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.DeleteVpcLinkOutput, error)
}

// APIGatewayV2VpcLinkReconciler reconciles APIGatewayV2VpcLink objects.
type APIGatewayV2VpcLinkReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	APIGatewayV2Client APIGatewayV2VpcLinkAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2vpclinks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2vpclinks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2vpclinks/finalizers,verbs=update

func (r *APIGatewayV2VpcLinkReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.APIGatewayV2VpcLink{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, obj); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !obj.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
			if shouldAbandon(obj) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, obj)
			}
			if err := r.deleteVpcLink(ctx, obj); err != nil {
				logger.Error(err, "failed to delete APIGatewayV2VpcLink")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, obj)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(obj, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	res, err := r.reconcileVpcLink(ctx, obj)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionVpcLink(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return res, nil
}

func (r *APIGatewayV2VpcLinkReconciler) reconcileVpcLink(ctx context.Context, obj *awsv1alpha1.APIGatewayV2VpcLink) (ctrl.Result, error) {
	if obj.Status.VPCLinkID != "" {
		getOut, err := r.APIGatewayV2Client.GetVpcLink(ctx, &awsapigwv2.GetVpcLinkInput{
			VpcLinkId: aws.String(obj.Status.VPCLinkID),
		})
		if err != nil && !apigwv2helper.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("get vpc link: %w", err)
		}
		if err == nil {
			obj.Status.VPCLinkStatus = string(getOut.VpcLinkStatus)
			// VPC links are immutable in this modeling (subnets cannot change
			// after creation); surface spec drift as UpdateNotSupported.
			if obj.Status.ObservedGeneration != 0 && obj.Status.ObservedGeneration != obj.Generation {
				return requeueResult(), r.setConditionVpcLink(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse,
					awsv1alpha1.ReasonUpdateNotSupported, "APIGatewayV2VpcLink subnets/security groups cannot be updated; recreate the resource")
			}
			if getOut.VpcLinkStatus == apigwv2types.VpcLinkStatusPending {
				_ = r.setConditionVpcLink(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonCreated, "VPC link is still PENDING")
				return requeueAPIGWPolling, nil
			}
			if getOut.VpcLinkStatus == apigwv2types.VpcLinkStatusFailed {
				return ctrl.Result{}, fmt.Errorf("vpc link failed: %s", aws.ToString(getOut.VpcLinkStatusMessage))
			}
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return requeueResult(), r.setConditionVpcLink(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "APIGatewayV2VpcLink reconciled")
		}
		obj.Status.VPCLinkID = ""
		obj.Status.VPCLinkStatus = ""
	}

	subnetIDs, err := resolveSubnetIDs(ctx, r.Client, obj.Namespace, obj.Spec.SubnetRefs)
	if err != nil {
		return ctrl.Result{}, err
	}
	sgIDs, err := resolveSGIDs(ctx, r.Client, obj.Namespace, obj.Spec.SecurityGroupRefs)
	if err != nil {
		return ctrl.Result{}, err
	}

	input := &awsapigwv2.CreateVpcLinkInput{
		Name:      aws.String(obj.Spec.Name),
		SubnetIds: subnetIDs,
	}
	if len(sgIDs) > 0 {
		input.SecurityGroupIds = sgIDs
	}
	if len(obj.Spec.Tags) > 0 {
		input.Tags = obj.Spec.Tags
	}

	out, err := r.APIGatewayV2Client.CreateVpcLink(ctx, input)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create vpc link: %w", err)
	}

	// Persist the VPC link ID immediately: the AWS resource now exists, and
	// losing the identifier would orphan it on delete.
	obj.Status.VPCLinkID = aws.ToString(out.VpcLinkId)
	obj.Status.VPCLinkStatus = string(out.VpcLinkStatus)
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist vpc link id after create: %w", err)
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	if err := r.setConditionVpcLink(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonCreated, "APIGatewayV2VpcLink created; waiting for AVAILABLE"); err != nil {
		return ctrl.Result{}, err
	}
	return requeueAPIGWPolling, nil
}

func (r *APIGatewayV2VpcLinkReconciler) deleteVpcLink(ctx context.Context, obj *awsv1alpha1.APIGatewayV2VpcLink) error {
	if obj.Status.VPCLinkID == "" {
		// VPC link IDs are AWS-generated and names are not unique: no
		// unambiguous spec-based lookup exists. Nothing recorded => nothing to delete.
		return nil
	}
	_, err := r.APIGatewayV2Client.DeleteVpcLink(ctx, &awsapigwv2.DeleteVpcLinkInput{
		VpcLinkId: aws.String(obj.Status.VPCLinkID),
	})
	if apigwv2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *APIGatewayV2VpcLinkReconciler) setConditionVpcLink(ctx context.Context, obj *awsv1alpha1.APIGatewayV2VpcLink, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *APIGatewayV2VpcLinkReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.APIGatewayV2VpcLink{}).
		Complete(r)
}
