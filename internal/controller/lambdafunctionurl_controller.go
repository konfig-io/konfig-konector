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
)

// LambdaFunctionURLReconciler reconciles LambdaFunctionURL objects.
type LambdaFunctionURLReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	LambdaClient *awslambda.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdafunctionurls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdafunctionurls/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdafunctionurls/finalizers,verbs=update

func (r *LambdaFunctionURLReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.LambdaFunctionURL{}
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
			if err := r.deleteFunctionURL(ctx, obj); err != nil {
				logger.Error(err, "failed to delete LambdaFunctionURL")
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

	if err := r.reconcileFunctionURL(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionLFU(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func buildCors(spec *awsv1alpha1.LambdaURLCORSConfig) *lambdatypes.Cors {
	if spec == nil {
		return nil
	}
	cors := &lambdatypes.Cors{}
	if spec.AllowCredentials {
		cors.AllowCredentials = aws.Bool(true)
	}
	if len(spec.AllowHeaders) > 0 {
		cors.AllowHeaders = spec.AllowHeaders
	}
	if len(spec.AllowMethods) > 0 {
		cors.AllowMethods = spec.AllowMethods
	}
	if len(spec.AllowOrigins) > 0 {
		cors.AllowOrigins = spec.AllowOrigins
	}
	if len(spec.ExposeHeaders) > 0 {
		cors.ExposeHeaders = spec.ExposeHeaders
	}
	if spec.MaxAge != nil {
		cors.MaxAge = spec.MaxAge
	}
	return cors
}

func (r *LambdaFunctionURLReconciler) reconcileFunctionURL(ctx context.Context, obj *awsv1alpha1.LambdaFunctionURL) error {
	existing, err := r.LambdaClient.GetFunctionUrlConfig(ctx, &awslambda.GetFunctionUrlConfigInput{
		FunctionName: aws.String(obj.Spec.FunctionName),
		Qualifier:    lambdaURLQualifier(obj),
	})
	if err != nil && !lambdahelper.IsNotFound(err) {
		return fmt.Errorf("get lambda function url config: %w", err)
	}

	if err == nil {
		obj.Status.FunctionURL = aws.ToString(existing.FunctionUrl)
		updateInput := &awslambda.UpdateFunctionUrlConfigInput{
			FunctionName: aws.String(obj.Spec.FunctionName),
			AuthType:     lambdatypes.FunctionUrlAuthType(obj.Spec.AuthType),
			Qualifier:    lambdaURLQualifier(obj),
			Cors:         buildCors(obj.Spec.CORS),
		}
		if obj.Spec.InvokeMode != "" {
			updateInput.InvokeMode = lambdatypes.InvokeMode(obj.Spec.InvokeMode)
		}
		out, err := r.LambdaClient.UpdateFunctionUrlConfig(ctx, updateInput)
		if err != nil {
			return fmt.Errorf("update lambda function url config: %w", err)
		}
		obj.Status.FunctionURL = aws.ToString(out.FunctionUrl)
	} else {
		createInput := &awslambda.CreateFunctionUrlConfigInput{
			FunctionName: aws.String(obj.Spec.FunctionName),
			AuthType:     lambdatypes.FunctionUrlAuthType(obj.Spec.AuthType),
			Qualifier:    lambdaURLQualifier(obj),
			Cors:         buildCors(obj.Spec.CORS),
		}
		if obj.Spec.InvokeMode != "" {
			createInput.InvokeMode = lambdatypes.InvokeMode(obj.Spec.InvokeMode)
		}
		out, err := r.LambdaClient.CreateFunctionUrlConfig(ctx, createInput)
		if err != nil {
			return fmt.Errorf("create lambda function url config: %w", err)
		}
		obj.Status.FunctionURL = aws.ToString(out.FunctionUrl)
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionLFU(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "LambdaFunctionURL reconciled")
}

func lambdaURLQualifier(obj *awsv1alpha1.LambdaFunctionURL) *string {
	if obj.Spec.Qualifier == "" {
		return nil
	}
	return aws.String(obj.Spec.Qualifier)
}

func (r *LambdaFunctionURLReconciler) deleteFunctionURL(ctx context.Context, obj *awsv1alpha1.LambdaFunctionURL) error {
	_, err := r.LambdaClient.DeleteFunctionUrlConfig(ctx, &awslambda.DeleteFunctionUrlConfigInput{
		FunctionName: aws.String(obj.Spec.FunctionName),
		Qualifier:    lambdaURLQualifier(obj),
	})
	if lambdahelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *LambdaFunctionURLReconciler) setConditionLFU(ctx context.Context, obj *awsv1alpha1.LambdaFunctionURL, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *LambdaFunctionURLReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.LambdaFunctionURL{}).
		Complete(r)
}
