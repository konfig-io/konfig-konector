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
	awsamp "github.com/aws/aws-sdk-go-v2/service/amp"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	amphelper "github.com/konfig-io/konfig-konector/internal/aws/amp"
)

// PrometheusAlertManagerDefinitionAWSAPI is the subset of the AMP API used by this controller.
type PrometheusAlertManagerDefinitionAWSAPI interface {
	DescribeAlertManagerDefinition(ctx context.Context, params *awsamp.DescribeAlertManagerDefinitionInput, optFns ...func(*awsamp.Options)) (*awsamp.DescribeAlertManagerDefinitionOutput, error)
	CreateAlertManagerDefinition(ctx context.Context, params *awsamp.CreateAlertManagerDefinitionInput, optFns ...func(*awsamp.Options)) (*awsamp.CreateAlertManagerDefinitionOutput, error)
	PutAlertManagerDefinition(ctx context.Context, params *awsamp.PutAlertManagerDefinitionInput, optFns ...func(*awsamp.Options)) (*awsamp.PutAlertManagerDefinitionOutput, error)
	DeleteAlertManagerDefinition(ctx context.Context, params *awsamp.DeleteAlertManagerDefinitionInput, optFns ...func(*awsamp.Options)) (*awsamp.DeleteAlertManagerDefinitionOutput, error)
}

// PrometheusAlertManagerDefinitionReconciler reconciles PrometheusAlertManagerDefinition objects.
type PrometheusAlertManagerDefinitionReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	AMPClient PrometheusAlertManagerDefinitionAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=prometheusalertmanagerdefinitions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=prometheusalertmanagerdefinitions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=prometheusalertmanagerdefinitions/finalizers,verbs=update

func (r *PrometheusAlertManagerDefinitionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	def := &awsv1alpha1.PrometheusAlertManagerDefinition{}
	if err := r.Get(ctx, req.NamespacedName, def); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, def); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !def.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(def, awsv1alpha1.FinalizerName) {
			if shouldAbandon(def) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(def, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, def)
			}
			if err := r.deleteDefinition(ctx, def); err != nil {
				logger.Error(err, "failed to delete Prometheus alert manager definition")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(def, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, def)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(def, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(def, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, def); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileDefinition(ctx, def); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, def, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *PrometheusAlertManagerDefinitionReconciler) reconcileDefinition(ctx context.Context, def *awsv1alpha1.PrometheusAlertManagerDefinition) error {
	workspaceID, err := resolvePrometheusWorkspaceID(ctx, r.Client, def.Namespace, def.Spec.WorkspaceRef)
	if err != nil {
		return err
	}

	_, err = r.AMPClient.DescribeAlertManagerDefinition(ctx, &awsamp.DescribeAlertManagerDefinitionInput{
		WorkspaceId: aws.String(workspaceID),
	})
	if amphelper.IsNotFound(err) {
		if _, err := r.AMPClient.CreateAlertManagerDefinition(ctx, &awsamp.CreateAlertManagerDefinitionInput{
			WorkspaceId: aws.String(workspaceID),
			Data:        []byte(def.Spec.Definition),
		}); err != nil {
			return fmt.Errorf("create Prometheus alert manager definition: %w", err)
		}
		def.Status.WorkspaceID = workspaceID
		// Persist the workspace ID immediately so the definition can be
		// deleted even if later steps fail.
		if err := persistStatus(ctx, r.Client, def); err != nil {
			return fmt.Errorf("persist workspace ID after create: %w", err)
		}
	} else if err != nil {
		return err
	} else {
		def.Status.WorkspaceID = workspaceID
		if def.Status.ObservedGeneration != def.Generation {
			if _, err := r.AMPClient.PutAlertManagerDefinition(ctx, &awsamp.PutAlertManagerDefinitionInput{
				WorkspaceId: aws.String(workspaceID),
				Data:        []byte(def.Spec.Definition),
			}); err != nil {
				return fmt.Errorf("put Prometheus alert manager definition: %w", err)
			}
		}
	}

	def.Status.ObservedGeneration = def.Generation
	now := metav1.Now()
	def.Status.LastSyncTime = &now
	return r.setCondition(ctx, def, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Prometheus alert manager definition reconciled")
}

func (r *PrometheusAlertManagerDefinitionReconciler) deleteDefinition(ctx context.Context, def *awsv1alpha1.PrometheusAlertManagerDefinition) error {
	workspaceID := def.Status.WorkspaceID
	if workspaceID == "" {
		if def.Spec.WorkspaceRef.WorkspaceID != "" {
			workspaceID = def.Spec.WorkspaceRef.WorkspaceID
		} else if def.Spec.WorkspaceRef.Name != "" {
			ws := &awsv1alpha1.PrometheusWorkspace{}
			err := r.Get(ctx, k8stypes.NamespacedName{Name: def.Spec.WorkspaceRef.Name, Namespace: def.Namespace}, ws)
			if err != nil {
				// Workspace CR gone: the definition was deleted with the workspace.
				return client.IgnoreNotFound(err)
			}
			workspaceID = ws.Status.WorkspaceID
		}
	}
	if workspaceID == "" {
		return nil
	}
	_, err := r.AMPClient.DeleteAlertManagerDefinition(ctx, &awsamp.DeleteAlertManagerDefinitionInput{
		WorkspaceId: aws.String(workspaceID),
	})
	if amphelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *PrometheusAlertManagerDefinitionReconciler) setCondition(ctx context.Context, def *awsv1alpha1.PrometheusAlertManagerDefinition, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&def.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: def.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, def); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *PrometheusAlertManagerDefinitionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.PrometheusAlertManagerDefinition{}).
		Complete(r)
}
