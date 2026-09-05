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

// ECRLifecyclePolicyReconciler reconciles ECRLifecyclePolicy objects.
type ECRLifecyclePolicyReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	ECRClient *multi.ECR
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecrlifecyclepolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecrlifecyclepolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecrlifecyclepolicies/finalizers,verbs=update

func (r *ECRLifecyclePolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	lp := &awsv1alpha1.ECRLifecyclePolicy{}
	if err := r.Get(ctx, req.NamespacedName, lp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, lp); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !lp.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(lp, awsv1alpha1.FinalizerName) {
			if shouldAbandon(lp) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(lp, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, lp)
			}
			if err := r.deleteECRLifecyclePolicy(ctx, lp); err != nil {
				logger.Error(err, "failed to delete ECRLifecyclePolicy")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(lp, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, lp)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(lp, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(lp, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, lp); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileECRLifecyclePolicy(ctx, lp); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionLP(ctx, lp, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ECRLifecyclePolicyReconciler) reconcileECRLifecyclePolicy(ctx context.Context, lp *awsv1alpha1.ECRLifecyclePolicy) error {
	repoName, err := r.resolveRepoName(ctx, lp)
	if err != nil {
		return err
	}

	_, err = r.ECRClient.PutLifecyclePolicy(ctx, &awsecr.PutLifecyclePolicyInput{
		RepositoryName:      aws.String(repoName),
		LifecyclePolicyText: aws.String(lp.Spec.LifecyclePolicyDocument),
	})
	if err != nil {
		return fmt.Errorf("put lifecycle policy: %w", err)
	}

	lp.Status.ObservedGeneration = lp.Generation
	now := metav1.Now()
	lp.Status.LastSyncTime = &now
	return r.setConditionLP(ctx, lp, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ECRLifecyclePolicy reconciled")
}

func (r *ECRLifecyclePolicyReconciler) resolveRepoName(ctx context.Context, lp *awsv1alpha1.ECRLifecyclePolicy) (string, error) {
	ref := lp.Spec.RepositoryRef
	if ref.RepositoryName != "" {
		return ref.RepositoryName, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("repositoryRef requires name or repositoryName")
	}
	repoCR := &awsv1alpha1.ECRRepository{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: lp.Namespace}, repoCR); err != nil {
		return "", err
	}
	if repoCR.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("ECRRepository %s/%s has no ARN yet", lp.Namespace, ref.Name)}
	}
	return repoCR.Spec.RepositoryName, nil
}

func (r *ECRLifecyclePolicyReconciler) deleteECRLifecyclePolicy(ctx context.Context, lp *awsv1alpha1.ECRLifecyclePolicy) error {
	repoName, err := r.resolveRepoName(ctx, lp)
	if err != nil {
		return nil
	}
	_, err = r.ECRClient.DeleteLifecyclePolicy(ctx, &awsecr.DeleteLifecyclePolicyInput{
		RepositoryName: aws.String(repoName),
	})
	if ecrhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ECRLifecyclePolicyReconciler) setConditionLP(ctx context.Context, lp *awsv1alpha1.ECRLifecyclePolicy, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&lp.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: lp.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, lp); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *ECRLifecyclePolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ECRLifecyclePolicy{}).
		Complete(r)
}
