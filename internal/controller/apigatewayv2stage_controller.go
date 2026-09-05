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

// APIGatewayV2StageAWSAPI is the subset of the API Gateway v2 API used by this controller.
type APIGatewayV2StageAWSAPI interface {
	GetStage(ctx context.Context, params *awsapigwv2.GetStageInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.GetStageOutput, error)
	CreateStage(ctx context.Context, params *awsapigwv2.CreateStageInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.CreateStageOutput, error)
	UpdateStage(ctx context.Context, params *awsapigwv2.UpdateStageInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.UpdateStageOutput, error)
	DeleteStage(ctx context.Context, params *awsapigwv2.DeleteStageInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.DeleteStageOutput, error)
}

// APIGatewayV2StageReconciler reconciles APIGatewayV2Stage objects.
type APIGatewayV2StageReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	APIGatewayV2Client APIGatewayV2StageAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2stages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2stages/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2stages/finalizers,verbs=update

func (r *APIGatewayV2StageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.APIGatewayV2Stage{}
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
			if err := r.deleteStage(ctx, obj); err != nil {
				logger.Error(err, "failed to delete APIGatewayV2Stage")
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

	if err := r.reconcileStage(ctx, obj); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionStage(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *APIGatewayV2StageReconciler) reconcileStage(ctx context.Context, obj *awsv1alpha1.APIGatewayV2Stage) error {
	apiID, err := resolveAPIGatewayV2APIID(ctx, r.Client, obj.Namespace, obj.Spec.APIRef)
	if err != nil {
		return err
	}

	_, getErr := r.APIGatewayV2Client.GetStage(ctx, &awsapigwv2.GetStageInput{
		ApiId:     aws.String(apiID),
		StageName: aws.String(obj.Spec.StageName),
	})
	if getErr != nil && !apigwv2helper.IsNotFound(getErr) {
		return fmt.Errorf("get stage: %w", getErr)
	}

	if getErr == nil {
		// Stage exists; update it when the spec changed.
		if obj.Status.ObservedGeneration != obj.Generation {
			input := &awsapigwv2.UpdateStageInput{
				ApiId:      aws.String(apiID),
				StageName:  aws.String(obj.Spec.StageName),
				AutoDeploy: aws.Bool(obj.Spec.AutoDeploy),
			}
			if obj.Spec.Description != "" {
				input.Description = aws.String(obj.Spec.Description)
			}
			if len(obj.Spec.StageVariables) > 0 {
				input.StageVariables = obj.Spec.StageVariables
			}
			if obj.Spec.DefaultRouteSettings != nil {
				input.DefaultRouteSettings = routeSettingsToAWS(obj.Spec.DefaultRouteSettings)
			}
			if _, err := r.APIGatewayV2Client.UpdateStage(ctx, input); err != nil {
				return fmt.Errorf("update stage: %w", err)
			}
		}
		obj.Status.StageName = obj.Spec.StageName
		obj.Status.APIID = apiID
		obj.Status.ObservedGeneration = obj.Generation
		now := metav1.Now()
		obj.Status.LastSyncTime = &now
		return r.setConditionStage(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "APIGatewayV2Stage reconciled")
	}

	input := &awsapigwv2.CreateStageInput{
		ApiId:     aws.String(apiID),
		StageName: aws.String(obj.Spec.StageName),
	}
	if obj.Spec.AutoDeploy {
		input.AutoDeploy = aws.Bool(true)
	}
	if obj.Spec.Description != "" {
		input.Description = aws.String(obj.Spec.Description)
	}
	if len(obj.Spec.StageVariables) > 0 {
		input.StageVariables = obj.Spec.StageVariables
	}
	if obj.Spec.DefaultRouteSettings != nil {
		input.DefaultRouteSettings = routeSettingsToAWS(obj.Spec.DefaultRouteSettings)
	}
	if len(obj.Spec.Tags) > 0 {
		input.Tags = obj.Spec.Tags
	}

	out, err := r.APIGatewayV2Client.CreateStage(ctx, input)
	if err != nil {
		return fmt.Errorf("create stage: %w", err)
	}

	// Persist the identifiers immediately: the AWS resource now exists, and
	// losing them would orphan it on delete.
	obj.Status.StageName = aws.ToString(out.StageName)
	obj.Status.APIID = apiID
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist stage name after create: %w", err)
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionStage(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "APIGatewayV2Stage created")
}

func routeSettingsToAWS(rs *awsv1alpha1.APIGatewayV2RouteSettings) *apigwv2types.RouteSettings {
	return &apigwv2types.RouteSettings{
		ThrottlingBurstLimit:   rs.ThrottlingBurstLimit,
		ThrottlingRateLimit:    rs.ThrottlingRateLimit,
		DetailedMetricsEnabled: rs.DetailedMetricsEnabled,
	}
}

func (r *APIGatewayV2StageReconciler) deleteStage(ctx context.Context, obj *awsv1alpha1.APIGatewayV2Stage) error {
	apiID := obj.Status.APIID
	if apiID == "" {
		// Status may have been lost before it was persisted; the stage is
		// deterministically addressable via the resolved API ref + spec name.
		var err error
		apiID, err = resolveAPIGatewayV2APIID(ctx, r.Client, obj.Namespace, obj.Spec.APIRef)
		if err != nil {
			// Parent API gone or never created: nothing to delete.
			return nil
		}
	}
	stageName := obj.Status.StageName
	if stageName == "" {
		stageName = obj.Spec.StageName
	}
	_, err := r.APIGatewayV2Client.DeleteStage(ctx, &awsapigwv2.DeleteStageInput{
		ApiId:     aws.String(apiID),
		StageName: aws.String(stageName),
	})
	if apigwv2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *APIGatewayV2StageReconciler) setConditionStage(ctx context.Context, obj *awsv1alpha1.APIGatewayV2Stage, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *APIGatewayV2StageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.APIGatewayV2Stage{}).
		Complete(r)
}
