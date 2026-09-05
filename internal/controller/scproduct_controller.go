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
	awssc "github.com/aws/aws-sdk-go-v2/service/servicecatalog"
	sctypes "github.com/aws/aws-sdk-go-v2/service/servicecatalog/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	schelper "github.com/konfig-io/konfig-konector/internal/aws/servicecatalog"
)

// SCProductAWSAPI is the subset of the Service Catalog API used by this controller.
type SCProductAWSAPI interface {
	CreateProduct(ctx context.Context, params *awssc.CreateProductInput, optFns ...func(*awssc.Options)) (*awssc.CreateProductOutput, error)
	UpdateProduct(ctx context.Context, params *awssc.UpdateProductInput, optFns ...func(*awssc.Options)) (*awssc.UpdateProductOutput, error)
	DeleteProduct(ctx context.Context, params *awssc.DeleteProductInput, optFns ...func(*awssc.Options)) (*awssc.DeleteProductOutput, error)
	DescribeProductAsAdmin(ctx context.Context, params *awssc.DescribeProductAsAdminInput, optFns ...func(*awssc.Options)) (*awssc.DescribeProductAsAdminOutput, error)
}

// SCProductReconciler reconciles SCProduct objects.
type SCProductReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	ServiceCatalogClient SCProductAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=scproducts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=scproducts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=scproducts/finalizers,verbs=update

func (r *SCProductReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	prod := &awsv1alpha1.SCProduct{}
	if err := r.Get(ctx, req.NamespacedName, prod); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, prod); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !prod.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(prod, awsv1alpha1.FinalizerName) {
			if shouldAbandon(prod) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(prod, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, prod)
			}
			if err := r.deleteProduct(ctx, prod); err != nil {
				logger.Error(err, "failed to delete product")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(prod, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, prod)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(prod, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(prod, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, prod); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileProduct(ctx, prod); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, prod, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *SCProductReconciler) reconcileProduct(ctx context.Context, prod *awsv1alpha1.SCProduct) error {
	exists := false
	if prod.Status.ProductID != "" {
		out, err := r.ServiceCatalogClient.DescribeProductAsAdmin(ctx, &awssc.DescribeProductAsAdminInput{
			Id: aws.String(prod.Status.ProductID),
		})
		if err == nil {
			exists = true
			if out.ProductViewDetail != nil {
				prod.Status.ARN = aws.ToString(out.ProductViewDetail.ProductARN)
			}
		} else if !schelper.IsNotFound(err) {
			return fmt.Errorf("describe product: %w", err)
		}
	}

	if !exists {
		productType := prod.Spec.ProductType
		if productType == "" {
			productType = string(sctypes.ProductTypeCloudFormationTemplate)
		}
		input := &awssc.CreateProductInput{
			Name:             aws.String(prod.Spec.Name),
			Owner:            aws.String(prod.Spec.Owner),
			ProductType:      sctypes.ProductType(productType),
			IdempotencyToken: aws.String(string(prod.UID)),
			ProvisioningArtifactParameters: &sctypes.ProvisioningArtifactProperties{
				Name:        aws.String(prod.Spec.ProvisioningArtifact.Name),
				Description: aws.String(prod.Spec.ProvisioningArtifact.Description),
				Type:        sctypes.ProvisioningArtifactTypeCloudFormationTemplate,
				Info: map[string]string{
					"LoadTemplateFromURL": prod.Spec.ProvisioningArtifact.TemplateURL,
				},
			},
			Tags: scTags(prod.Spec.Tags),
		}
		if prod.Spec.Description != "" {
			input.Description = aws.String(prod.Spec.Description)
		}
		if prod.Spec.Distributor != "" {
			input.Distributor = aws.String(prod.Spec.Distributor)
		}
		if prod.Spec.SupportEmail != "" {
			input.SupportEmail = aws.String(prod.Spec.SupportEmail)
		}
		out, err := r.ServiceCatalogClient.CreateProduct(ctx, input)
		if err != nil {
			return fmt.Errorf("create product: %w", err)
		}
		// Persist the product ID immediately: the AWS resource now exists,
		// and losing the identifier would orphan it on delete.
		if out.ProductViewDetail != nil && out.ProductViewDetail.ProductViewSummary != nil {
			prod.Status.ProductID = aws.ToString(out.ProductViewDetail.ProductViewSummary.ProductId)
			prod.Status.ARN = aws.ToString(out.ProductViewDetail.ProductARN)
		}
		if out.ProvisioningArtifactDetail != nil {
			prod.Status.ProvisioningArtifactID = aws.ToString(out.ProvisioningArtifactDetail.Id)
		}
		if err := persistStatus(ctx, r.Client, prod); err != nil {
			return fmt.Errorf("persist product ID after create: %w", err)
		}
	} else if prod.Status.ObservedGeneration != prod.Generation {
		input := &awssc.UpdateProductInput{
			Id:          aws.String(prod.Status.ProductID),
			Name:        aws.String(prod.Spec.Name),
			Owner:       aws.String(prod.Spec.Owner),
			Description: aws.String(prod.Spec.Description),
			AddTags:     scTags(prod.Spec.Tags),
		}
		if prod.Spec.Distributor != "" {
			input.Distributor = aws.String(prod.Spec.Distributor)
		}
		if prod.Spec.SupportEmail != "" {
			input.SupportEmail = aws.String(prod.Spec.SupportEmail)
		}
		if _, err := r.ServiceCatalogClient.UpdateProduct(ctx, input); err != nil {
			return fmt.Errorf("update product: %w", err)
		}
	}

	prod.Status.ObservedGeneration = prod.Generation
	now := metav1.Now()
	prod.Status.LastSyncTime = &now
	return r.setCondition(ctx, prod, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "product reconciled")
}

func (r *SCProductReconciler) deleteProduct(ctx context.Context, prod *awsv1alpha1.SCProduct) error {
	if prod.Status.ProductID == "" {
		// Never created (or the identifier was lost). Product IDs cannot be
		// derived unambiguously from the spec, so do not guess.
		return nil
	}
	_, err := r.ServiceCatalogClient.DeleteProduct(ctx, &awssc.DeleteProductInput{
		Id: aws.String(prod.Status.ProductID),
	})
	if schelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SCProductReconciler) setCondition(ctx context.Context, prod *awsv1alpha1.SCProduct, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&prod.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: prod.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, prod); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *SCProductReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.SCProduct{}).
		Complete(r)
}
