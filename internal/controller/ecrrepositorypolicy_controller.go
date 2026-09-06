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
	awsecr "github.com/aws/aws-sdk-go-v2/service/ecr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ecrhelper "github.com/konfig-io/konfig-konector/internal/aws/ecr"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// ECRRepositoryPolicyReconciler reconciles ECRRepositoryPolicy objects.
type ECRRepositoryPolicyReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	ECRClient *multi.ECR
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecrrepositorypolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecrrepositorypolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecrrepositorypolicies/finalizers,verbs=update

func (r *ECRRepositoryPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	rp := &awsv1alpha1.ECRRepositoryPolicy{}
	if err := r.Get(ctx, req.NamespacedName, rp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, rp); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !rp.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(rp, awsv1alpha1.FinalizerName) {
			if shouldAbandon(rp) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(rp, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, rp)
			}
			if err := r.deleteECRRepositoryPolicy(ctx, rp); err != nil {
				logger.Error(err, "failed to delete ECRRepositoryPolicy")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(rp, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, rp)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(rp, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(rp, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, rp); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileECRRepositoryPolicy(ctx, rp); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionRP(ctx, rp, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ECRRepositoryPolicyReconciler) reconcileECRRepositoryPolicy(ctx context.Context, rp *awsv1alpha1.ECRRepositoryPolicy) error {
	repoName, err := r.resolveRepositoryName(ctx, rp)
	if err != nil {
		return err
	}

	_, err = r.ECRClient.SetRepositoryPolicy(ctx, &awsecr.SetRepositoryPolicyInput{
		RepositoryName: aws.String(repoName),
		PolicyText:     aws.String(rp.Spec.PolicyDocument),
	})
	if err != nil {
		return fmt.Errorf("set repository policy: %w", err)
	}

	rp.Status.ObservedGeneration = rp.Generation
	now := metav1.Now()
	rp.Status.LastSyncTime = &now
	return r.setConditionRP(ctx, rp, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ECRRepositoryPolicy reconciled")
}

func (r *ECRRepositoryPolicyReconciler) resolveRepositoryName(ctx context.Context, rp *awsv1alpha1.ECRRepositoryPolicy) (string, error) {
	ref := rp.Spec.RepositoryRef
	if ref.RepositoryName != "" {
		return ref.RepositoryName, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("repositoryRef requires name or repositoryName")
	}
	repoCR := &awsv1alpha1.ECRRepository{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: rp.Namespace}, repoCR); err != nil {
		return "", err
	}
	if repoCR.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("ECRRepository %s/%s has no ARN yet", rp.Namespace, ref.Name)}
	}
	return repoCR.Spec.RepositoryName, nil
}

func (r *ECRRepositoryPolicyReconciler) deleteECRRepositoryPolicy(ctx context.Context, rp *awsv1alpha1.ECRRepositoryPolicy) error {
	repoName, err := r.resolveRepositoryName(ctx, rp)
	if err != nil {
		return nil
	}
	_, err = r.ECRClient.DeleteRepositoryPolicy(ctx, &awsecr.DeleteRepositoryPolicyInput{
		RepositoryName: aws.String(repoName),
	})
	if ecrhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ECRRepositoryPolicyReconciler) setConditionRP(ctx context.Context, rp *awsv1alpha1.ECRRepositoryPolicy, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&rp.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: rp.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, rp); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *ECRRepositoryPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ECRRepositoryPolicy{}).
		Complete(r)
}
