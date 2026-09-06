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
	awsapigwv2 "github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	apigwv2helper "github.com/konfig-io/konfig-konector/internal/aws/apigatewayv2"
)

// APIGatewayV2ApiMappingAWSAPI is the subset of the API Gateway v2 API used by this controller.
type APIGatewayV2ApiMappingAWSAPI interface {
	GetApiMapping(ctx context.Context, params *awsapigwv2.GetApiMappingInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.GetApiMappingOutput, error)
	CreateApiMapping(ctx context.Context, params *awsapigwv2.CreateApiMappingInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.CreateApiMappingOutput, error)
	UpdateApiMapping(ctx context.Context, params *awsapigwv2.UpdateApiMappingInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.UpdateApiMappingOutput, error)
	DeleteApiMapping(ctx context.Context, params *awsapigwv2.DeleteApiMappingInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.DeleteApiMappingOutput, error)
}

// APIGatewayV2ApiMappingReconciler reconciles APIGatewayV2ApiMapping objects.
type APIGatewayV2ApiMappingReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	APIGatewayV2Client APIGatewayV2ApiMappingAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2apimappings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2apimappings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2apimappings/finalizers,verbs=update

func (r *APIGatewayV2ApiMappingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.APIGatewayV2ApiMapping{}
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
			if err := r.deleteApiMapping(ctx, obj); err != nil {
				logger.Error(err, "failed to delete APIGatewayV2ApiMapping")
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
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileApiMapping(ctx, obj); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionMapping(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *APIGatewayV2ApiMappingReconciler) reconcileApiMapping(ctx context.Context, obj *awsv1alpha1.APIGatewayV2ApiMapping) error {
	apiID, err := resolveAPIGatewayV2APIID(ctx, r.Client, obj.Namespace, obj.Spec.APIRef)
	if err != nil {
		return err
	}

	domainName, err := r.resolveDomainName(ctx, obj)
	if err != nil {
		return err
	}

	if obj.Status.APIMappingID != "" {
		_, getErr := r.APIGatewayV2Client.GetApiMapping(ctx, &awsapigwv2.GetApiMappingInput{
			ApiMappingId: aws.String(obj.Status.APIMappingID),
			DomainName:   aws.String(domainName),
		})
		if getErr != nil && !apigwv2helper.IsNotFound(getErr) {
			return fmt.Errorf("get api mapping: %w", getErr)
		}
		if getErr == nil {
			if obj.Status.ObservedGeneration != obj.Generation {
				input := &awsapigwv2.UpdateApiMappingInput{
					ApiId:        aws.String(apiID),
					ApiMappingId: aws.String(obj.Status.APIMappingID),
					DomainName:   aws.String(domainName),
					Stage:        aws.String(obj.Spec.Stage),
				}
				if obj.Spec.APIMappingKey != "" {
					input.ApiMappingKey = aws.String(obj.Spec.APIMappingKey)
				}
				if _, err := r.APIGatewayV2Client.UpdateApiMapping(ctx, input); err != nil {
					return fmt.Errorf("update api mapping: %w", err)
				}
			}
			obj.Status.DomainName = domainName
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionMapping(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "APIGatewayV2ApiMapping reconciled")
		}
		obj.Status.APIMappingID = ""
	}

	input := &awsapigwv2.CreateApiMappingInput{
		ApiId:      aws.String(apiID),
		DomainName: aws.String(domainName),
		Stage:      aws.String(obj.Spec.Stage),
	}
	if obj.Spec.APIMappingKey != "" {
		input.ApiMappingKey = aws.String(obj.Spec.APIMappingKey)
	}

	out, err := r.APIGatewayV2Client.CreateApiMapping(ctx, input)
	if err != nil {
		return fmt.Errorf("create api mapping: %w", err)
	}

	// Persist the mapping ID immediately: the AWS resource now exists, and
	// losing the identifier would orphan it on delete.
	obj.Status.APIMappingID = aws.ToString(out.ApiMappingId)
	obj.Status.DomainName = domainName
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist api mapping id after create: %w", err)
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionMapping(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "APIGatewayV2ApiMapping created")
}

func (r *APIGatewayV2ApiMappingReconciler) resolveDomainName(ctx context.Context, obj *awsv1alpha1.APIGatewayV2ApiMapping) (string, error) {
	ref := obj.Spec.DomainNameRef
	if ref.DomainName != "" {
		return ref.DomainName, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("domainNameRef requires name or domainName")
	}
	dn := &awsv1alpha1.APIGatewayV2DomainName{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: obj.Namespace}, dn); err != nil {
		return "", err
	}
	if dn.Status.DomainName == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("APIGatewayV2DomainName %s/%s has no domainName yet", obj.Namespace, ref.Name)}
	}
	return dn.Status.DomainName, nil
}

func (r *APIGatewayV2ApiMappingReconciler) deleteApiMapping(ctx context.Context, obj *awsv1alpha1.APIGatewayV2ApiMapping) error {
	if obj.Status.APIMappingID == "" || obj.Status.DomainName == "" {
		return nil
	}
	_, err := r.APIGatewayV2Client.DeleteApiMapping(ctx, &awsapigwv2.DeleteApiMappingInput{
		ApiMappingId: aws.String(obj.Status.APIMappingID),
		DomainName:   aws.String(obj.Status.DomainName),
	})
	if apigwv2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *APIGatewayV2ApiMappingReconciler) setConditionMapping(ctx context.Context, obj *awsv1alpha1.APIGatewayV2ApiMapping, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *APIGatewayV2ApiMappingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.APIGatewayV2ApiMapping{}).
		Complete(r)
}
