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
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscc "github.com/aws/aws-sdk-go-v2/service/codecommit"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	cchelper "github.com/konfig-io/konfig-konector/internal/aws/codecommit"
)

// CodeCommitRepositoryReconciler reconciles CodeCommitRepository objects.
type CodeCommitRepositoryReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	CodeCommitClient *awscc.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=codecommitrepositories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=codecommitrepositories/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=codecommitrepositories/finalizers,verbs=update

func (r *CodeCommitRepositoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.CodeCommitRepository{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
			if shouldAbandon(obj) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, obj)
			}
			if err := r.deleteRepository(ctx, obj); err != nil {
				logger.Error(err, "failed to delete CodeCommitRepository")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, obj)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(obj, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileRepository(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionCCR(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *CodeCommitRepositoryReconciler) reconcileRepository(ctx context.Context, obj *awsv1alpha1.CodeCommitRepository) error {
	getOut, err := r.CodeCommitClient.GetRepository(ctx, &awscc.GetRepositoryInput{
		RepositoryName: aws.String(obj.Spec.RepositoryName),
	})
	if err != nil && !cchelper.IsNotFound(err) {
		return fmt.Errorf("get codecommit repository: %w", err)
	}

	if err == nil && getOut.RepositoryMetadata != nil {
		repoMeta := getOut.RepositoryMetadata
		obj.Status.RepositoryID = aws.ToString(repoMeta.RepositoryId)
		obj.Status.RepositoryARN = aws.ToString(repoMeta.Arn)
		obj.Status.CloneURLHTTP = aws.ToString(repoMeta.CloneUrlHttp)

		if obj.Spec.RepositoryDescription != "" {
			if _, err := r.CodeCommitClient.UpdateRepositoryDescription(ctx, &awscc.UpdateRepositoryDescriptionInput{
				RepositoryName:        aws.String(obj.Spec.RepositoryName),
				RepositoryDescription: aws.String(obj.Spec.RepositoryDescription),
			}); err != nil {
				return fmt.Errorf("update codecommit repository description: %w", err)
			}
		}

		obj.Status.ObservedGeneration = obj.Generation
		now := metav1.Now()
		obj.Status.LastSyncTime = &now
		return r.setConditionCCR(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "CodeCommitRepository reconciled")
	}

	tags := make(map[string]string, len(obj.Spec.Tags))
	for k, v := range obj.Spec.Tags {
		tags[k] = v
	}

	input := &awscc.CreateRepositoryInput{
		RepositoryName: aws.String(obj.Spec.RepositoryName),
		Tags:           tags,
	}
	if obj.Spec.RepositoryDescription != "" {
		input.RepositoryDescription = aws.String(obj.Spec.RepositoryDescription)
	}

	out, err := r.CodeCommitClient.CreateRepository(ctx, input)
	if err != nil {
		return fmt.Errorf("create codecommit repository: %w", err)
	}

	if out.RepositoryMetadata != nil {
		obj.Status.RepositoryID = aws.ToString(out.RepositoryMetadata.RepositoryId)
		obj.Status.RepositoryARN = aws.ToString(out.RepositoryMetadata.Arn)
		obj.Status.CloneURLHTTP = aws.ToString(out.RepositoryMetadata.CloneUrlHttp)
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionCCR(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "CodeCommitRepository created")
}

func (r *CodeCommitRepositoryReconciler) deleteRepository(ctx context.Context, obj *awsv1alpha1.CodeCommitRepository) error {
	_, err := r.CodeCommitClient.DeleteRepository(ctx, &awscc.DeleteRepositoryInput{
		RepositoryName: aws.String(obj.Spec.RepositoryName),
	})
	if cchelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *CodeCommitRepositoryReconciler) setConditionCCR(ctx context.Context, obj *awsv1alpha1.CodeCommitRepository, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: obj.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *CodeCommitRepositoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CodeCommitRepository{}).
		Complete(r)
}
