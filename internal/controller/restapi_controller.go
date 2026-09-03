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
	apigwtypes "github.com/aws/aws-sdk-go-v2/service/apigateway/types"
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

// RestAPIAWSAPI is the subset of the API Gateway (REST) API used by this controller.
type RestAPIAWSAPI interface {
	GetRestApi(ctx context.Context, params *awsapigw.GetRestApiInput, optFns ...func(*awsapigw.Options)) (*awsapigw.GetRestApiOutput, error)
	CreateRestApi(ctx context.Context, params *awsapigw.CreateRestApiInput, optFns ...func(*awsapigw.Options)) (*awsapigw.CreateRestApiOutput, error)
	UpdateRestApi(ctx context.Context, params *awsapigw.UpdateRestApiInput, optFns ...func(*awsapigw.Options)) (*awsapigw.UpdateRestApiOutput, error)
	PutRestApi(ctx context.Context, params *awsapigw.PutRestApiInput, optFns ...func(*awsapigw.Options)) (*awsapigw.PutRestApiOutput, error)
	DeleteRestApi(ctx context.Context, params *awsapigw.DeleteRestApiInput, optFns ...func(*awsapigw.Options)) (*awsapigw.DeleteRestApiOutput, error)
	GetResources(ctx context.Context, params *awsapigw.GetResourcesInput, optFns ...func(*awsapigw.Options)) (*awsapigw.GetResourcesOutput, error)
}

// RestAPIReconciler reconciles RestAPI objects.
type RestAPIReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	APIGatewayClient RestAPIAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=restapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=restapis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=restapis/finalizers,verbs=update

func (r *RestAPIReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.RestAPI{}
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
			if err := r.deleteRestAPI(ctx, obj); err != nil {
				logger.Error(err, "failed to delete RestAPI")
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

	if err := r.reconcileRestAPI(ctx, obj); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionRestAPI(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *RestAPIReconciler) reconcileRestAPI(ctx context.Context, obj *awsv1alpha1.RestAPI) error {
	if obj.Status.APIID != "" {
		_, getErr := r.APIGatewayClient.GetRestApi(ctx, &awsapigw.GetRestApiInput{
			RestApiId: aws.String(obj.Status.APIID),
		})
		if getErr != nil && !apigwhelper.IsNotFound(getErr) {
			return fmt.Errorf("get rest api: %w", getErr)
		}
		if getErr == nil {
			if obj.Status.ObservedGeneration != obj.Generation {
				if err := r.updateRestAPI(ctx, obj); err != nil {
					return err
				}
			}
			return r.finishSync(ctx, obj, awsv1alpha1.ReasonSynced, "RestAPI reconciled")
		}
		obj.Status.APIID = ""
		obj.Status.RootResourceID = ""
	}

	input := &awsapigw.CreateRestApiInput{
		Name: aws.String(obj.Spec.Name),
	}
	if obj.Spec.Description != "" {
		input.Description = aws.String(obj.Spec.Description)
	}
	if len(obj.Spec.EndpointTypes) > 0 {
		input.EndpointConfiguration = endpointConfigToAWS(obj.Spec.EndpointTypes)
	}
	if obj.Spec.DisableExecuteAPIEndpoint {
		input.DisableExecuteApiEndpoint = true
	}
	if obj.Spec.MinimumCompressionSize != nil {
		input.MinimumCompressionSize = obj.Spec.MinimumCompressionSize
	}
	if obj.Spec.Policy != "" {
		input.Policy = aws.String(obj.Spec.Policy)
	}
	if len(obj.Spec.Tags) > 0 {
		input.Tags = obj.Spec.Tags
	}

	out, err := r.APIGatewayClient.CreateRestApi(ctx, input)
	if err != nil {
		return fmt.Errorf("create rest api: %w", err)
	}

	// Persist the API ID immediately: the AWS resource now exists, and losing
	// the identifier would orphan it on delete.
	obj.Status.APIID = aws.ToString(out.Id)
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist rest api id after create: %w", err)
	}
	obj.Status.RootResourceID = aws.ToString(out.RootResourceId)

	// Import the OpenAPI body, if provided, to materialise the full API.
	if obj.Spec.Body != "" {
		if err := r.putBody(ctx, obj); err != nil {
			return err
		}
	}

	return r.finishSync(ctx, obj, awsv1alpha1.ReasonCreated, "RestAPI created")
}

// updateRestAPI applies name/description patch operations and re-imports the
// OpenAPI body (overwrite mode) when one is declared.
func (r *RestAPIReconciler) updateRestAPI(ctx context.Context, obj *awsv1alpha1.RestAPI) error {
	patchOps := []apigwtypes.PatchOperation{
		{Op: apigwtypes.OpReplace, Path: aws.String("/name"), Value: aws.String(obj.Spec.Name)},
		{Op: apigwtypes.OpReplace, Path: aws.String("/description"), Value: aws.String(obj.Spec.Description)},
	}
	if _, err := r.APIGatewayClient.UpdateRestApi(ctx, &awsapigw.UpdateRestApiInput{
		RestApiId:       aws.String(obj.Status.APIID),
		PatchOperations: patchOps,
	}); err != nil {
		return fmt.Errorf("update rest api: %w", err)
	}
	if obj.Spec.Body != "" {
		if err := r.putBody(ctx, obj); err != nil {
			return err
		}
	}
	return nil
}

// putBody overwrites the API definition with the declared OpenAPI body.
func (r *RestAPIReconciler) putBody(ctx context.Context, obj *awsv1alpha1.RestAPI) error {
	if _, err := r.APIGatewayClient.PutRestApi(ctx, &awsapigw.PutRestApiInput{
		RestApiId: aws.String(obj.Status.APIID),
		Mode:      apigwtypes.PutModeOverwrite,
		Body:      []byte(obj.Spec.Body),
	}); err != nil {
		return fmt.Errorf("put rest api body: %w", err)
	}
	return nil
}

func (r *RestAPIReconciler) finishSync(ctx context.Context, obj *awsv1alpha1.RestAPI, reason, message string) error {
	if obj.Status.RootResourceID == "" {
		// Look up the root ("/") resource ID for downstream consumers.
		resOut, err := r.APIGatewayClient.GetResources(ctx, &awsapigw.GetResourcesInput{
			RestApiId: aws.String(obj.Status.APIID),
		})
		if err == nil {
			for _, res := range resOut.Items {
				if aws.ToString(res.Path) == "/" {
					obj.Status.RootResourceID = aws.ToString(res.Id)
					break
				}
			}
		}
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionRestAPI(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, reason, message)
}

func endpointConfigToAWS(types []string) *apigwtypes.EndpointConfiguration {
	cfg := &apigwtypes.EndpointConfiguration{}
	for _, t := range types {
		cfg.Types = append(cfg.Types, apigwtypes.EndpointType(t))
	}
	return cfg
}

func (r *RestAPIReconciler) deleteRestAPI(ctx context.Context, obj *awsv1alpha1.RestAPI) error {
	if obj.Status.APIID == "" {
		// REST API IDs are AWS-generated and names are not unique: no
		// unambiguous spec-based lookup exists. Nothing recorded => nothing to delete.
		return nil
	}
	_, err := r.APIGatewayClient.DeleteRestApi(ctx, &awsapigw.DeleteRestApiInput{
		RestApiId: aws.String(obj.Status.APIID),
	})
	if apigwhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *RestAPIReconciler) setConditionRestAPI(ctx context.Context, obj *awsv1alpha1.RestAPI, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *RestAPIReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.RestAPI{}).
		Complete(r)
}
