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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	schelper "github.com/konfig-io/konfig-konector/internal/aws/servicecatalog"
)

// SCPortfolioProductAssociationAWSAPI is the subset of the Service Catalog API
// used by this controller.
type SCPortfolioProductAssociationAWSAPI interface {
	AssociateProductWithPortfolio(ctx context.Context, params *awssc.AssociateProductWithPortfolioInput, optFns ...func(*awssc.Options)) (*awssc.AssociateProductWithPortfolioOutput, error)
	DisassociateProductFromPortfolio(ctx context.Context, params *awssc.DisassociateProductFromPortfolioInput, optFns ...func(*awssc.Options)) (*awssc.DisassociateProductFromPortfolioOutput, error)
}

// SCPortfolioProductAssociationReconciler reconciles SCPortfolioProductAssociation objects.
type SCPortfolioProductAssociationReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	ServiceCatalogClient SCPortfolioProductAssociationAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=scportfolioproductassociations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=scportfolioproductassociations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=scportfolioproductassociations/finalizers,verbs=update

func (r *SCPortfolioProductAssociationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	assoc := &awsv1alpha1.SCPortfolioProductAssociation{}
	if err := r.Get(ctx, req.NamespacedName, assoc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, assoc); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !assoc.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(assoc, awsv1alpha1.FinalizerName) {
			if shouldAbandon(assoc) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(assoc, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, assoc)
			}
			if assoc.Status.ProductID != "" && assoc.Status.PortfolioID != "" {
				if _, err := r.ServiceCatalogClient.DisassociateProductFromPortfolio(ctx, &awssc.DisassociateProductFromPortfolioInput{
					ProductId:   aws.String(assoc.Status.ProductID),
					PortfolioId: aws.String(assoc.Status.PortfolioID),
				}); err != nil && !schelper.IsNotFound(err) {
					logger.Error(err, "failed to disassociate product from portfolio")
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(assoc, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, assoc)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(assoc, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(assoc, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, assoc); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileAssociation(ctx, assoc); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, assoc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *SCPortfolioProductAssociationReconciler) resolveProductID(ctx context.Context, assoc *awsv1alpha1.SCPortfolioProductAssociation) (string, error) {
	if assoc.Spec.ProductRef.ProductID != "" {
		return assoc.Spec.ProductRef.ProductID, nil
	}
	if assoc.Spec.ProductRef.Name == "" {
		return "", fmt.Errorf("productRef must set either name or productId")
	}
	prod := &awsv1alpha1.SCProduct{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: assoc.Spec.ProductRef.Name, Namespace: assoc.Namespace}, prod); err != nil {
		return "", err
	}
	if prod.Status.ProductID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("SCProduct %s/%s has no product ID yet", assoc.Namespace, assoc.Spec.ProductRef.Name)}
	}
	return prod.Status.ProductID, nil
}

func (r *SCPortfolioProductAssociationReconciler) resolvePortfolioID(ctx context.Context, assoc *awsv1alpha1.SCPortfolioProductAssociation) (string, error) {
	if assoc.Spec.PortfolioRef.PortfolioID != "" {
		return assoc.Spec.PortfolioRef.PortfolioID, nil
	}
	if assoc.Spec.PortfolioRef.Name == "" {
		return "", fmt.Errorf("portfolioRef must set either name or portfolioId")
	}
	pf := &awsv1alpha1.SCPortfolio{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: assoc.Spec.PortfolioRef.Name, Namespace: assoc.Namespace}, pf); err != nil {
		return "", err
	}
	if pf.Status.PortfolioID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("SCPortfolio %s/%s has no portfolio ID yet", assoc.Namespace, assoc.Spec.PortfolioRef.Name)}
	}
	return pf.Status.PortfolioID, nil
}

func (r *SCPortfolioProductAssociationReconciler) reconcileAssociation(ctx context.Context, assoc *awsv1alpha1.SCPortfolioProductAssociation) error {
	productID, err := r.resolveProductID(ctx, assoc)
	if err != nil {
		return err
	}
	portfolioID, err := r.resolvePortfolioID(ctx, assoc)
	if err != nil {
		return err
	}

	// AssociateProductWithPortfolio is idempotent: re-associating an already
	// associated pair succeeds.
	if _, err := r.ServiceCatalogClient.AssociateProductWithPortfolio(ctx, &awssc.AssociateProductWithPortfolioInput{
		ProductId:   aws.String(productID),
		PortfolioId: aws.String(portfolioID),
	}); err != nil {
		return fmt.Errorf("associate product with portfolio: %w", err)
	}

	assoc.Status.ProductID = productID
	assoc.Status.PortfolioID = portfolioID
	if err := persistStatus(ctx, r.Client, assoc); err != nil {
		return fmt.Errorf("persist association identifiers: %w", err)
	}

	assoc.Status.ObservedGeneration = assoc.Generation
	now := metav1.Now()
	assoc.Status.LastSyncTime = &now
	return r.setCondition(ctx, assoc, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "portfolio-product association reconciled")
}

func (r *SCPortfolioProductAssociationReconciler) setCondition(ctx context.Context, assoc *awsv1alpha1.SCPortfolioProductAssociation, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&assoc.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: assoc.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, assoc); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *SCPortfolioProductAssociationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.SCPortfolioProductAssociation{}).
		Complete(r)
}
