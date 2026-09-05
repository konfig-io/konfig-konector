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
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsapigw "github.com/aws/aws-sdk-go-v2/service/apigateway"
	apigwtypes "github.com/aws/aws-sdk-go-v2/service/apigateway/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	apigwhelper "github.com/konfig-io/konfig-konector/internal/aws/apigateway"
)

// RestAPIStageAWSAPI is the subset of the API Gateway (REST) API used by this controller.
type RestAPIStageAWSAPI interface {
	GetStage(ctx context.Context, params *awsapigw.GetStageInput, optFns ...func(*awsapigw.Options)) (*awsapigw.GetStageOutput, error)
	CreateStage(ctx context.Context, params *awsapigw.CreateStageInput, optFns ...func(*awsapigw.Options)) (*awsapigw.CreateStageOutput, error)
	UpdateStage(ctx context.Context, params *awsapigw.UpdateStageInput, optFns ...func(*awsapigw.Options)) (*awsapigw.UpdateStageOutput, error)
	DeleteStage(ctx context.Context, params *awsapigw.DeleteStageInput, optFns ...func(*awsapigw.Options)) (*awsapigw.DeleteStageOutput, error)
}

// RestAPIStageReconciler reconciles RestAPIStage objects.
type RestAPIStageReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	APIGatewayClient RestAPIStageAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=restapistages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=restapistages/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=restapistages/finalizers,verbs=update

func (r *RestAPIStageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.RestAPIStage{}
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
				logger.Error(err, "failed to delete RestAPIStage")
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
		_ = r.setConditionRestStage(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *RestAPIStageReconciler) reconcileStage(ctx context.Context, obj *awsv1alpha1.RestAPIStage) error {
	apiID, err := resolveRestAPIID(ctx, r.Client, obj.Namespace, obj.Spec.RestAPIRef)
	if err != nil {
		return err
	}

	deploymentID, err := r.resolveDeploymentID(ctx, obj)
	if err != nil {
		return err
	}

	getOut, getErr := r.APIGatewayClient.GetStage(ctx, &awsapigw.GetStageInput{
		RestApiId: aws.String(apiID),
		StageName: aws.String(obj.Spec.StageName),
	})
	if getErr != nil && !apigwhelper.IsNotFound(getErr) {
		return fmt.Errorf("get stage: %w", getErr)
	}

	if getErr == nil {
		if obj.Status.ObservedGeneration != obj.Generation {
			var ops []apigwtypes.PatchOperation
			if aws.ToString(getOut.DeploymentId) != deploymentID {
				ops = append(ops, apigwtypes.PatchOperation{
					Op: apigwtypes.OpReplace, Path: aws.String("/deploymentId"), Value: aws.String(deploymentID),
				})
			}
			ops = append(ops,
				apigwtypes.PatchOperation{Op: apigwtypes.OpReplace, Path: aws.String("/description"), Value: aws.String(obj.Spec.Description)},
				apigwtypes.PatchOperation{Op: apigwtypes.OpReplace, Path: aws.String("/tracingEnabled"), Value: aws.String(strconv.FormatBool(obj.Spec.TracingEnabled))},
			)
			for k, v := range obj.Spec.Variables {
				ops = append(ops, apigwtypes.PatchOperation{
					Op: apigwtypes.OpReplace, Path: aws.String("/variables/" + k), Value: aws.String(v),
				})
			}
			if _, err := r.APIGatewayClient.UpdateStage(ctx, &awsapigw.UpdateStageInput{
				RestApiId:       aws.String(apiID),
				StageName:       aws.String(obj.Spec.StageName),
				PatchOperations: ops,
			}); err != nil {
				return fmt.Errorf("update stage: %w", err)
			}
		}
		obj.Status.StageName = obj.Spec.StageName
		obj.Status.APIID = apiID
		obj.Status.ObservedGeneration = obj.Generation
		now := metav1.Now()
		obj.Status.LastSyncTime = &now
		return r.setConditionRestStage(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "RestAPIStage reconciled")
	}

	input := &awsapigw.CreateStageInput{
		RestApiId:    aws.String(apiID),
		StageName:    aws.String(obj.Spec.StageName),
		DeploymentId: aws.String(deploymentID),
	}
	if obj.Spec.Description != "" {
		input.Description = aws.String(obj.Spec.Description)
	}
	if len(obj.Spec.Variables) > 0 {
		input.Variables = obj.Spec.Variables
	}
	if obj.Spec.TracingEnabled {
		input.TracingEnabled = true
	}
	if len(obj.Spec.Tags) > 0 {
		input.Tags = obj.Spec.Tags
	}

	out, err := r.APIGatewayClient.CreateStage(ctx, input)
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
	return r.setConditionRestStage(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "RestAPIStage created")
}

func (r *RestAPIStageReconciler) resolveDeploymentID(ctx context.Context, obj *awsv1alpha1.RestAPIStage) (string, error) {
	ref := obj.Spec.DeploymentRef
	if ref.DeploymentID != "" {
		return ref.DeploymentID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("deploymentRef requires name or deploymentId")
	}
	dep := &awsv1alpha1.RestAPIDeployment{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: obj.Namespace}, dep); err != nil {
		return "", err
	}
	if dep.Status.DeploymentID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("RestAPIDeployment %s/%s has no deploymentId yet", obj.Namespace, ref.Name)}
	}
	return dep.Status.DeploymentID, nil
}

func (r *RestAPIStageReconciler) deleteStage(ctx context.Context, obj *awsv1alpha1.RestAPIStage) error {
	apiID := obj.Status.APIID
	if apiID == "" {
		// Status may have been lost before it was persisted; the stage is
		// deterministically addressable via the resolved API ref + spec name.
		var err error
		apiID, err = resolveRestAPIID(ctx, r.Client, obj.Namespace, obj.Spec.RestAPIRef)
		if err != nil {
			// Parent API gone or never created: nothing to delete.
			return nil
		}
	}
	stageName := obj.Status.StageName
	if stageName == "" {
		stageName = obj.Spec.StageName
	}
	_, err := r.APIGatewayClient.DeleteStage(ctx, &awsapigw.DeleteStageInput{
		RestApiId: aws.String(apiID),
		StageName: aws.String(stageName),
	})
	if apigwhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *RestAPIStageReconciler) setConditionRestStage(ctx context.Context, obj *awsv1alpha1.RestAPIStage, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *RestAPIStageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.RestAPIStage{}).
		Complete(r)
}
