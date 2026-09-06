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
	apigwv2types "github.com/aws/aws-sdk-go-v2/service/apigatewayv2/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	apigwv2helper "github.com/konfig-io/konfig-konector/internal/aws/apigatewayv2"
)

// APIGatewayV2DomainNameAWSAPI is the subset of the API Gateway v2 API used by this controller.
type APIGatewayV2DomainNameAWSAPI interface {
	GetDomainName(ctx context.Context, params *awsapigwv2.GetDomainNameInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.GetDomainNameOutput, error)
	CreateDomainName(ctx context.Context, params *awsapigwv2.CreateDomainNameInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.CreateDomainNameOutput, error)
	UpdateDomainName(ctx context.Context, params *awsapigwv2.UpdateDomainNameInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.UpdateDomainNameOutput, error)
	DeleteDomainName(ctx context.Context, params *awsapigwv2.DeleteDomainNameInput, optFns ...func(*awsapigwv2.Options)) (*awsapigwv2.DeleteDomainNameOutput, error)
}

// APIGatewayV2DomainNameReconciler reconciles APIGatewayV2DomainName objects.
type APIGatewayV2DomainNameReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	APIGatewayV2Client APIGatewayV2DomainNameAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2domainnames,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2domainnames/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=apigatewayv2domainnames/finalizers,verbs=update

func (r *APIGatewayV2DomainNameReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.APIGatewayV2DomainName{}
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
			if err := r.deleteDomainName(ctx, obj); err != nil {
				logger.Error(err, "failed to delete APIGatewayV2DomainName")
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

	if err := r.reconcileDomainName(ctx, obj); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionDomain(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *APIGatewayV2DomainNameReconciler) domainNameConfigurations(obj *awsv1alpha1.APIGatewayV2DomainName) []apigwv2types.DomainNameConfiguration {
	cfg := apigwv2types.DomainNameConfiguration{
		CertificateArn: aws.String(obj.Spec.CertificateARN),
	}
	if obj.Spec.EndpointType != "" {
		cfg.EndpointType = apigwv2types.EndpointType(obj.Spec.EndpointType)
	}
	if obj.Spec.SecurityPolicy != "" {
		cfg.SecurityPolicy = apigwv2types.SecurityPolicy(obj.Spec.SecurityPolicy)
	}
	return []apigwv2types.DomainNameConfiguration{cfg}
}

func (r *APIGatewayV2DomainNameReconciler) reconcileDomainName(ctx context.Context, obj *awsv1alpha1.APIGatewayV2DomainName) error {
	getOut, getErr := r.APIGatewayV2Client.GetDomainName(ctx, &awsapigwv2.GetDomainNameInput{
		DomainName: aws.String(obj.Spec.DomainName),
	})
	if getErr != nil && !apigwv2helper.IsNotFound(getErr) {
		return fmt.Errorf("get domain name: %w", getErr)
	}

	var configs []apigwv2types.DomainNameConfiguration
	if getErr == nil {
		if obj.Status.ObservedGeneration != obj.Generation {
			if _, err := r.APIGatewayV2Client.UpdateDomainName(ctx, &awsapigwv2.UpdateDomainNameInput{
				DomainName:               aws.String(obj.Spec.DomainName),
				DomainNameConfigurations: r.domainNameConfigurations(obj),
			}); err != nil {
				return fmt.Errorf("update domain name: %w", err)
			}
		}
		configs = getOut.DomainNameConfigurations
	} else {
		input := &awsapigwv2.CreateDomainNameInput{
			DomainName:               aws.String(obj.Spec.DomainName),
			DomainNameConfigurations: r.domainNameConfigurations(obj),
		}
		if len(obj.Spec.Tags) > 0 {
			input.Tags = obj.Spec.Tags
		}
		out, err := r.APIGatewayV2Client.CreateDomainName(ctx, input)
		if err != nil {
			return fmt.Errorf("create domain name: %w", err)
		}
		// Persist the domain name immediately: the AWS resource now exists,
		// and losing the identifier would orphan it on delete.
		obj.Status.DomainName = aws.ToString(out.DomainName)
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return fmt.Errorf("persist domain name after create: %w", err)
		}
		configs = out.DomainNameConfigurations
	}

	obj.Status.DomainName = obj.Spec.DomainName
	if len(configs) > 0 {
		obj.Status.APIGatewayDomainName = aws.ToString(configs[0].ApiGatewayDomainName)
		obj.Status.HostedZoneID = aws.ToString(configs[0].HostedZoneId)
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionDomain(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "APIGatewayV2DomainName reconciled")
}

func (r *APIGatewayV2DomainNameReconciler) deleteDomainName(ctx context.Context, obj *awsv1alpha1.APIGatewayV2DomainName) error {
	domainName := obj.Status.DomainName
	if domainName == "" {
		// The domain name is deterministically derivable from the spec.
		domainName = obj.Spec.DomainName
	}
	_, err := r.APIGatewayV2Client.DeleteDomainName(ctx, &awsapigwv2.DeleteDomainNameInput{
		DomainName: aws.String(domainName),
	})
	if apigwv2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *APIGatewayV2DomainNameReconciler) setConditionDomain(ctx context.Context, obj *awsv1alpha1.APIGatewayV2DomainName, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *APIGatewayV2DomainNameReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.APIGatewayV2DomainName{}).
		Complete(r)
}
