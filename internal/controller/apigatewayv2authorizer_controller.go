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

// APIGatewayV2AuthorizerAWSAPI is the subset of the API Gateway v2 API used by this controller.
type APIGatewayV2AuthorizerAWSAPI interface {
	GetAuthorizer(ctx context.Context, params *awsapigwv2.GetAuthorizerInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.GetAuthorizerOutput, error)
	CreateAuthorizer(ctx context.Context, params *awsapigwv2.CreateAuthorizerInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.CreateAuthorizerOutput, error)
	UpdateAuthorizer(ctx context.Context, params *awsapigwv2.UpdateAuthorizerInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.UpdateAuthorizerOutput, error)
	DeleteAuthorizer(ctx context.Context, params *awsapigwv2.DeleteAuthorizerInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.DeleteAuthorizerOutput, error)
}

// APIGatewayV2AuthorizerReconciler reconciles APIGatewayV2Authorizer objects.
type APIGatewayV2AuthorizerReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	APIGatewayV2Client APIGatewayV2AuthorizerAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2authorizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2authorizers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2authorizers/finalizers,verbs=update

func (r *APIGatewayV2AuthorizerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.APIGatewayV2Authorizer{}
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
			if err := r.deleteAuthorizer(ctx, obj); err != nil {
				logger.Error(err, "failed to delete APIGatewayV2Authorizer")
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

	if err := r.reconcileAuthorizer(ctx, obj); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionAuthorizer(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *APIGatewayV2AuthorizerReconciler) reconcileAuthorizer(ctx context.Context, obj *awsv1alpha1.APIGatewayV2Authorizer) error {
	apiID, err := resolveAPIGatewayV2APIID(ctx, r.Client, obj.Namespace, obj.Spec.APIRef)
	if err != nil {
		return err
	}

	authorizerURI, err := r.resolveAuthorizerURI(ctx, obj)
	if err != nil {
		return err
	}

	if obj.Status.AuthorizerID != "" {
		_, getErr := r.APIGatewayV2Client.GetAuthorizer(ctx, &awsapigwv2.GetAuthorizerInput{
			ApiId:        aws.String(apiID),
			AuthorizerId: aws.String(obj.Status.AuthorizerID),
		})
		if getErr != nil && !apigwv2helper.IsNotFound(getErr) {
			return fmt.Errorf("get authorizer: %w", getErr)
		}
		if getErr == nil {
			if obj.Status.ObservedGeneration != obj.Generation {
				input := &awsapigwv2.UpdateAuthorizerInput{
					ApiId:          aws.String(apiID),
					AuthorizerId:   aws.String(obj.Status.AuthorizerID),
					Name:           aws.String(obj.Spec.Name),
					AuthorizerType: apigwv2types.AuthorizerType(obj.Spec.AuthorizerType),
				}
				if len(obj.Spec.IdentitySource) > 0 {
					input.IdentitySource = obj.Spec.IdentitySource
				}
				if obj.Spec.JWTConfiguration != nil {
					input.JwtConfiguration = jwtConfigToAWS(obj.Spec.JWTConfiguration)
				}
				if authorizerURI != "" {
					input.AuthorizerUri = aws.String(authorizerURI)
				}
				if obj.Spec.AuthorizerPayloadFormatVersion != "" {
					input.AuthorizerPayloadFormatVersion = aws.String(obj.Spec.AuthorizerPayloadFormatVersion)
				}
				if obj.Spec.AuthorizerResultTTLInSeconds != nil {
					input.AuthorizerResultTtlInSeconds = obj.Spec.AuthorizerResultTTLInSeconds
				}
				if obj.Spec.EnableSimpleResponses {
					input.EnableSimpleResponses = aws.Bool(true)
				}
				if obj.Spec.AuthorizerCredentialsARN != "" {
					input.AuthorizerCredentialsArn = aws.String(obj.Spec.AuthorizerCredentialsARN)
				}
				if _, err := r.APIGatewayV2Client.UpdateAuthorizer(ctx, input); err != nil {
					return fmt.Errorf("update authorizer: %w", err)
				}
			}
			obj.Status.APIID = apiID
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionAuthorizer(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "APIGatewayV2Authorizer reconciled")
		}
		obj.Status.AuthorizerID = ""
	}

	input := &awsapigwv2.CreateAuthorizerInput{
		ApiId:          aws.String(apiID),
		Name:           aws.String(obj.Spec.Name),
		AuthorizerType: apigwv2types.AuthorizerType(obj.Spec.AuthorizerType),
		IdentitySource: obj.Spec.IdentitySource,
	}
	if obj.Spec.JWTConfiguration != nil {
		input.JwtConfiguration = jwtConfigToAWS(obj.Spec.JWTConfiguration)
	}
	if authorizerURI != "" {
		input.AuthorizerUri = aws.String(authorizerURI)
	}
	if obj.Spec.AuthorizerPayloadFormatVersion != "" {
		input.AuthorizerPayloadFormatVersion = aws.String(obj.Spec.AuthorizerPayloadFormatVersion)
	}
	if obj.Spec.AuthorizerResultTTLInSeconds != nil {
		input.AuthorizerResultTtlInSeconds = obj.Spec.AuthorizerResultTTLInSeconds
	}
	if obj.Spec.EnableSimpleResponses {
		input.EnableSimpleResponses = aws.Bool(true)
	}
	if obj.Spec.AuthorizerCredentialsARN != "" {
		input.AuthorizerCredentialsArn = aws.String(obj.Spec.AuthorizerCredentialsARN)
	}

	out, err := r.APIGatewayV2Client.CreateAuthorizer(ctx, input)
	if err != nil {
		return fmt.Errorf("create authorizer: %w", err)
	}

	// Persist the authorizer ID immediately: the AWS resource now exists, and
	// losing the identifier would orphan it on delete.
	obj.Status.AuthorizerID = aws.ToString(out.AuthorizerId)
	obj.Status.APIID = apiID
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist authorizer id after create: %w", err)
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionAuthorizer(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "APIGatewayV2Authorizer created")
}

func jwtConfigToAWS(j *awsv1alpha1.JWTConfiguration) *apigwv2types.JWTConfiguration {
	return &apigwv2types.JWTConfiguration{
		Issuer:   aws.String(j.Issuer),
		Audience: j.Audience,
	}
}

// resolveAuthorizerURI returns the Lambda authorizer invocation URI: the raw
// spec value if set, otherwise built from the referenced Lambda function's ARN.
func (r *APIGatewayV2AuthorizerReconciler) resolveAuthorizerURI(ctx context.Context, obj *awsv1alpha1.APIGatewayV2Authorizer) (string, error) {
	if obj.Spec.AuthorizerURI != "" {
		return obj.Spec.AuthorizerURI, nil
	}
	if obj.Spec.FunctionRef == nil {
		return "", nil
	}
	fnARN, err := resolveLambdaFunctionName(ctx, r.Client, obj.Namespace, "", obj.Spec.FunctionRef)
	if err != nil {
		return "", err
	}
	return lambdaInvocationURI(fnARN), nil
}

func (r *APIGatewayV2AuthorizerReconciler) deleteAuthorizer(ctx context.Context, obj *awsv1alpha1.APIGatewayV2Authorizer) error {
	if obj.Status.AuthorizerID == "" || obj.Status.APIID == "" {
		return nil
	}
	_, err := r.APIGatewayV2Client.DeleteAuthorizer(ctx, &awsapigwv2.DeleteAuthorizerInput{
		ApiId:        aws.String(obj.Status.APIID),
		AuthorizerId: aws.String(obj.Status.AuthorizerID),
	})
	if apigwv2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *APIGatewayV2AuthorizerReconciler) setConditionAuthorizer(ctx context.Context, obj *awsv1alpha1.APIGatewayV2Authorizer, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *APIGatewayV2AuthorizerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.APIGatewayV2Authorizer{}).
		Complete(r)
}
