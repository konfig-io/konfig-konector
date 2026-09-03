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

// APIGatewayV2RouteAWSAPI is the subset of the API Gateway v2 API used by this controller.
type APIGatewayV2RouteAWSAPI interface {
	GetRoute(ctx context.Context, params *awsapigwv2.GetRouteInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.GetRouteOutput, error)
	CreateRoute(ctx context.Context, params *awsapigwv2.CreateRouteInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.CreateRouteOutput, error)
	UpdateRoute(ctx context.Context, params *awsapigwv2.UpdateRouteInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.UpdateRouteOutput, error)
	DeleteRoute(ctx context.Context, params *awsapigwv2.DeleteRouteInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.DeleteRouteOutput, error)
}

// APIGatewayV2RouteReconciler reconciles APIGatewayV2Route objects.
type APIGatewayV2RouteReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	APIGatewayV2Client APIGatewayV2RouteAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2routes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2routes/finalizers,verbs=update

func (r *APIGatewayV2RouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.APIGatewayV2Route{}
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
			if err := r.deleteRoute(ctx, obj); err != nil {
				logger.Error(err, "failed to delete APIGatewayV2Route")
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

	if err := r.reconcileRoute(ctx, obj); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionRoute(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *APIGatewayV2RouteReconciler) reconcileRoute(ctx context.Context, obj *awsv1alpha1.APIGatewayV2Route) error {
	apiID, err := resolveAPIGatewayV2APIID(ctx, r.Client, obj.Namespace, obj.Spec.APIRef)
	if err != nil {
		return err
	}

	target, err := r.resolveTarget(ctx, obj)
	if err != nil {
		return err
	}

	authorizerID, err := r.resolveAuthorizerID(ctx, obj)
	if err != nil {
		return err
	}

	if obj.Status.RouteID != "" {
		_, getErr := r.APIGatewayV2Client.GetRoute(ctx, &awsapigwv2.GetRouteInput{
			ApiId:   aws.String(apiID),
			RouteId: aws.String(obj.Status.RouteID),
		})
		if getErr != nil && !apigwv2helper.IsNotFound(getErr) {
			return fmt.Errorf("get route: %w", getErr)
		}
		if getErr == nil {
			if obj.Status.ObservedGeneration != obj.Generation {
				input := &awsapigwv2.UpdateRouteInput{
					ApiId:    aws.String(apiID),
					RouteId:  aws.String(obj.Status.RouteID),
					RouteKey: aws.String(obj.Spec.RouteKey),
				}
				if target != "" {
					input.Target = aws.String(target)
				}
				if obj.Spec.AuthorizationType != "" {
					input.AuthorizationType = apigwv2types.AuthorizationType(obj.Spec.AuthorizationType)
				}
				if authorizerID != "" {
					input.AuthorizerId = aws.String(authorizerID)
				}
				if len(obj.Spec.AuthorizationScopes) > 0 {
					input.AuthorizationScopes = obj.Spec.AuthorizationScopes
				}
				if obj.Spec.APIKeyRequired {
					input.ApiKeyRequired = aws.Bool(true)
				}
				if obj.Spec.OperationName != "" {
					input.OperationName = aws.String(obj.Spec.OperationName)
				}
				if _, err := r.APIGatewayV2Client.UpdateRoute(ctx, input); err != nil {
					return fmt.Errorf("update route: %w", err)
				}
			}
			obj.Status.APIID = apiID
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionRoute(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "APIGatewayV2Route reconciled")
		}
		obj.Status.RouteID = ""
	}

	input := &awsapigwv2.CreateRouteInput{
		ApiId:    aws.String(apiID),
		RouteKey: aws.String(obj.Spec.RouteKey),
	}
	if target != "" {
		input.Target = aws.String(target)
	}
	if obj.Spec.AuthorizationType != "" {
		input.AuthorizationType = apigwv2types.AuthorizationType(obj.Spec.AuthorizationType)
	}
	if authorizerID != "" {
		input.AuthorizerId = aws.String(authorizerID)
	}
	if len(obj.Spec.AuthorizationScopes) > 0 {
		input.AuthorizationScopes = obj.Spec.AuthorizationScopes
	}
	if obj.Spec.APIKeyRequired {
		input.ApiKeyRequired = aws.Bool(true)
	}
	if obj.Spec.OperationName != "" {
		input.OperationName = aws.String(obj.Spec.OperationName)
	}

	out, err := r.APIGatewayV2Client.CreateRoute(ctx, input)
	if err != nil {
		return fmt.Errorf("create route: %w", err)
	}

	// Persist the route ID immediately: the AWS resource now exists, and
	// losing the identifier would orphan it on delete.
	obj.Status.RouteID = aws.ToString(out.RouteId)
	obj.Status.APIID = apiID
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist route id after create: %w", err)
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionRoute(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "APIGatewayV2Route created")
}

// resolveTarget returns the route target: the raw spec.target if set,
// otherwise "integrations/<id>" from the referenced integration CR.
func (r *APIGatewayV2RouteReconciler) resolveTarget(ctx context.Context, obj *awsv1alpha1.APIGatewayV2Route) (string, error) {
	if obj.Spec.Target != "" {
		return obj.Spec.Target, nil
	}
	if obj.Spec.IntegrationRef == nil {
		return "", nil
	}
	if obj.Spec.IntegrationRef.IntegrationID != "" {
		return "integrations/" + obj.Spec.IntegrationRef.IntegrationID, nil
	}
	integ := &awsv1alpha1.APIGatewayV2Integration{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: obj.Spec.IntegrationRef.Name, Namespace: obj.Namespace}, integ); err != nil {
		return "", err
	}
	if integ.Status.IntegrationID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("APIGatewayV2Integration %s/%s has no integrationId yet", obj.Namespace, obj.Spec.IntegrationRef.Name)}
	}
	return "integrations/" + integ.Status.IntegrationID, nil
}

func (r *APIGatewayV2RouteReconciler) resolveAuthorizerID(ctx context.Context, obj *awsv1alpha1.APIGatewayV2Route) (string, error) {
	if obj.Spec.AuthorizerRef == nil {
		return "", nil
	}
	if obj.Spec.AuthorizerRef.AuthorizerID != "" {
		return obj.Spec.AuthorizerRef.AuthorizerID, nil
	}
	auth := &awsv1alpha1.APIGatewayV2Authorizer{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: obj.Spec.AuthorizerRef.Name, Namespace: obj.Namespace}, auth); err != nil {
		return "", err
	}
	if auth.Status.AuthorizerID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("APIGatewayV2Authorizer %s/%s has no authorizerId yet", obj.Namespace, obj.Spec.AuthorizerRef.Name)}
	}
	return auth.Status.AuthorizerID, nil
}

func (r *APIGatewayV2RouteReconciler) deleteRoute(ctx context.Context, obj *awsv1alpha1.APIGatewayV2Route) error {
	if obj.Status.RouteID == "" || obj.Status.APIID == "" {
		return nil
	}
	_, err := r.APIGatewayV2Client.DeleteRoute(ctx, &awsapigwv2.DeleteRouteInput{
		ApiId:   aws.String(obj.Status.APIID),
		RouteId: aws.String(obj.Status.RouteID),
	})
	if apigwv2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *APIGatewayV2RouteReconciler) setConditionRoute(ctx context.Context, obj *awsv1alpha1.APIGatewayV2Route, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *APIGatewayV2RouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.APIGatewayV2Route{}).
		Complete(r)
}
