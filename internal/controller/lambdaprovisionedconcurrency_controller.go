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

// LambdaProvisionedConcurrencyAWSAPI is the subset of the Lambda API used by
// this controller.
type LambdaProvisionedConcurrencyAWSAPI interface {
	PutProvisionedConcurrencyConfig(ctx context.Context, params *awslambda.PutProvisionedConcurrencyConfigInput, optFns ...func(*awslambda.Options)) (*awslambda.PutProvisionedConcurrencyConfigOutput, error)
	DeleteProvisionedConcurrencyConfig(ctx context.Context, params *awslambda.DeleteProvisionedConcurrencyConfigInput, optFns ...func(*awslambda.Options)) (*awslambda.DeleteProvisionedConcurrencyConfigOutput, error)
}

// LambdaProvisionedConcurrencyReconciler reconciles LambdaProvisionedConcurrency objects.
type LambdaProvisionedConcurrencyReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	LambdaClient LambdaProvisionedConcurrencyAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdaprovisionedconcurrencies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdaprovisionedconcurrencies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdaprovisionedconcurrencies/finalizers,verbs=update

func (r *LambdaProvisionedConcurrencyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	pc := &awsv1alpha1.LambdaProvisionedConcurrency{}
	if err := r.Get(ctx, req.NamespacedName, pc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !pc.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(pc, awsv1alpha1.FinalizerName) {
			if shouldAbandon(pc) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(pc, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, pc)
			}
			if err := r.deleteConfig(ctx, pc); err != nil {
				logger.Error(err, "failed to delete provisioned concurrency config")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(pc, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, pc)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(pc, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(pc, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, pc); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileConfig(ctx, pc); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, pc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *LambdaProvisionedConcurrencyReconciler) reconcileConfig(ctx context.Context, pc *awsv1alpha1.LambdaProvisionedConcurrency) error {
	fnName, err := resolveLambdaFunctionName(ctx, r.Client, pc.Namespace, pc.Spec.FunctionName, pc.Spec.FunctionRef)
	if err != nil {
		return err
	}

	// PutProvisionedConcurrencyConfig is an idempotent upsert.
	out, err := r.LambdaClient.PutProvisionedConcurrencyConfig(ctx, &awslambda.PutProvisionedConcurrencyConfigInput{
		FunctionName:                    aws.String(fnName),
		Qualifier:                       aws.String(pc.Spec.Qualifier),
		ProvisionedConcurrentExecutions: aws.Int32(pc.Spec.ProvisionedConcurrentExecutions),
	})
	if err != nil {
		return fmt.Errorf("put provisioned concurrency config: %w", err)
	}
	pc.Status.FunctionName = fnName
	pc.Status.AllocatedConcurrentExecutions = aws.ToInt32(out.AllocatedProvisionedConcurrentExecutions)
	pc.Status.Status = string(out.Status)
	// Persist the resolved function name immediately so delete can find the
	// config even if the CR's ref resolution breaks later.
	if err := persistStatus(ctx, r.Client, pc); err != nil {
		return fmt.Errorf("persist function name after put: %w", err)
	}

	pc.Status.ObservedGeneration = pc.Generation
	now := metav1.Now()
	pc.Status.LastSyncTime = &now
	return r.setCondition(ctx, pc, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "provisioned concurrency config reconciled")
}

func (r *LambdaProvisionedConcurrencyReconciler) deleteConfig(ctx context.Context, pc *awsv1alpha1.LambdaProvisionedConcurrency) error {
	fnName := pc.Status.FunctionName
	if fnName == "" {
		var err error
		fnName, err = resolveLambdaFunctionName(ctx, r.Client, pc.Namespace, pc.Spec.FunctionName, pc.Spec.FunctionRef)
		if err != nil {
			// The referenced function CR is gone or never became ready; the
			// config dies with the function version, so nothing to delete.
			return nil
		}
	}
	_, err := r.LambdaClient.DeleteProvisionedConcurrencyConfig(ctx, &awslambda.DeleteProvisionedConcurrencyConfigInput{
		FunctionName: aws.String(fnName),
		Qualifier:    aws.String(pc.Spec.Qualifier),
	})
	if lambdahelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *LambdaProvisionedConcurrencyReconciler) setCondition(ctx context.Context, pc *awsv1alpha1.LambdaProvisionedConcurrency, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&pc.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: pc.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, pc); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *LambdaProvisionedConcurrencyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.LambdaProvisionedConcurrency{}).
		Complete(r)
}
