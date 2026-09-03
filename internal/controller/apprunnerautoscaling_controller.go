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
	awsapprunner "github.com/aws/aws-sdk-go-v2/service/apprunner"
	apprunnertypes "github.com/aws/aws-sdk-go-v2/service/apprunner/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	apprunnerhelper "github.com/konfig-io/konfig-konector/internal/aws/apprunner"
)

// AppRunnerAutoScalingAWSAPI is the subset of the App Runner API used by this controller.
type AppRunnerAutoScalingAWSAPI interface {
	CreateAutoScalingConfiguration(ctx context.Context, params *awsapprunner.CreateAutoScalingConfigurationInput, optFns ...func(*awsapprunner.Options)) (*awsapprunner.CreateAutoScalingConfigurationOutput, error)
	DeleteAutoScalingConfiguration(ctx context.Context, params *awsapprunner.DeleteAutoScalingConfigurationInput, optFns ...func(*awsapprunner.Options)) (*awsapprunner.DeleteAutoScalingConfigurationOutput, error)
}

// AppRunnerAutoScalingReconciler reconciles AppRunnerAutoScaling objects.
type AppRunnerAutoScalingReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	AppRunnerClient AppRunnerAutoScalingAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=apprunnerautoscalings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=apprunnerautoscalings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=apprunnerautoscalings/finalizers,verbs=update

func (r *AppRunnerAutoScalingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	asc := &awsv1alpha1.AppRunnerAutoScaling{}
	if err := r.Get(ctx, req.NamespacedName, asc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !asc.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(asc, awsv1alpha1.FinalizerName) {
			if shouldAbandon(asc) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(asc, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, asc)
			}
			if err := r.deleteAutoScalingConfiguration(ctx, asc); err != nil {
				logger.Error(err, "failed to delete App Runner auto scaling configuration")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(asc, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, asc)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(asc, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(asc, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, asc); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileAutoScalingConfiguration(ctx, asc); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, asc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *AppRunnerAutoScalingReconciler) reconcileAutoScalingConfiguration(ctx context.Context, asc *awsv1alpha1.AppRunnerAutoScaling) error {
	if asc.Status.AutoScalingConfigurationARN != "" {
		// Auto scaling configurations are immutable versioned resources:
		// spec changes after creation cannot be applied in place.
		if asc.Status.ObservedGeneration != asc.Generation {
			return r.setCondition(ctx, asc, awsv1alpha1.ConditionReady, metav1.ConditionFalse,
				awsv1alpha1.ReasonUpdateNotSupported,
				"App Runner auto scaling configurations are immutable; recreate the resource to change it")
		}
		now := metav1.Now()
		asc.Status.LastSyncTime = &now
		return r.setCondition(ctx, asc, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "App Runner auto scaling configuration active")
	}

	createIn := &awsapprunner.CreateAutoScalingConfigurationInput{
		AutoScalingConfigurationName: aws.String(asc.Spec.Name),
	}
	if asc.Spec.MaxConcurrency > 0 {
		createIn.MaxConcurrency = aws.Int32(asc.Spec.MaxConcurrency)
	}
	if asc.Spec.MaxSize > 0 {
		createIn.MaxSize = aws.Int32(asc.Spec.MaxSize)
	}
	if asc.Spec.MinSize > 0 {
		createIn.MinSize = aws.Int32(asc.Spec.MinSize)
	}
	for k, v := range asc.Spec.Tags {
		k, v := k, v
		createIn.Tags = append(createIn.Tags, apprunnertypes.Tag{Key: &k, Value: &v})
	}

	created, err := r.AppRunnerClient.CreateAutoScalingConfiguration(ctx, createIn)
	if err != nil {
		return fmt.Errorf("create App Runner auto scaling configuration: %w", err)
	}
	if created.AutoScalingConfiguration != nil {
		asc.Status.AutoScalingConfigurationARN = aws.ToString(created.AutoScalingConfiguration.AutoScalingConfigurationArn)
		asc.Status.Revision = aws.ToInt32(created.AutoScalingConfiguration.AutoScalingConfigurationRevision)
	}
	// Persist the ARN immediately: the AWS resource now exists, and losing
	// the identifier would create a duplicate revision on retry.
	if err := persistStatus(ctx, r.Client, asc); err != nil {
		return fmt.Errorf("persist auto scaling configuration ARN after create: %w", err)
	}
	asc.Status.ObservedGeneration = asc.Generation
	now := metav1.Now()
	asc.Status.LastSyncTime = &now
	return r.setCondition(ctx, asc, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "App Runner auto scaling configuration created")
}

func (r *AppRunnerAutoScalingReconciler) deleteAutoScalingConfiguration(ctx context.Context, asc *awsv1alpha1.AppRunnerAutoScaling) error {
	arn := asc.Status.AutoScalingConfigurationARN
	if arn == "" {
		// The delete API accepts a partial ARN suffix by name; without the
		// full ARN or the account/region there is no unambiguous handle, so
		// nothing to delete.
		return nil
	}
	_, err := r.AppRunnerClient.DeleteAutoScalingConfiguration(ctx, &awsapprunner.DeleteAutoScalingConfigurationInput{
		AutoScalingConfigurationArn: aws.String(arn),
	})
	if apprunnerhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *AppRunnerAutoScalingReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.AppRunnerAutoScaling, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *AppRunnerAutoScalingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.AppRunnerAutoScaling{}).
		Complete(r)
}
