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
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	apigwv2helper "github.com/konfig-io/konfig-konector/internal/aws/apigatewayv2"
)

// APIGatewayV2IntegrationAWSAPI is the subset of the API Gateway v2 API used by this controller.
type APIGatewayV2IntegrationAWSAPI interface {
	GetIntegration(ctx context.Context, params *awsapigwv2.GetIntegrationInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.GetIntegrationOutput, error)
	CreateIntegration(ctx context.Context, params *awsapigwv2.CreateIntegrationInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.CreateIntegrationOutput, error)
	UpdateIntegration(ctx context.Context, params *awsapigwv2.UpdateIntegrationInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.UpdateIntegrationOutput, error)
	DeleteIntegration(ctx context.Context, params *awsapigwv2.DeleteIntegrationInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.DeleteIntegrationOutput, error)
}

// APIGatewayV2IntegrationReconciler reconciles APIGatewayV2Integration objects.
type APIGatewayV2IntegrationReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	APIGatewayV2Client APIGatewayV2IntegrationAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2integrations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2integrations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2integrations/finalizers,verbs=update

func (r *APIGatewayV2IntegrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.APIGatewayV2Integration{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
			if shouldAbandon(obj) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, obj)
			}
			if err := r.deleteIntegration(ctx, obj); err != nil {
				logger.Error(err, "failed to delete APIGatewayV2Integration")
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
	}

	if err := r.reconcileIntegration(ctx, obj); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionIntegration(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *APIGatewayV2IntegrationReconciler) reconcileIntegration(ctx context.Context, obj *awsv1alpha1.APIGatewayV2Integration) error {
	apiID, err := resolveAPIGatewayV2APIID(ctx, r.Client, obj.Namespace, obj.Spec.APIRef)
	if err != nil {
		return err
	}

	uri, connectionID, err := r.resolveIntegrationTargets(ctx, obj)
	if err != nil {
		return err
	}

	if obj.Status.IntegrationID != "" {
		_, getErr := r.APIGatewayV2Client.GetIntegration(ctx, &awsapigwv2.GetIntegrationInput{
			ApiId:         aws.String(apiID),
			IntegrationId: aws.String(obj.Status.IntegrationID),
		})
		if getErr != nil && !apigwv2helper.IsNotFound(getErr) {
			return fmt.Errorf("get integration: %w", getErr)
		}
		if getErr == nil {
			if obj.Status.ObservedGeneration != obj.Generation {
				input := &awsapigwv2.UpdateIntegrationInput{
					ApiId:           aws.String(apiID),
					IntegrationId:   aws.String(obj.Status.IntegrationID),
					IntegrationType: apigwv2types.IntegrationType(obj.Spec.IntegrationType),
				}
				if uri != "" {
					input.IntegrationUri = aws.String(uri)
				}
				if obj.Spec.IntegrationMethod != "" {
					input.IntegrationMethod = aws.String(obj.Spec.IntegrationMethod)
				}
				if obj.Spec.PayloadFormatVersion != "" {
					input.PayloadFormatVersion = aws.String(obj.Spec.PayloadFormatVersion)
				}
				if obj.Spec.Description != "" {
					input.Description = aws.String(obj.Spec.Description)
				}
				if obj.Spec.ConnectionType != "" {
					input.ConnectionType = apigwv2types.ConnectionType(obj.Spec.ConnectionType)
				}
				if connectionID != "" {
					input.ConnectionId = aws.String(connectionID)
				}
				if obj.Spec.CredentialsARN != "" {
					input.CredentialsArn = aws.String(obj.Spec.CredentialsARN)
				}
				if len(obj.Spec.RequestParameters) > 0 {
					input.RequestParameters = obj.Spec.RequestParameters
				}
				if obj.Spec.TimeoutInMillis != nil {
					input.TimeoutInMillis = obj.Spec.TimeoutInMillis
				}
				if _, err := r.APIGatewayV2Client.UpdateIntegration(ctx, input); err != nil {
					return fmt.Errorf("update integration: %w", err)
				}
			}
			obj.Status.APIID = apiID
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionIntegration(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "APIGatewayV2Integration reconciled")
		}
		obj.Status.IntegrationID = ""
	}

	input := &awsapigwv2.CreateIntegrationInput{
		ApiId:           aws.String(apiID),
		IntegrationType: apigwv2types.IntegrationType(obj.Spec.IntegrationType),
	}
	if uri != "" {
		input.IntegrationUri = aws.String(uri)
	}
	if obj.Spec.IntegrationMethod != "" {
		input.IntegrationMethod = aws.String(obj.Spec.IntegrationMethod)
	}
	if obj.Spec.PayloadFormatVersion != "" {
		input.PayloadFormatVersion = aws.String(obj.Spec.PayloadFormatVersion)
	}
	if obj.Spec.Description != "" {
		input.Description = aws.String(obj.Spec.Description)
	}
	if obj.Spec.ConnectionType != "" {
		input.ConnectionType = apigwv2types.ConnectionType(obj.Spec.ConnectionType)
	}
	if connectionID != "" {
		input.ConnectionId = aws.String(connectionID)
	}
	if obj.Spec.CredentialsARN != "" {
		input.CredentialsArn = aws.String(obj.Spec.CredentialsARN)
	}
	if len(obj.Spec.RequestParameters) > 0 {
		input.RequestParameters = obj.Spec.RequestParameters
	}
	if obj.Spec.TimeoutInMillis != nil {
		input.TimeoutInMillis = obj.Spec.TimeoutInMillis
	}

	out, err := r.APIGatewayV2Client.CreateIntegration(ctx, input)
	if err != nil {
		return fmt.Errorf("create integration: %w", err)
	}

	// Persist the integration ID immediately: the AWS resource now exists,
	// and losing the identifier would orphan it on delete.
	obj.Status.IntegrationID = aws.ToString(out.IntegrationId)
	obj.Status.APIID = apiID
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist integration id after create: %w", err)
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionIntegration(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "APIGatewayV2Integration created")
}

// resolveIntegrationTargets resolves the integration URI (raw value or Lambda
// function ref) and the VPC link connection ID (raw value or VPC link ref).
func (r *APIGatewayV2IntegrationReconciler) resolveIntegrationTargets(ctx context.Context, obj *awsv1alpha1.APIGatewayV2Integration) (uri string, connectionID string, err error) {
	uri = obj.Spec.IntegrationURI
	if uri == "" && obj.Spec.FunctionRef != nil {
		uri, err = resolveLambdaFunctionName(ctx, r.Client, obj.Namespace, "", obj.Spec.FunctionRef)
		if err != nil {
			return "", "", err
		}
	}

	connectionID = obj.Spec.ConnectionID
	if connectionID == "" && obj.Spec.VPCLinkRef != nil {
		vl := &awsv1alpha1.APIGatewayV2VpcLink{}
		ns := obj.Spec.VPCLinkRef.Namespace
		if ns == "" {
			ns = obj.Namespace
		}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: obj.Spec.VPCLinkRef.Name, Namespace: ns}, vl); err != nil {
			return "", "", err
		}
		if vl.Status.VPCLinkID == "" {
			return "", "", &dependencyNotReady{msg: fmt.Sprintf("APIGatewayV2VpcLink %s/%s has no vpcLinkId yet", ns, obj.Spec.VPCLinkRef.Name)}
		}
		connectionID = vl.Status.VPCLinkID
	}
	return uri, connectionID, nil
}

func (r *APIGatewayV2IntegrationReconciler) deleteIntegration(ctx context.Context, obj *awsv1alpha1.APIGatewayV2Integration) error {
	if obj.Status.IntegrationID == "" || obj.Status.APIID == "" {
		return nil
	}
	_, err := r.APIGatewayV2Client.DeleteIntegration(ctx, &awsapigwv2.DeleteIntegrationInput{
		ApiId:         aws.String(obj.Status.APIID),
		IntegrationId: aws.String(obj.Status.IntegrationID),
	})
	if apigwv2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *APIGatewayV2IntegrationReconciler) setConditionIntegration(ctx context.Context, obj *awsv1alpha1.APIGatewayV2Integration, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *APIGatewayV2IntegrationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.APIGatewayV2Integration{}).
		Complete(r)
}
