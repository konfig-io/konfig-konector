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
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	lambdahelper "github.com/konfig-io/konfig-konector/internal/aws/lambda"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// LambdaCodeSigningConfigReconciler reconciles LambdaCodeSigningConfig objects.
type LambdaCodeSigningConfigReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	LambdaClient *multi.Lambda
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdacodesigningconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdacodesigningconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdacodesigningconfigs/finalizers,verbs=update

func (r *LambdaCodeSigningConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.LambdaCodeSigningConfig{}
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
			if err := r.deleteCodeSigningConfig(ctx, obj); err != nil {
				logger.Error(err, "failed to delete LambdaCodeSigningConfig")
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

	if err := r.reconcileCodeSigningConfig(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionLCSC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *LambdaCodeSigningConfigReconciler) reconcileCodeSigningConfig(ctx context.Context, obj *awsv1alpha1.LambdaCodeSigningConfig) error {
	if obj.Status.CodeSigningConfigARN != "" {
		existing, err := r.LambdaClient.GetCodeSigningConfig(ctx, &awslambda.GetCodeSigningConfigInput{
			CodeSigningConfigArn: aws.String(obj.Status.CodeSigningConfigARN),
		})
		if err != nil && !lambdahelper.IsNotFound(err) {
			return fmt.Errorf("get lambda code signing config: %w", err)
		}
		if err == nil {
			policies := &lambdatypes.CodeSigningPolicies{}
			if obj.Spec.UntrustedArtifactOnDeployment != "" {
				policies.UntrustedArtifactOnDeployment = lambdatypes.CodeSigningPolicy(obj.Spec.UntrustedArtifactOnDeployment)
			}
			updateInput := &awslambda.UpdateCodeSigningConfigInput{
				CodeSigningConfigArn: aws.String(aws.ToString(existing.CodeSigningConfig.CodeSigningConfigArn)),
				AllowedPublishers: &lambdatypes.AllowedPublishers{
					SigningProfileVersionArns: obj.Spec.AllowedPublisherARNs,
				},
				CodeSigningPolicies: policies,
			}
			if obj.Spec.Description != "" {
				updateInput.Description = aws.String(obj.Spec.Description)
			}
			_, err := r.LambdaClient.UpdateCodeSigningConfig(ctx, updateInput)
			if err != nil {
				return fmt.Errorf("update lambda code signing config: %w", err)
			}
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionLCSC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "LambdaCodeSigningConfig reconciled")
		}
		obj.Status.CodeSigningConfigARN = ""
		obj.Status.CodeSigningConfigID = ""
	}

	policies := &lambdatypes.CodeSigningPolicies{}
	if obj.Spec.UntrustedArtifactOnDeployment != "" {
		policies.UntrustedArtifactOnDeployment = lambdatypes.CodeSigningPolicy(obj.Spec.UntrustedArtifactOnDeployment)
	}
	createInput := &awslambda.CreateCodeSigningConfigInput{
		AllowedPublishers: &lambdatypes.AllowedPublishers{
			SigningProfileVersionArns: obj.Spec.AllowedPublisherARNs,
		},
		CodeSigningPolicies: policies,
	}
	if obj.Spec.Description != "" {
		createInput.Description = aws.String(obj.Spec.Description)
	}
	out, err := r.LambdaClient.CreateCodeSigningConfig(ctx, createInput)
	if err != nil {
		return fmt.Errorf("create lambda code signing config: %w", err)
	}

	obj.Status.CodeSigningConfigARN = aws.ToString(out.CodeSigningConfig.CodeSigningConfigArn)
	obj.Status.CodeSigningConfigID = aws.ToString(out.CodeSigningConfig.CodeSigningConfigId)
	// Persist the ARN immediately: the AWS resource now exists, and losing the
	// identifier would orphan it or create a duplicate on retry.
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist code signing config ARN after create: %w", err)
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionLCSC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "LambdaCodeSigningConfig created")
}

func (r *LambdaCodeSigningConfigReconciler) deleteCodeSigningConfig(ctx context.Context, obj *awsv1alpha1.LambdaCodeSigningConfig) error {
	if obj.Status.CodeSigningConfigARN == "" {
		return nil
	}
	_, err := r.LambdaClient.DeleteCodeSigningConfig(ctx, &awslambda.DeleteCodeSigningConfigInput{
		CodeSigningConfigArn: aws.String(obj.Status.CodeSigningConfigARN),
	})
	if lambdahelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *LambdaCodeSigningConfigReconciler) setConditionLCSC(ctx context.Context, obj *awsv1alpha1.LambdaCodeSigningConfig, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *LambdaCodeSigningConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.LambdaCodeSigningConfig{}).
		Complete(r)
}
