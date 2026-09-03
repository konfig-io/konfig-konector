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

// AthenaNamedQueryAWSAPI is the subset of the Athena SDK client used by this
// controller. *awsathena.Client satisfies it.
type AthenaNamedQueryAWSAPI interface {
	GetNamedQuery(ctx context.Context, params *awsathena.GetNamedQueryInput, optFns ...func(*awsathena.Options)) (*awsathena.GetNamedQueryOutput, error)
	CreateNamedQuery(ctx context.Context, params *awsathena.CreateNamedQueryInput, optFns ...func(*awsathena.Options)) (*awsathena.CreateNamedQueryOutput, error)
	DeleteNamedQuery(ctx context.Context, params *awsathena.DeleteNamedQueryInput, optFns ...func(*awsathena.Options)) (*awsathena.DeleteNamedQueryOutput, error)
}

// AthenaNamedQueryReconciler reconciles AthenaNamedQuery objects.
type AthenaNamedQueryReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	AthenaClient AthenaNamedQueryAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=athenanamedqueries,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=athenanamedqueries/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=athenanamedqueries/finalizers,verbs=update

func (r *AthenaNamedQueryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	nq := &awsv1alpha1.AthenaNamedQuery{}
	if err := r.Get(ctx, req.NamespacedName, nq); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !nq.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(nq, awsv1alpha1.FinalizerName) {
			if shouldAbandon(nq) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(nq, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, nq)
			}
			if err := r.deleteNamedQuery(ctx, nq); err != nil {
				logger.Error(err, "failed to delete Athena named query")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(nq, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, nq)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(nq, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(nq, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, nq); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileNamedQuery(ctx, nq); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, nq, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *AthenaNamedQueryReconciler) reconcileNamedQuery(ctx context.Context, nq *awsv1alpha1.AthenaNamedQuery) error {
	if nq.Status.NamedQueryID != "" {
		getOut, err := r.AthenaClient.GetNamedQuery(ctx, &awsathena.GetNamedQueryInput{
			NamedQueryId: aws.String(nq.Status.NamedQueryID),
		})
		if err != nil && !athenahelper.IsNotFound(err) {
			return err
		}
		if err == nil && getOut.NamedQuery != nil {
			// Athena has no UpdateNamedQuery API for the fields managed here.
			// Delete+recreate is not allowed, so spec changes after creation
			// are reported as UpdateNotSupported without bumping
			// ObservedGeneration.
			if nq.Status.ObservedGeneration != nq.Generation && nq.Status.ObservedGeneration != 0 {
				drifted := aws.ToString(getOut.NamedQuery.Name) != nq.Spec.Name ||
					aws.ToString(getOut.NamedQuery.QueryString) != nq.Spec.QueryString ||
					aws.ToString(getOut.NamedQuery.Database) != nq.Spec.Database ||
					aws.ToString(getOut.NamedQuery.Description) != nq.Spec.Description
				if drifted {
					return r.setCondition(ctx, nq, awsv1alpha1.ConditionReady, metav1.ConditionFalse,
						awsv1alpha1.ReasonUpdateNotSupported,
						"Athena named queries cannot be updated; revert the spec or recreate the CR")
				}
			}
			nq.Status.ObservedGeneration = nq.Generation
			now := metav1.Now()
			nq.Status.LastSyncTime = &now
			return r.setCondition(ctx, nq, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Athena named query reconciled")
		}
		// The query the status pointed at is gone; fall through and recreate.
	}

	input := &awsathena.CreateNamedQueryInput{
		Name:        aws.String(nq.Spec.Name),
		Database:    aws.String(nq.Spec.Database),
		QueryString: aws.String(nq.Spec.QueryString),
	}
	if nq.Spec.WorkGroup != "" {
		input.WorkGroup = aws.String(nq.Spec.WorkGroup)
	}
	if nq.Spec.Description != "" {
		input.Description = aws.String(nq.Spec.Description)
	}
	createOut, err := r.AthenaClient.CreateNamedQuery(ctx, input)
	if err != nil {
		return fmt.Errorf("create Athena named query: %w", err)
	}
	// Persist the identifier immediately: the AWS resource now exists and the
	// generated ID is the only handle on it.
	nq.Status.NamedQueryID = aws.ToString(createOut.NamedQueryId)
	if err := persistStatus(ctx, r.Client, nq); err != nil {
		return fmt.Errorf("persist named query ID after create: %w", err)
	}

	nq.Status.ObservedGeneration = nq.Generation
	now := metav1.Now()
	nq.Status.LastSyncTime = &now
	return r.setCondition(ctx, nq, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Athena named query reconciled")
}

func (r *AthenaNamedQueryReconciler) deleteNamedQuery(ctx context.Context, nq *awsv1alpha1.AthenaNamedQuery) error {
	// NamedQuery IDs are AWS-generated; without one in status there is no
	// unambiguous spec-based lookup, so there is nothing to delete.
	if nq.Status.NamedQueryID == "" {
		return nil
	}
	_, err := r.AthenaClient.DeleteNamedQuery(ctx, &awsathena.DeleteNamedQueryInput{
		NamedQueryId: aws.String(nq.Status.NamedQueryID),
	})
	if athenahelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *AthenaNamedQueryReconciler) setCondition(ctx context.Context, nq *awsv1alpha1.AthenaNamedQuery, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&nq.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: nq.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, nq); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *AthenaNamedQueryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.AthenaNamedQuery{}).
		Complete(r)
}
