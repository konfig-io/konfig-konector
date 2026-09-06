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
	awsgrafana "github.com/aws/aws-sdk-go-v2/service/grafana"
	grafanatypes "github.com/aws/aws-sdk-go-v2/service/grafana/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	grafanahelper "github.com/konfig-io/konfig-konector/internal/aws/grafana"
)

// GrafanaWorkspaceAWSAPI is the subset of the Managed Grafana API used by this controller.
type GrafanaWorkspaceAWSAPI interface {
	DescribeWorkspace(ctx context.Context, params *awsgrafana.DescribeWorkspaceInput, optFns ...func(*awsgrafana.Options)) (*awsgrafana.DescribeWorkspaceOutput, error)
	CreateWorkspace(ctx context.Context, params *awsgrafana.CreateWorkspaceInput, optFns ...func(*awsgrafana.Options)) (*awsgrafana.CreateWorkspaceOutput, error)
	UpdateWorkspace(ctx context.Context, params *awsgrafana.UpdateWorkspaceInput, optFns ...func(*awsgrafana.Options)) (*awsgrafana.UpdateWorkspaceOutput, error)
	DeleteWorkspace(ctx context.Context, params *awsgrafana.DeleteWorkspaceInput, optFns ...func(*awsgrafana.Options)) (*awsgrafana.DeleteWorkspaceOutput, error)
}

// GrafanaWorkspaceReconciler reconciles GrafanaWorkspace objects.
type GrafanaWorkspaceReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	GrafanaClient GrafanaWorkspaceAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=grafanaworkspaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=grafanaworkspaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=grafanaworkspaces/finalizers,verbs=update

func (r *GrafanaWorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ws := &awsv1alpha1.GrafanaWorkspace{}
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
				logger.Error(err, "failed to delete Grafana workspace")
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

func (r *GrafanaWorkspaceReconciler) authProviders(ws *awsv1alpha1.GrafanaWorkspace) []grafanatypes.AuthenticationProviderTypes {
	providers := make([]grafanatypes.AuthenticationProviderTypes, 0, len(ws.Spec.AuthenticationProviders))
	for _, p := range ws.Spec.AuthenticationProviders {
		providers = append(providers, grafanatypes.AuthenticationProviderTypes(p))
	}
	return providers
}

func (r *GrafanaWorkspaceReconciler) dataSources(ws *awsv1alpha1.GrafanaWorkspace) []grafanatypes.DataSourceType {
	sources := make([]grafanatypes.DataSourceType, 0, len(ws.Spec.DataSources))
	for _, s := range ws.Spec.DataSources {
		sources = append(sources, grafanatypes.DataSourceType(s))
	}
	return sources
}

func (r *GrafanaWorkspaceReconciler) resolveWorkspaceRoleARN(ctx context.Context, ws *awsv1alpha1.GrafanaWorkspace) (string, error) {
	if ws.Spec.WorkspaceRoleRef == nil {
		return "", nil
	}
	return resolveIAMRoleARN(ctx, r.Client, ws.Namespace, *ws.Spec.WorkspaceRoleRef)
}

func (r *GrafanaWorkspaceReconciler) reconcileWorkspace(ctx context.Context, ws *awsv1alpha1.GrafanaWorkspace) (ctrl.Result, error) {
	roleARN, err := r.resolveWorkspaceRoleARN(ctx, ws)
	if err != nil {
		return ctrl.Result{}, err
	}

	if ws.Status.WorkspaceID == "" {
		in := &awsgrafana.CreateWorkspaceInput{
			AccountAccessType:       grafanatypes.AccountAccessType(ws.Spec.AccountAccessType),
			AuthenticationProviders: r.authProviders(ws),
			PermissionType:          grafanatypes.PermissionType(ws.Spec.PermissionType),
			WorkspaceName:           aws.String(ws.Spec.WorkspaceName),
			WorkspaceDataSources:    r.dataSources(ws),
			Tags:                    ws.Spec.Tags,
		}
		if roleARN != "" {
			in.WorkspaceRoleArn = aws.String(roleARN)
		}
		if ws.Spec.GrafanaVersion != "" {
			in.GrafanaVersion = aws.String(ws.Spec.GrafanaVersion)
		}
		created, err := r.GrafanaClient.CreateWorkspace(ctx, in)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("create Grafana workspace: %w", err)
		}
		if created.Workspace != nil {
			ws.Status.WorkspaceID = aws.ToString(created.Workspace.Id)
			ws.Status.Status = string(created.Workspace.Status)
		}
		// Persist the workspace ID immediately: the AWS resource now exists,
		// and losing the identifier would orphan it on delete.
		if err := persistStatus(ctx, r.Client, ws); err != nil {
			return ctrl.Result{}, fmt.Errorf("persist workspace ID after create: %w", err)
		}
		_ = r.setCondition(ctx, ws, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "Grafana workspace is CREATING")
		return requeueDevOpsCostPolling, nil
	}

	got, err := r.GrafanaClient.DescribeWorkspace(ctx, &awsgrafana.DescribeWorkspaceInput{
		WorkspaceId: aws.String(ws.Status.WorkspaceID),
	})
	if grafanahelper.IsNotFound(err) {
		// Recreate on next pass by clearing the stale identifier.
		ws.Status.WorkspaceID = ""
		ws.Status.Status = ""
		ws.Status.Endpoint = ""
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
		return ctrl.Result{}, fmt.Errorf("describe Grafana workspace returned no workspace")
	}
	ws.Status.Status = string(desc.Status)
	ws.Status.Endpoint = aws.ToString(desc.Endpoint)
	ws.Status.GrafanaVersion = aws.ToString(desc.GrafanaVersion)

	if desc.Status != grafanatypes.WorkspaceStatusActive {
		_ = r.setCondition(ctx, ws, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning",
			fmt.Sprintf("Grafana workspace is %s", ws.Status.Status))
		return requeueDevOpsCostPolling, nil
	}

	if ws.Status.ObservedGeneration != ws.Generation {
		in := &awsgrafana.UpdateWorkspaceInput{
			WorkspaceId:          aws.String(ws.Status.WorkspaceID),
			AccountAccessType:    grafanatypes.AccountAccessType(ws.Spec.AccountAccessType),
			WorkspaceName:        aws.String(ws.Spec.WorkspaceName),
			WorkspaceDataSources: r.dataSources(ws),
		}
		// Only CUSTOMER_MANAGED may be (re)stated on update; passing
		// SERVICE_MANAGED for an already service-managed workspace is invalid.
		if ws.Spec.PermissionType == string(grafanatypes.PermissionTypeCustomerManaged) {
			in.PermissionType = grafanatypes.PermissionTypeCustomerManaged
		}
		if roleARN != "" {
			in.WorkspaceRoleArn = aws.String(roleARN)
		}
		if _, err := r.GrafanaClient.UpdateWorkspace(ctx, in); err != nil {
			return ctrl.Result{}, fmt.Errorf("update Grafana workspace: %w", err)
		}
	}

	ws.Status.ObservedGeneration = ws.Generation
	now := metav1.Now()
	ws.Status.LastSyncTime = &now
	if err := r.setCondition(ctx, ws, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Grafana workspace active"); err != nil {
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *GrafanaWorkspaceReconciler) deleteWorkspace(ctx context.Context, ws *awsv1alpha1.GrafanaWorkspace) error {
	if ws.Status.WorkspaceID == "" {
		// Workspace IDs are AWS-generated; without one there is no unambiguous
		// resource to delete (workspace names are not unique).
		return nil
	}
	_, err := r.GrafanaClient.DeleteWorkspace(ctx, &awsgrafana.DeleteWorkspaceInput{
		WorkspaceId: aws.String(ws.Status.WorkspaceID),
	})
	if grafanahelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *GrafanaWorkspaceReconciler) setCondition(ctx context.Context, ws *awsv1alpha1.GrafanaWorkspace, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *GrafanaWorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.GrafanaWorkspace{}).
		Complete(r)
}
