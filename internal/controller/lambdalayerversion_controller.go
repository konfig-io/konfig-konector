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

// LambdaLayerVersionReconciler reconciles LambdaLayerVersion objects.
type LambdaLayerVersionReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	LambdaClient *multi.Lambda
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdalayerversions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdalayerversions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdalayerversions/finalizers,verbs=update

func (r *LambdaLayerVersionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.LambdaLayerVersion{}
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
			if err := r.deleteLayerVersion(ctx, obj); err != nil {
				logger.Error(err, "failed to delete LambdaLayerVersion")
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

	if err := r.reconcileLayerVersion(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionLLV(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *LambdaLayerVersionReconciler) reconcileLayerVersion(ctx context.Context, obj *awsv1alpha1.LambdaLayerVersion) error {
	if obj.Status.LayerVersionARN != "" {
		_, err := r.LambdaClient.GetLayerVersion(ctx, &awslambda.GetLayerVersionInput{
			LayerName:     aws.String(obj.Spec.LayerName),
			VersionNumber: aws.Int64(obj.Status.Version),
		})
		if err != nil && !lambdahelper.IsNotFound(err) {
			return fmt.Errorf("get lambda layer version: %w", err)
		}
		if err == nil {
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionLLV(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "LambdaLayerVersion reconciled")
		}
		obj.Status.LayerVersionARN = ""
		obj.Status.Version = 0
	}

	content := &lambdatypes.LayerVersionContentInput{}
	if obj.Spec.Content.S3Bucket != "" {
		content.S3Bucket = aws.String(obj.Spec.Content.S3Bucket)
	}
	if obj.Spec.Content.S3Key != "" {
		content.S3Key = aws.String(obj.Spec.Content.S3Key)
	}
	if obj.Spec.Content.S3ObjectVersion != "" {
		content.S3ObjectVersion = aws.String(obj.Spec.Content.S3ObjectVersion)
	}

	input := &awslambda.PublishLayerVersionInput{
		LayerName: aws.String(obj.Spec.LayerName),
		Content:   content,
	}
	if obj.Spec.Description != "" {
		input.Description = aws.String(obj.Spec.Description)
	}
	if obj.Spec.LicenseInfo != "" {
		input.LicenseInfo = aws.String(obj.Spec.LicenseInfo)
	}
	for _, rt := range obj.Spec.CompatibleRuntimes {
		input.CompatibleRuntimes = append(input.CompatibleRuntimes, lambdatypes.Runtime(rt))
	}
	for _, arch := range obj.Spec.CompatibleArchitectures {
		input.CompatibleArchitectures = append(input.CompatibleArchitectures, lambdatypes.Architecture(arch))
	}

	out, err := r.LambdaClient.PublishLayerVersion(ctx, input)
	if err != nil {
		return fmt.Errorf("publish lambda layer version: %w", err)
	}

	obj.Status.LayerVersionARN = aws.ToString(out.LayerVersionArn)
	obj.Status.Version = out.Version
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionLLV(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "LambdaLayerVersion created")
}

func (r *LambdaLayerVersionReconciler) deleteLayerVersion(ctx context.Context, obj *awsv1alpha1.LambdaLayerVersion) error {
	if obj.Status.LayerVersionARN == "" {
		return nil
	}
	_, err := r.LambdaClient.DeleteLayerVersion(ctx, &awslambda.DeleteLayerVersionInput{
		LayerName:     aws.String(obj.Spec.LayerName),
		VersionNumber: aws.Int64(obj.Status.Version),
	})
	if lambdahelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *LambdaLayerVersionReconciler) setConditionLLV(ctx context.Context, obj *awsv1alpha1.LambdaLayerVersion, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *LambdaLayerVersionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.LambdaLayerVersion{}).
		Complete(r)
}
