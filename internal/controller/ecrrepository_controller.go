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
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ecrhelper "github.com/konfig-io/konfig-konector/internal/aws/ecr"
)

// ECRRepositoryAWSAPI is the subset of the ECR API used by this controller.
type ECRRepositoryAWSAPI interface {
	DescribeRepositories(ctx context.Context, params *awsecr.DescribeRepositoriesInput, optFns ...func(*awsecr.Options)) (*awsecr.DescribeRepositoriesOutput, error)
	CreateRepository(ctx context.Context, params *awsecr.CreateRepositoryInput, optFns ...func(*awsecr.Options)) (*awsecr.CreateRepositoryOutput, error)
	DeleteRepository(ctx context.Context, params *awsecr.DeleteRepositoryInput, optFns ...func(*awsecr.Options)) (*awsecr.DeleteRepositoryOutput, error)
}

// ECRRepositoryReconciler reconciles ECRRepository objects.
type ECRRepositoryReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	ECRClient ECRRepositoryAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecrrepositories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecrrepositories/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecrrepositories/finalizers,verbs=update

func (r *ECRRepositoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	repo := &awsv1alpha1.ECRRepository{}
	if err := r.Get(ctx, req.NamespacedName, repo); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, repo); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !repo.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(repo, awsv1alpha1.FinalizerName) {
			if shouldAbandon(repo) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(repo, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, repo)
			}
			if err := r.deleteECRRepository(ctx, repo); err != nil {
				logger.Error(err, "failed to delete ECRRepository")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(repo, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, repo)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(repo, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(repo, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, repo); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileECRRepository(ctx, repo); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionRepo(ctx, repo, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ECRRepositoryReconciler) reconcileECRRepository(ctx context.Context, repo *awsv1alpha1.ECRRepository) error {
	if repo.Status.ARN != "" {
		out, err := r.ECRClient.DescribeRepositories(ctx, &awsecr.DescribeRepositoriesInput{
			RepositoryNames: []string{repo.Spec.RepositoryName},
		})
		if err != nil && !ecrhelper.IsNotFound(err) {
			return fmt.Errorf("describe repository: %w", err)
		}
		if err == nil && len(out.Repositories) > 0 {
			repo.Status.RepositoryURI = aws.ToString(out.Repositories[0].RepositoryUri)
			repo.Status.ObservedGeneration = repo.Generation
			now := metav1.Now()
			repo.Status.LastSyncTime = &now
			return r.setConditionRepo(ctx, repo, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ECRRepository reconciled")
		}
		repo.Status.ARN = ""
	}

	input := &awsecr.CreateRepositoryInput{
		RepositoryName: aws.String(repo.Spec.RepositoryName),
	}
	if repo.Spec.ImageTagMutability != "" {
		input.ImageTagMutability = ecrtypes.ImageTagMutability(repo.Spec.ImageTagMutability)
	}
	if repo.Spec.ScanOnPush {
		input.ImageScanningConfiguration = &ecrtypes.ImageScanningConfiguration{
			ScanOnPush: repo.Spec.ScanOnPush,
		}
	}
	if repo.Spec.EncryptionType != "" {
		input.EncryptionConfiguration = &ecrtypes.EncryptionConfiguration{
			EncryptionType: ecrtypes.EncryptionType(repo.Spec.EncryptionType),
		}
		if repo.Spec.KMSKeyARN != "" {
			input.EncryptionConfiguration.KmsKey = aws.String(repo.Spec.KMSKeyARN)
		}
	}
	if len(repo.Spec.Tags) > 0 {
		tags := make([]ecrtypes.Tag, 0, len(repo.Spec.Tags))
		for k, v := range repo.Spec.Tags {
			k, v := k, v
			tags = append(tags, ecrtypes.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.ECRClient.CreateRepository(ctx, input)
	if err != nil {
		return fmt.Errorf("create repository: %w", err)
	}

	repo.Status.ARN = aws.ToString(out.Repository.RepositoryArn)
	repo.Status.RepositoryURI = aws.ToString(out.Repository.RepositoryUri)
	repo.Status.ObservedGeneration = repo.Generation
	now := metav1.Now()
	repo.Status.LastSyncTime = &now
	return r.setConditionRepo(ctx, repo, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "ECRRepository created")
}

func (r *ECRRepositoryReconciler) deleteECRRepository(ctx context.Context, repo *awsv1alpha1.ECRRepository) error {
	if repo.Spec.RepositoryName == "" {
		return nil
	}
	_, err := r.ECRClient.DeleteRepository(ctx, &awsecr.DeleteRepositoryInput{
		RepositoryName: aws.String(repo.Spec.RepositoryName),
		Force:          true,
	})
	if ecrhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ECRRepositoryReconciler) setConditionRepo(ctx context.Context, repo *awsv1alpha1.ECRRepository, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&repo.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: repo.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, repo); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *ECRRepositoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ECRRepository{}).
		Complete(r)
}
