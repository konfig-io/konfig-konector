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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsamp "github.com/aws/aws-sdk-go-v2/service/amp"
	amptypes "github.com/aws/aws-sdk-go-v2/service/amp/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	amphelper "github.com/konfig-io/konfig-konector/internal/aws/amp"
)

// requeueDevOpsCostPolling is the poll interval for async devopscost-family
// resources (AMP and Grafana workspaces).
var requeueDevOpsCostPolling = ctrl.Result{RequeueAfter: 15 * time.Second}

// PrometheusWorkspaceAWSAPI is the subset of the AMP API used by this controller.
type PrometheusWorkspaceAWSAPI interface {
	DescribeWorkspace(ctx context.Context, params *awsamp.DescribeWorkspaceInput, optFns ...func(*awsamp.Options)) (*awsamp.DescribeWorkspaceOutput, error)
	CreateWorkspace(ctx context.Context, params *awsamp.CreateWorkspaceInput, optFns ...func(*awsamp.Options)) (*awsamp.CreateWorkspaceOutput, error)
	UpdateWorkspaceAlias(ctx context.Context, params *awsamp.UpdateWorkspaceAliasInput, optFns ...func(*awsamp.Options)) (*awsamp.UpdateWorkspaceAliasOutput, error)
	DeleteWorkspace(ctx context.Context, params *awsamp.DeleteWorkspaceInput, optFns ...func(*awsamp.Options)) (*awsamp.DeleteWorkspaceOutput, error)
}

// PrometheusWorkspaceReconciler reconciles PrometheusWorkspace objects.
type PrometheusWorkspaceReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	AMPClient PrometheusWorkspaceAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=prometheusworkspaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=prometheusworkspaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=prometheusworkspaces/finalizers,verbs=update

func (r *PrometheusWorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ws := &awsv1alpha1.PrometheusWorkspace{}
	if err := r.Get(ctx, req.NamespacedName, ws); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, ws); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !ws.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ws, awsv1alpha1.FinalizerName) {
			if shouldAbandon(ws) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(ws, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, ws)
			}
			if err := r.deleteWorkspace(ctx, ws); err != nil {
				logger.Error(err, "failed to delete Prometheus workspace")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(ws, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, ws)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(ws, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(ws, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, ws); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	result, err := r.reconcileWorkspace(ctx, ws)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, ws, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *PrometheusWorkspaceReconciler) reconcileWorkspace(ctx context.Context, ws *awsv1alpha1.PrometheusWorkspace) (ctrl.Result, error) {
	if ws.Status.WorkspaceID == "" {
		in := &awsamp.CreateWorkspaceInput{
			Tags: ws.Spec.Tags,
		}
		if ws.Spec.Alias != "" {
			in.Alias = aws.String(ws.Spec.Alias)
		}
		if ws.Spec.KMSKeyARN != "" {
			in.KmsKeyArn = aws.String(ws.Spec.KMSKeyARN)
		}
		created, err := r.AMPClient.CreateWorkspace(ctx, in)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("create Prometheus workspace: %w", err)
		}
		ws.Status.WorkspaceID = aws.ToString(created.WorkspaceId)
		ws.Status.ARN = aws.ToString(created.Arn)
		if created.Status != nil {
			ws.Status.Status = string(created.Status.StatusCode)
		}
		// Persist the workspace ID immediately: the AWS resource now exists,
		// and losing the identifier would orphan it on delete.
		if err := persistStatus(ctx, r.Client, ws); err != nil {
			return ctrl.Result{}, fmt.Errorf("persist workspace ID after create: %w", err)
		}
		_ = r.setCondition(ctx, ws, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "Prometheus workspace is CREATING")
		return requeueDevOpsCostPolling, nil
	}

	got, err := r.AMPClient.DescribeWorkspace(ctx, &awsamp.DescribeWorkspaceInput{
		WorkspaceId: aws.String(ws.Status.WorkspaceID),
	})
	if amphelper.IsNotFound(err) {
		// Recreate on next pass by clearing the stale identifier.
		ws.Status.WorkspaceID = ""
		ws.Status.ARN = ""
		ws.Status.Status = ""
		if perr := persistStatus(ctx, r.Client, ws); perr != nil {
			return ctrl.Result{}, perr
		}
		return requeueDevOpsCostPolling, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	desc := got.Workspace
	if desc == nil {
		return ctrl.Result{}, fmt.Errorf("describe Prometheus workspace returned no workspace")
	}
	ws.Status.ARN = aws.ToString(desc.Arn)
	ws.Status.PrometheusEndpoint = aws.ToString(desc.PrometheusEndpoint)
	if desc.Status != nil {
		ws.Status.Status = string(desc.Status.StatusCode)
	}

	if desc.Status != nil && desc.Status.StatusCode != amptypes.WorkspaceStatusCodeActive {
		_ = r.setCondition(ctx, ws, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning",
			fmt.Sprintf("Prometheus workspace is %s", ws.Status.Status))
		return requeueDevOpsCostPolling, nil
	}

	if ws.Status.ObservedGeneration != ws.Generation {
		if ws.Spec.Alias != aws.ToString(desc.Alias) {
			if _, err := r.AMPClient.UpdateWorkspaceAlias(ctx, &awsamp.UpdateWorkspaceAliasInput{
				WorkspaceId: aws.String(ws.Status.WorkspaceID),
				Alias:       aws.String(ws.Spec.Alias),
			}); err != nil {
				return ctrl.Result{}, fmt.Errorf("update Prometheus workspace alias: %w", err)
			}
		}
		// KMS key cannot change after creation.
		if ws.Spec.KMSKeyARN != "" && desc.KmsKeyArn != nil && ws.Spec.KMSKeyARN != aws.ToString(desc.KmsKeyArn) {
			return ctrl.Result{}, r.setCondition(ctx, ws, awsv1alpha1.ConditionReady, metav1.ConditionFalse,
				awsv1alpha1.ReasonUpdateNotSupported, "kmsKeyArn cannot be changed after workspace creation")
		}
	}

	ws.Status.ObservedGeneration = ws.Generation
	now := metav1.Now()
	ws.Status.LastSyncTime = &now
	if err := r.setCondition(ctx, ws, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Prometheus workspace active"); err != nil {
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *PrometheusWorkspaceReconciler) deleteWorkspace(ctx context.Context, ws *awsv1alpha1.PrometheusWorkspace) error {
	if ws.Status.WorkspaceID == "" {
		// Workspace IDs are AWS-generated; without one there is no unambiguous
		// resource to delete (aliases are not unique).
		return nil
	}
	_, err := r.AMPClient.DeleteWorkspace(ctx, &awsamp.DeleteWorkspaceInput{
		WorkspaceId: aws.String(ws.Status.WorkspaceID),
	})
	if amphelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *PrometheusWorkspaceReconciler) setCondition(ctx context.Context, ws *awsv1alpha1.PrometheusWorkspace, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&ws.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: ws.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, ws); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *PrometheusWorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.PrometheusWorkspace{}).
		Complete(r)
}
