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

// scTags converts a CR tag map to Service Catalog SDK tags.
func scTags(tags map[string]string) []sctypes.Tag {
	if len(tags) == 0 {
		return nil
	}
	out := make([]sctypes.Tag, 0, len(tags))
	for k, v := range tags {
		out = append(out, sctypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	return out
}

// SCPortfolioAWSAPI is the subset of the Service Catalog API used by this controller.
type SCPortfolioAWSAPI interface {
	CreatePortfolio(ctx context.Context, params *awssc.CreatePortfolioInput, optFns ...func(*awssc.Options)) (*awssc.CreatePortfolioOutput, error)
	UpdatePortfolio(ctx context.Context, params *awssc.UpdatePortfolioInput, optFns ...func(*awssc.Options)) (*awssc.UpdatePortfolioOutput, error)
	DeletePortfolio(ctx context.Context, params *awssc.DeletePortfolioInput, optFns ...func(*awssc.Options)) (*awssc.DeletePortfolioOutput, error)
	DescribePortfolio(ctx context.Context, params *awssc.DescribePortfolioInput, optFns ...func(*awssc.Options)) (*awssc.DescribePortfolioOutput, error)
}

// SCPortfolioReconciler reconciles SCPortfolio objects.
type SCPortfolioReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	ServiceCatalogClient SCPortfolioAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=scportfolios,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=scportfolios/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=scportfolios/finalizers,verbs=update

func (r *SCPortfolioReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	pf := &awsv1alpha1.SCPortfolio{}
	if err := r.Get(ctx, req.NamespacedName, pf); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, pf); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !pf.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(pf, awsv1alpha1.FinalizerName) {
			if shouldAbandon(pf) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(pf, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, pf)
			}
			if err := r.deletePortfolio(ctx, pf); err != nil {
				logger.Error(err, "failed to delete portfolio")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(pf, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, pf)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(pf, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(pf, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, pf); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcilePortfolio(ctx, pf); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, pf, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *SCPortfolioReconciler) reconcilePortfolio(ctx context.Context, pf *awsv1alpha1.SCPortfolio) error {
	exists := false
	if pf.Status.PortfolioID != "" {
		out, err := r.ServiceCatalogClient.DescribePortfolio(ctx, &awssc.DescribePortfolioInput{
			Id: aws.String(pf.Status.PortfolioID),
		})
		if err == nil {
			exists = true
			pf.Status.ARN = aws.ToString(out.PortfolioDetail.ARN)
		} else if !schelper.IsNotFound(err) {
			return fmt.Errorf("describe portfolio: %w", err)
		}
	}

	if !exists {
		input := &awssc.CreatePortfolioInput{
			DisplayName:      aws.String(pf.Spec.DisplayName),
			ProviderName:     aws.String(pf.Spec.ProviderName),
			IdempotencyToken: aws.String(string(pf.UID)),
			Tags:             scTags(pf.Spec.Tags),
		}
		if pf.Spec.Description != "" {
			input.Description = aws.String(pf.Spec.Description)
		}
		out, err := r.ServiceCatalogClient.CreatePortfolio(ctx, input)
		if err != nil {
			return fmt.Errorf("create portfolio: %w", err)
		}
		// Persist the portfolio ID immediately: the AWS resource now exists,
		// and losing the identifier would orphan it on delete.
		pf.Status.PortfolioID = aws.ToString(out.PortfolioDetail.Id)
		pf.Status.ARN = aws.ToString(out.PortfolioDetail.ARN)
		if err := persistStatus(ctx, r.Client, pf); err != nil {
			return fmt.Errorf("persist portfolio ID after create: %w", err)
		}
	} else if pf.Status.ObservedGeneration != pf.Generation {
		if _, err := r.ServiceCatalogClient.UpdatePortfolio(ctx, &awssc.UpdatePortfolioInput{
			Id:           aws.String(pf.Status.PortfolioID),
			DisplayName:  aws.String(pf.Spec.DisplayName),
			ProviderName: aws.String(pf.Spec.ProviderName),
			Description:  aws.String(pf.Spec.Description),
			AddTags:      scTags(pf.Spec.Tags),
		}); err != nil {
			return fmt.Errorf("update portfolio: %w", err)
		}
	}

	pf.Status.ObservedGeneration = pf.Generation
	now := metav1.Now()
	pf.Status.LastSyncTime = &now
	return r.setCondition(ctx, pf, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "portfolio reconciled")
}

func (r *SCPortfolioReconciler) deletePortfolio(ctx context.Context, pf *awsv1alpha1.SCPortfolio) error {
	if pf.Status.PortfolioID == "" {
		// Never created (or the identifier was lost). Portfolio IDs cannot be
		// derived unambiguously from the spec, so do not guess.
		return nil
	}
	_, err := r.ServiceCatalogClient.DeletePortfolio(ctx, &awssc.DeletePortfolioInput{
		Id: aws.String(pf.Status.PortfolioID),
	})
	if schelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SCPortfolioReconciler) setCondition(ctx context.Context, pf *awsv1alpha1.SCPortfolio, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&pf.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: pf.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, pf); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *SCPortfolioReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.SCPortfolio{}).
		Complete(r)
}
