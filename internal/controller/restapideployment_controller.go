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
	awsapigw "github.com/aws/aws-sdk-go-v2/service/apigateway"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	apigwhelper "github.com/konfig-io/konfig-konector/internal/aws/apigateway"
)

// RestAPIDeploymentAWSAPI is the subset of the API Gateway (REST) API used by this controller.
type RestAPIDeploymentAWSAPI interface {
	GetDeployment(ctx context.Context, params *awsapigw.GetDeploymentInput, optFns ...func(*awsapigw.Options)) (*awsapigw.GetDeploymentOutput, error)
	CreateDeployment(ctx context.Context, params *awsapigw.CreateDeploymentInput, optFns ...func(*awsapigw.Options)) (*awsapigw.CreateDeploymentOutput, error)
	DeleteDeployment(ctx context.Context, params *awsapigw.DeleteDeploymentInput, optFns ...func(*awsapigw.Options)) (*awsapigw.DeleteDeploymentOutput, error)
}

// RestAPIDeploymentReconciler reconciles RestAPIDeployment objects.
type RestAPIDeploymentReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	APIGatewayClient RestAPIDeploymentAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=restapideployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=restapideployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=restapideployments/finalizers,verbs=update

func (r *RestAPIDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.RestAPIDeployment{}
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
			if err := r.deleteDeployment(ctx, obj); err != nil {
				logger.Error(err, "failed to delete RestAPIDeployment")
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

	if err := r.reconcileDeployment(ctx, obj); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionDeployment(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *RestAPIDeploymentReconciler) reconcileDeployment(ctx context.Context, obj *awsv1alpha1.RestAPIDeployment) error {
	if obj.Status.DeploymentID != "" {
		// Deployments are immutable snapshots: never update, only verify.
		_, getErr := r.APIGatewayClient.GetDeployment(ctx, &awsapigw.GetDeploymentInput{
			RestApiId:    aws.String(obj.Status.APIID),
			DeploymentId: aws.String(obj.Status.DeploymentID),
		})
		if getErr != nil && !apigwhelper.IsNotFound(getErr) {
			return fmt.Errorf("get deployment: %w", getErr)
		}
		if getErr == nil {
			if obj.Status.ObservedGeneration != obj.Generation {
				// Spec changed after creation: deployments cannot be updated.
				return r.setConditionDeployment(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse,
					awsv1alpha1.ReasonUpdateNotSupported, "RestAPIDeployment is an immutable snapshot; create a new deployment instead")
			}
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionDeployment(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "RestAPIDeployment reconciled")
		}
		obj.Status.DeploymentID = ""
	}

	apiID, err := resolveRestAPIID(ctx, r.Client, obj.Namespace, obj.Spec.RestAPIRef)
	if err != nil {
		return err
	}

	input := &awsapigw.CreateDeploymentInput{
		RestApiId: aws.String(apiID),
	}
	if obj.Spec.StageName != "" {
		input.StageName = aws.String(obj.Spec.StageName)
	}
	if obj.Spec.Description != "" {
		input.Description = aws.String(obj.Spec.Description)
	}

	out, err := r.APIGatewayClient.CreateDeployment(ctx, input)
	if err != nil {
		return fmt.Errorf("create deployment: %w", err)
	}

	// Persist the deployment ID immediately: the AWS resource now exists, and
	// losing the identifier would orphan it on delete.
	obj.Status.DeploymentID = aws.ToString(out.Id)
	obj.Status.APIID = apiID
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist deployment id after create: %w", err)
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionDeployment(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "RestAPIDeployment created")
}

func (r *RestAPIDeploymentReconciler) deleteDeployment(ctx context.Context, obj *awsv1alpha1.RestAPIDeployment) error {
	if obj.Status.DeploymentID == "" || obj.Status.APIID == "" {
		return nil
	}
	_, err := r.APIGatewayClient.DeleteDeployment(ctx, &awsapigw.DeleteDeploymentInput{
		RestApiId:    aws.String(obj.Status.APIID),
		DeploymentId: aws.String(obj.Status.DeploymentID),
	})
	if apigwhelper.IsNotFound(err) {
		return nil
	}
	// Deleting the parent RestAPI removes its deployments; a BadRequest for a
	// deployment still referenced by a stage must surface as an error.
	return err
}

func (r *RestAPIDeploymentReconciler) setConditionDeployment(ctx context.Context, obj *awsv1alpha1.RestAPIDeployment, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *RestAPIDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.RestAPIDeployment{}).
		Complete(r)
}
