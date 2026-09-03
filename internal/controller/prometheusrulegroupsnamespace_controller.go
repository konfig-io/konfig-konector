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

// resolvePrometheusWorkspaceID resolves a PrometheusWorkspaceRef to an AWS
// workspace ID.
func resolvePrometheusWorkspaceID(ctx context.Context, c client.Client, namespace string, ref awsv1alpha1.PrometheusWorkspaceRef) (string, error) {
	if ref.WorkspaceID != "" {
		return ref.WorkspaceID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("workspaceRef requires either name or workspaceId")
	}
	ws := &awsv1alpha1.PrometheusWorkspace{}
	if err := c.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, ws); err != nil {
		return "", err
	}
	if ws.Status.WorkspaceID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("PrometheusWorkspace %s/%s has no workspace ID yet", namespace, ref.Name)}
	}
	if ws.Status.Status != "ACTIVE" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("PrometheusWorkspace %s/%s is not yet ACTIVE (status: %s)", namespace, ref.Name, ws.Status.Status)}
	}
	return ws.Status.WorkspaceID, nil
}

// PrometheusRuleGroupsNamespaceAWSAPI is the subset of the AMP API used by this controller.
type PrometheusRuleGroupsNamespaceAWSAPI interface {
	DescribeRuleGroupsNamespace(ctx context.Context, params *awsamp.DescribeRuleGroupsNamespaceInput, optFns ...func(*awsamp.Options)) (*awsamp.DescribeRuleGroupsNamespaceOutput, error)
	CreateRuleGroupsNamespace(ctx context.Context, params *awsamp.CreateRuleGroupsNamespaceInput, optFns ...func(*awsamp.Options)) (*awsamp.CreateRuleGroupsNamespaceOutput, error)
	PutRuleGroupsNamespace(ctx context.Context, params *awsamp.PutRuleGroupsNamespaceInput, optFns ...func(*awsamp.Options)) (*awsamp.PutRuleGroupsNamespaceOutput, error)
	DeleteRuleGroupsNamespace(ctx context.Context, params *awsamp.DeleteRuleGroupsNamespaceInput, optFns ...func(*awsamp.Options)) (*awsamp.DeleteRuleGroupsNamespaceOutput, error)
}

// PrometheusRuleGroupsNamespaceReconciler reconciles PrometheusRuleGroupsNamespace objects.
type PrometheusRuleGroupsNamespaceReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	AMPClient PrometheusRuleGroupsNamespaceAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=prometheusrulegroupsnamespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=prometheusrulegroupsnamespaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=prometheusrulegroupsnamespaces/finalizers,verbs=update

func (r *PrometheusRuleGroupsNamespaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ns := &awsv1alpha1.PrometheusRuleGroupsNamespace{}
	if err := r.Get(ctx, req.NamespacedName, ns); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !ns.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ns, awsv1alpha1.FinalizerName) {
			if shouldAbandon(ns) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(ns, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, ns)
			}
			if err := r.deleteNamespace(ctx, ns); err != nil {
				logger.Error(err, "failed to delete Prometheus rule groups namespace")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(ns, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, ns)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(ns, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(ns, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, ns); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileNamespace(ctx, ns); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, ns, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *PrometheusRuleGroupsNamespaceReconciler) reconcileNamespace(ctx context.Context, ns *awsv1alpha1.PrometheusRuleGroupsNamespace) error {
	workspaceID, err := resolvePrometheusWorkspaceID(ctx, r.Client, ns.Namespace, ns.Spec.WorkspaceRef)
	if err != nil {
		return err
	}

	_, err = r.AMPClient.DescribeRuleGroupsNamespace(ctx, &awsamp.DescribeRuleGroupsNamespaceInput{
		WorkspaceId: aws.String(workspaceID),
		Name:        aws.String(ns.Spec.Name),
	})
	if amphelper.IsNotFound(err) {
		created, err := r.AMPClient.CreateRuleGroupsNamespace(ctx, &awsamp.CreateRuleGroupsNamespaceInput{
			WorkspaceId: aws.String(workspaceID),
			Name:        aws.String(ns.Spec.Name),
			Data:        []byte(ns.Spec.Data),
			Tags:        ns.Spec.Tags,
		})
		if err != nil {
			return fmt.Errorf("create Prometheus rule groups namespace: %w", err)
		}
		ns.Status.ARN = aws.ToString(created.Arn)
		ns.Status.WorkspaceID = workspaceID
		// Persist the identifier immediately: the AWS resource now exists, and
		// losing it would orphan the namespace on delete.
		if err := persistStatus(ctx, r.Client, ns); err != nil {
			return fmt.Errorf("persist namespace ARN after create: %w", err)
		}
	} else if err != nil {
		return err
	} else {
		ns.Status.WorkspaceID = workspaceID
		if ns.Status.ObservedGeneration != ns.Generation {
			if _, err := r.AMPClient.PutRuleGroupsNamespace(ctx, &awsamp.PutRuleGroupsNamespaceInput{
				WorkspaceId: aws.String(workspaceID),
				Name:        aws.String(ns.Spec.Name),
				Data:        []byte(ns.Spec.Data),
			}); err != nil {
				return fmt.Errorf("put Prometheus rule groups namespace: %w", err)
			}
		}
	}

	ns.Status.ObservedGeneration = ns.Generation
	now := metav1.Now()
	ns.Status.LastSyncTime = &now
	return r.setCondition(ctx, ns, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Prometheus rule groups namespace reconciled")
}

func (r *PrometheusRuleGroupsNamespaceReconciler) deleteNamespace(ctx context.Context, ns *awsv1alpha1.PrometheusRuleGroupsNamespace) error {
	workspaceID := ns.Status.WorkspaceID
	if workspaceID == "" {
		if ns.Spec.WorkspaceRef.WorkspaceID != "" {
			workspaceID = ns.Spec.WorkspaceRef.WorkspaceID
		} else if ns.Spec.WorkspaceRef.Name != "" {
			ws := &awsv1alpha1.PrometheusWorkspace{}
			err := r.Get(ctx, k8stypes.NamespacedName{Name: ns.Spec.WorkspaceRef.Name, Namespace: ns.Namespace}, ws)
			if err != nil {
				// Workspace CR gone: the namespace was deleted with the workspace.
				return client.IgnoreNotFound(err)
			}
			workspaceID = ws.Status.WorkspaceID
		}
	}
	if workspaceID == "" {
		return nil
	}
	_, err := r.AMPClient.DeleteRuleGroupsNamespace(ctx, &awsamp.DeleteRuleGroupsNamespaceInput{
		WorkspaceId: aws.String(workspaceID),
		Name:        aws.String(ns.Spec.Name),
	})
	if amphelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *PrometheusRuleGroupsNamespaceReconciler) setCondition(ctx context.Context, ns *awsv1alpha1.PrometheusRuleGroupsNamespace, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&ns.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: ns.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, ns); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *PrometheusRuleGroupsNamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.PrometheusRuleGroupsNamespace{}).
		Complete(r)
}
