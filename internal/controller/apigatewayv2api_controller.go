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

// APIGatewayV2APIAWSAPI is the subset of the API Gateway v2 API used by this controller.
type APIGatewayV2APIAWSAPI interface {
	GetApi(ctx context.Context, params *awsapigwv2.GetApiInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.GetApiOutput, error)
	CreateApi(ctx context.Context, params *awsapigwv2.CreateApiInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.CreateApiOutput, error)
	UpdateApi(ctx context.Context, params *awsapigwv2.UpdateApiInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.UpdateApiOutput, error)
	DeleteApi(ctx context.Context, params *awsapigwv2.DeleteApiInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.DeleteApiOutput, error)
}

// APIGatewayV2APIReconciler reconciles APIGatewayV2API objects.
type APIGatewayV2APIReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	APIGatewayV2Client APIGatewayV2APIAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2apis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2apis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2apis/finalizers,verbs=update

func (r *APIGatewayV2APIReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.APIGatewayV2API{}
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
			if err := r.deleteAPI(ctx, obj); err != nil {
				logger.Error(err, "failed to delete APIGatewayV2API")
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

	if err := r.reconcileAPI(ctx, obj); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionAPI(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *APIGatewayV2APIReconciler) reconcileAPI(ctx context.Context, obj *awsv1alpha1.APIGatewayV2API) error {
	if obj.Status.APIID != "" {
		getOut, err := r.APIGatewayV2Client.GetApi(ctx, &awsapigwv2.GetApiInput{
			ApiId: aws.String(obj.Status.APIID),
		})
		if err != nil && !apigwv2helper.IsNotFound(err) {
			return fmt.Errorf("get api: %w", err)
		}
		if err == nil {
			// Update the API only when the spec changed.
			if obj.Status.ObservedGeneration != obj.Generation {
				input := &awsapigwv2.UpdateApiInput{
					ApiId: aws.String(obj.Status.APIID),
					Name:  aws.String(obj.Spec.Name),
				}
				if obj.Spec.Description != "" {
					input.Description = aws.String(obj.Spec.Description)
				}
				if obj.Spec.RouteSelectionExpression != "" {
					input.RouteSelectionExpression = aws.String(obj.Spec.RouteSelectionExpression)
				}
				if obj.Spec.APIKeySelectionExpression != "" {
					input.ApiKeySelectionExpression = aws.String(obj.Spec.APIKeySelectionExpression)
				}
				if obj.Spec.CORSConfiguration != nil {
					input.CorsConfiguration = corsToAWS(obj.Spec.CORSConfiguration)
				}
				input.DisableExecuteApiEndpoint = aws.Bool(obj.Spec.DisableExecuteAPIEndpoint)
				if _, err := r.APIGatewayV2Client.UpdateApi(ctx, input); err != nil {
					return fmt.Errorf("update api: %w", err)
				}
			}
			obj.Status.APIEndpoint = aws.ToString(getOut.ApiEndpoint)
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionAPI(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "APIGatewayV2API reconciled")
		}
		// Recorded API vanished in AWS; recreate it.
		obj.Status.APIID = ""
		obj.Status.APIEndpoint = ""
	}

	input := &awsapigwv2.CreateApiInput{
		Name:         aws.String(obj.Spec.Name),
		ProtocolType: apigwv2types.ProtocolType(obj.Spec.ProtocolType),
	}
	if obj.Spec.Description != "" {
		input.Description = aws.String(obj.Spec.Description)
	}
	if obj.Spec.RouteSelectionExpression != "" {
		input.RouteSelectionExpression = aws.String(obj.Spec.RouteSelectionExpression)
	}
	if obj.Spec.APIKeySelectionExpression != "" {
		input.ApiKeySelectionExpression = aws.String(obj.Spec.APIKeySelectionExpression)
	}
	if obj.Spec.CORSConfiguration != nil {
		input.CorsConfiguration = corsToAWS(obj.Spec.CORSConfiguration)
	}
	if obj.Spec.DisableExecuteAPIEndpoint {
		input.DisableExecuteApiEndpoint = aws.Bool(true)
	}
	if len(obj.Spec.Tags) > 0 {
		input.Tags = obj.Spec.Tags
	}

	out, err := r.APIGatewayV2Client.CreateApi(ctx, input)
	if err != nil {
		return fmt.Errorf("create api: %w", err)
	}

	// Persist the API ID immediately: the AWS resource now exists, and losing
	// the identifier would orphan it on delete.
	obj.Status.APIID = aws.ToString(out.ApiId)
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist api id after create: %w", err)
	}

	obj.Status.APIEndpoint = aws.ToString(out.ApiEndpoint)
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionAPI(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "APIGatewayV2API created")
}

func corsToAWS(c *awsv1alpha1.APIGatewayV2CorsConfiguration) *apigwv2types.Cors {
	return &apigwv2types.Cors{
		AllowCredentials: c.AllowCredentials,
		AllowHeaders:     c.AllowHeaders,
		AllowMethods:     c.AllowMethods,
		AllowOrigins:     c.AllowOrigins,
		ExposeHeaders:    c.ExposeHeaders,
		MaxAge:           c.MaxAge,
	}
}

func (r *APIGatewayV2APIReconciler) deleteAPI(ctx context.Context, obj *awsv1alpha1.APIGatewayV2API) error {
	if obj.Status.APIID == "" {
		// API IDs are AWS-generated and names are not unique, so there is no
		// unambiguous spec-based lookup. Nothing was recorded => nothing to delete.
		return nil
	}
	_, err := r.APIGatewayV2Client.DeleteApi(ctx, &awsapigwv2.DeleteApiInput{
		ApiId: aws.String(obj.Status.APIID),
	})
	if apigwv2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *APIGatewayV2APIReconciler) setConditionAPI(ctx context.Context, obj *awsv1alpha1.APIGatewayV2API, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *APIGatewayV2APIReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.APIGatewayV2API{}).
		Complete(r)
}
