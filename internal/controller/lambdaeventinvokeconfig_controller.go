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

// LambdaEventInvokeConfigAWSAPI is the subset of the Lambda API used by this
// controller.
type LambdaEventInvokeConfigAWSAPI interface {
	PutFunctionEventInvokeConfig(ctx context.Context, params *awslambda.PutFunctionEventInvokeConfigInput, optFns ...func(*awslambda.Options)) (*awslambda.PutFunctionEventInvokeConfigOutput, error)
	DeleteFunctionEventInvokeConfig(ctx context.Context, params *awslambda.DeleteFunctionEventInvokeConfigInput, optFns ...func(*awslambda.Options)) (*awslambda.DeleteFunctionEventInvokeConfigOutput, error)
}

// LambdaEventInvokeConfigReconciler reconciles LambdaEventInvokeConfig objects.
type LambdaEventInvokeConfigReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	LambdaClient LambdaEventInvokeConfigAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdaeventinvokeconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdaeventinvokeconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdaeventinvokeconfigs/finalizers,verbs=update

func (r *LambdaEventInvokeConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	eic := &awsv1alpha1.LambdaEventInvokeConfig{}
	if err := r.Get(ctx, req.NamespacedName, eic); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !eic.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(eic, awsv1alpha1.FinalizerName) {
			if shouldAbandon(eic) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(eic, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, eic)
			}
			if err := r.deleteConfig(ctx, eic); err != nil {
				logger.Error(err, "failed to delete event invoke config")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(eic, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, eic)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(eic, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(eic, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, eic); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileConfig(ctx, eic); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, eic, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *LambdaEventInvokeConfigReconciler) reconcileConfig(ctx context.Context, eic *awsv1alpha1.LambdaEventInvokeConfig) error {
	fnName, err := resolveLambdaFunctionName(ctx, r.Client, eic.Namespace, eic.Spec.FunctionName, eic.Spec.FunctionRef)
	if err != nil {
		return err
	}

	input := &awslambda.PutFunctionEventInvokeConfigInput{
		FunctionName: aws.String(fnName),
	}
	if eic.Spec.Qualifier != "" {
		input.Qualifier = aws.String(eic.Spec.Qualifier)
	}
	if eic.Spec.MaximumRetryAttempts != nil {
		input.MaximumRetryAttempts = eic.Spec.MaximumRetryAttempts
	}
	if eic.Spec.MaximumEventAgeInSeconds != nil {
		input.MaximumEventAgeInSeconds = eic.Spec.MaximumEventAgeInSeconds
	}
	if eic.Spec.OnSuccessDestinationARN != "" || eic.Spec.OnFailureDestinationARN != "" {
		dest := &lambdatypes.DestinationConfig{}
		if eic.Spec.OnSuccessDestinationARN != "" {
			dest.OnSuccess = &lambdatypes.OnSuccess{Destination: aws.String(eic.Spec.OnSuccessDestinationARN)}
		}
		if eic.Spec.OnFailureDestinationARN != "" {
			dest.OnFailure = &lambdatypes.OnFailure{Destination: aws.String(eic.Spec.OnFailureDestinationARN)}
		}
		input.DestinationConfig = dest
	}

	// PutFunctionEventInvokeConfig is an idempotent upsert that replaces the
	// whole config, so omitted fields reset to defaults as desired.
	out, err := r.LambdaClient.PutFunctionEventInvokeConfig(ctx, input)
	if err != nil {
		return fmt.Errorf("put function event invoke config: %w", err)
	}
	eic.Status.FunctionARN = aws.ToString(out.FunctionArn)
	if err := persistStatus(ctx, r.Client, eic); err != nil {
		return fmt.Errorf("persist function ARN after put: %w", err)
	}

	eic.Status.ObservedGeneration = eic.Generation
	now := metav1.Now()
	eic.Status.LastSyncTime = &now
	return r.setCondition(ctx, eic, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "event invoke config reconciled")
}

func (r *LambdaEventInvokeConfigReconciler) deleteConfig(ctx context.Context, eic *awsv1alpha1.LambdaEventInvokeConfig) error {
	fnName := eic.Status.FunctionARN
	if fnName == "" {
		var err error
		fnName, err = resolveLambdaFunctionName(ctx, r.Client, eic.Namespace, eic.Spec.FunctionName, eic.Spec.FunctionRef)
		if err != nil {
			// The referenced function CR is gone or never became ready; the
			// config dies with the function, so nothing to delete.
			return nil
		}
	}
	input := &awslambda.DeleteFunctionEventInvokeConfigInput{
		FunctionName: aws.String(fnName),
	}
	// When deleting via the qualified function ARN from status, the qualifier
	// is already embedded; only pass it when using the bare name.
	if eic.Status.FunctionARN == "" && eic.Spec.Qualifier != "" {
		input.Qualifier = aws.String(eic.Spec.Qualifier)
	}
	_, err := r.LambdaClient.DeleteFunctionEventInvokeConfig(ctx, input)
	if lambdahelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *LambdaEventInvokeConfigReconciler) setCondition(ctx context.Context, eic *awsv1alpha1.LambdaEventInvokeConfig, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&eic.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: eic.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, eic); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *LambdaEventInvokeConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.LambdaEventInvokeConfig{}).
		Complete(r)
}
