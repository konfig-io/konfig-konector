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
	awsathena "github.com/aws/aws-sdk-go-v2/service/athena"
	athenatypes "github.com/aws/aws-sdk-go-v2/service/athena/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	athenahelper "github.com/konfig-io/konfig-konector/internal/aws/athena"
)

// AthenaDataCatalogAWSAPI is the subset of the Athena SDK client used by this
// controller. *awsathena.Client satisfies it.
type AthenaDataCatalogAWSAPI interface {
	GetDataCatalog(ctx context.Context, params *awsathena.GetDataCatalogInput, optFns ...func(*awsathena.Options)) (*awsathena.GetDataCatalogOutput, error)
	CreateDataCatalog(ctx context.Context, params *awsathena.CreateDataCatalogInput, optFns ...func(*awsathena.Options)) (*awsathena.CreateDataCatalogOutput, error)
	UpdateDataCatalog(ctx context.Context, params *awsathena.UpdateDataCatalogInput, optFns ...func(*awsathena.Options)) (*awsathena.UpdateDataCatalogOutput, error)
	DeleteDataCatalog(ctx context.Context, params *awsathena.DeleteDataCatalogInput, optFns ...func(*awsathena.Options)) (*awsathena.DeleteDataCatalogOutput, error)
}

// AthenaDataCatalogReconciler reconciles AthenaDataCatalog objects.
type AthenaDataCatalogReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	AthenaClient AthenaDataCatalogAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=athenadatacatalogs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=athenadatacatalogs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=athenadatacatalogs/finalizers,verbs=update

func (r *AthenaDataCatalogReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	dc := &awsv1alpha1.AthenaDataCatalog{}
	if err := r.Get(ctx, req.NamespacedName, dc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, dc); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !dc.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(dc, awsv1alpha1.FinalizerName) {
			if shouldAbandon(dc) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(dc, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, dc)
			}
			if err := r.deleteDataCatalog(ctx, dc); err != nil {
				logger.Error(err, "failed to delete Athena data catalog")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(dc, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, dc)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(dc, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(dc, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, dc); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileDataCatalog(ctx, dc); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, dc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *AthenaDataCatalogReconciler) reconcileDataCatalog(ctx context.Context, dc *awsv1alpha1.AthenaDataCatalog) error {
	_, err := r.AthenaClient.GetDataCatalog(ctx, &awsathena.GetDataCatalogInput{
		Name: aws.String(dc.Spec.Name),
	})
	if athenahelper.IsNotFound(err) {
		input := &awsathena.CreateDataCatalogInput{
			Name: aws.String(dc.Spec.Name),
			Type: athenatypes.DataCatalogType(dc.Spec.Type),
			Tags: athenaTags(dc.Spec.Tags),
		}
		if len(dc.Spec.Parameters) > 0 {
			input.Parameters = dc.Spec.Parameters
		}
		if dc.Spec.Description != "" {
			input.Description = aws.String(dc.Spec.Description)
		}
		if _, err := r.AthenaClient.CreateDataCatalog(ctx, input); err != nil {
			return fmt.Errorf("create Athena data catalog: %w", err)
		}
		// Persist the identifier immediately: the AWS resource now exists.
		dc.Status.CatalogName = dc.Spec.Name
		if err := persistStatus(ctx, r.Client, dc); err != nil {
			return fmt.Errorf("persist catalog name after create: %w", err)
		}
	} else if err != nil {
		return err
	} else if dc.Status.ObservedGeneration != dc.Generation {
		input := &awsathena.UpdateDataCatalogInput{
			Name: aws.String(dc.Spec.Name),
			Type: athenatypes.DataCatalogType(dc.Spec.Type),
		}
		if len(dc.Spec.Parameters) > 0 {
			input.Parameters = dc.Spec.Parameters
		}
		if dc.Spec.Description != "" {
			input.Description = aws.String(dc.Spec.Description)
		}
		if _, err := r.AthenaClient.UpdateDataCatalog(ctx, input); err != nil {
			return fmt.Errorf("update Athena data catalog: %w", err)
		}
	}

	dc.Status.CatalogName = dc.Spec.Name
	dc.Status.ObservedGeneration = dc.Generation
	now := metav1.Now()
	dc.Status.LastSyncTime = &now
	return r.setCondition(ctx, dc, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Athena data catalog reconciled")
}

func (r *AthenaDataCatalogReconciler) deleteDataCatalog(ctx context.Context, dc *awsv1alpha1.AthenaDataCatalog) error {
	name := dc.Status.CatalogName
	if name == "" {
		name = dc.Spec.Name
	}
	_, err := r.AthenaClient.DeleteDataCatalog(ctx, &awsathena.DeleteDataCatalogInput{
		Name: aws.String(name),
	})
	if athenahelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *AthenaDataCatalogReconciler) setCondition(ctx context.Context, dc *awsv1alpha1.AthenaDataCatalog, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&dc.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: dc.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, dc); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *AthenaDataCatalogReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.AthenaDataCatalog{}).
		Complete(r)
}
