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

// LambdaAliasReconciler reconciles LambdaAlias objects.
type LambdaAliasReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	LambdaClient *multi.Lambda
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdaaliases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdaaliases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdaaliases/finalizers,verbs=update

func (r *LambdaAliasReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.LambdaAlias{}
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
			if err := r.deleteAlias(ctx, obj); err != nil {
				logger.Error(err, "failed to delete LambdaAlias")
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

	if err := r.reconcileAlias(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionLA(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *LambdaAliasReconciler) reconcileAlias(ctx context.Context, obj *awsv1alpha1.LambdaAlias) error {
	existing, err := r.LambdaClient.GetAlias(ctx, &awslambda.GetAliasInput{
		FunctionName: aws.String(obj.Spec.FunctionName),
		Name:         aws.String(obj.Spec.Name),
	})
	if err != nil && !lambdahelper.IsNotFound(err) {
		return fmt.Errorf("get lambda alias: %w", err)
	}

	if err == nil {
		obj.Status.AliasARN = aws.ToString(existing.AliasArn)
		// Update alias
		updateInput := &awslambda.UpdateAliasInput{
			FunctionName:    aws.String(obj.Spec.FunctionName),
			Name:            aws.String(obj.Spec.Name),
			FunctionVersion: aws.String(obj.Spec.FunctionVersion),
		}
		if obj.Spec.Description != "" {
			updateInput.Description = aws.String(obj.Spec.Description)
		}
		if obj.Spec.RoutingConfig != nil && len(obj.Spec.RoutingConfig.AdditionalVersionWeights) > 0 {
			updateInput.RoutingConfig = &lambdatypes.AliasRoutingConfiguration{
				AdditionalVersionWeights: obj.Spec.RoutingConfig.AdditionalVersionWeights,
			}
		}
		out, err := r.LambdaClient.UpdateAlias(ctx, updateInput)
		if err != nil {
			return fmt.Errorf("update lambda alias: %w", err)
		}
		obj.Status.AliasARN = aws.ToString(out.AliasArn)
	} else {
		createInput := &awslambda.CreateAliasInput{
			FunctionName:    aws.String(obj.Spec.FunctionName),
			Name:            aws.String(obj.Spec.Name),
			FunctionVersion: aws.String(obj.Spec.FunctionVersion),
		}
		if obj.Spec.Description != "" {
			createInput.Description = aws.String(obj.Spec.Description)
		}
		if obj.Spec.RoutingConfig != nil && len(obj.Spec.RoutingConfig.AdditionalVersionWeights) > 0 {
			createInput.RoutingConfig = &lambdatypes.AliasRoutingConfiguration{
				AdditionalVersionWeights: obj.Spec.RoutingConfig.AdditionalVersionWeights,
			}
		}
		out, err := r.LambdaClient.CreateAlias(ctx, createInput)
		if err != nil {
			return fmt.Errorf("create lambda alias: %w", err)
		}
		obj.Status.AliasARN = aws.ToString(out.AliasArn)
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionLA(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "LambdaAlias reconciled")
}

func (r *LambdaAliasReconciler) deleteAlias(ctx context.Context, obj *awsv1alpha1.LambdaAlias) error {
	_, err := r.LambdaClient.DeleteAlias(ctx, &awslambda.DeleteAliasInput{
		FunctionName: aws.String(obj.Spec.FunctionName),
		Name:         aws.String(obj.Spec.Name),
	})
	if lambdahelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *LambdaAliasReconciler) setConditionLA(ctx context.Context, obj *awsv1alpha1.LambdaAlias, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *LambdaAliasReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.LambdaAlias{}).
		Complete(r)
}
