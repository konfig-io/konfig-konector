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
	awscf "github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	cfhelper "github.com/konfig-io/konfig-konector/internal/aws/cloudfront"
)

// CloudFrontFunctionReconciler reconciles CloudFrontFunction objects.
type CloudFrontFunctionReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	CloudFrontClient *awscf.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudfrontfunctions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudfrontfunctions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudfrontfunctions/finalizers,verbs=update

func (r *CloudFrontFunctionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.CloudFrontFunction{}
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
			if err := r.deleteCFFunction(ctx, obj); err != nil {
				logger.Error(err, "failed to delete CloudFrontFunction")
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

	if err := r.reconcileCFFunction(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionCFF(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func cfFunctionRuntime(obj *awsv1alpha1.CloudFrontFunction) cftypes.FunctionRuntime {
	if obj.Spec.Runtime != "" {
		return cftypes.FunctionRuntime(obj.Spec.Runtime)
	}
	return cftypes.FunctionRuntimeCloudfrontJs10
}

func (r *CloudFrontFunctionReconciler) reconcileCFFunction(ctx context.Context, obj *awsv1alpha1.CloudFrontFunction) error {
	descOut, err := r.CloudFrontClient.DescribeFunction(ctx, &awscf.DescribeFunctionInput{
		Name: aws.String(obj.Spec.Name),
	})
	if err != nil && !cfhelper.IsNotFound(err) {
		return fmt.Errorf("describe cloudfront function: %w", err)
	}

	comment := obj.Spec.Comment
	if comment == "" {
		comment = obj.Spec.Name
	}

	if err == nil && descOut.FunctionSummary != nil {
		obj.Status.ETag = aws.ToString(descOut.ETag)
		if descOut.FunctionSummary.FunctionMetadata != nil {
			obj.Status.FunctionARN = aws.ToString(descOut.FunctionSummary.FunctionMetadata.FunctionARN)
			obj.Status.FunctionStatus = string(descOut.FunctionSummary.FunctionMetadata.Stage)
		}

		_, err := r.CloudFrontClient.UpdateFunction(ctx, &awscf.UpdateFunctionInput{
			Name:         aws.String(obj.Spec.Name),
			IfMatch:      aws.String(obj.Status.ETag),
			FunctionCode: []byte(obj.Spec.FunctionCode),
			FunctionConfig: &cftypes.FunctionConfig{
				Comment: aws.String(comment),
				Runtime: cfFunctionRuntime(obj),
			},
		})
		if err != nil {
			return fmt.Errorf("update cloudfront function: %w", err)
		}
		obj.Status.ObservedGeneration = obj.Generation
		now := metav1.Now()
		obj.Status.LastSyncTime = &now
		return r.setConditionCFF(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "CloudFrontFunction reconciled")
	}

	out, err := r.CloudFrontClient.CreateFunction(ctx, &awscf.CreateFunctionInput{
		Name:         aws.String(obj.Spec.Name),
		FunctionCode: []byte(obj.Spec.FunctionCode),
		FunctionConfig: &cftypes.FunctionConfig{
			Comment: aws.String(comment),
			Runtime: cfFunctionRuntime(obj),
		},
	})
	if err != nil {
		return fmt.Errorf("create cloudfront function: %w", err)
	}
	if out.FunctionSummary != nil && out.FunctionSummary.FunctionMetadata != nil {
		obj.Status.FunctionARN = aws.ToString(out.FunctionSummary.FunctionMetadata.FunctionARN)
		obj.Status.FunctionStatus = string(out.FunctionSummary.FunctionMetadata.Stage)
	}
	obj.Status.ETag = aws.ToString(out.ETag)

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionCFF(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "CloudFrontFunction created")
}

func (r *CloudFrontFunctionReconciler) deleteCFFunction(ctx context.Context, obj *awsv1alpha1.CloudFrontFunction) error {
	etag := obj.Status.ETag
	if etag == "" {
		descOut, err := r.CloudFrontClient.DescribeFunction(ctx, &awscf.DescribeFunctionInput{
			Name: aws.String(obj.Spec.Name),
		})
		if cfhelper.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		etag = aws.ToString(descOut.ETag)
	}
	_, err := r.CloudFrontClient.DeleteFunction(ctx, &awscf.DeleteFunctionInput{
		Name:    aws.String(obj.Spec.Name),
		IfMatch: aws.String(etag),
	})
	if cfhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *CloudFrontFunctionReconciler) setConditionCFF(ctx context.Context, obj *awsv1alpha1.CloudFrontFunction, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *CloudFrontFunctionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CloudFrontFunction{}).
		Complete(r)
}
