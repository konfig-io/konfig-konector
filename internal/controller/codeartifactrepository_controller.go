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
	awscodeartifact "github.com/aws/aws-sdk-go-v2/service/codeartifact"
	codeartifacttypes "github.com/aws/aws-sdk-go-v2/service/codeartifact/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	codeartifacthelper "github.com/konfig-io/konfig-konector/internal/aws/codeartifact"
)

// CodeArtifactRepositoryAWSAPI is the subset of the CodeArtifact API used by this controller.
type CodeArtifactRepositoryAWSAPI interface {
	DescribeRepository(ctx context.Context, params *awscodeartifact.DescribeRepositoryInput, optFns ...func(*awscodeartifact.Options)) (*awscodeartifact.DescribeRepositoryOutput, error)
	CreateRepository(ctx context.Context, params *awscodeartifact.CreateRepositoryInput, optFns ...func(*awscodeartifact.Options)) (*awscodeartifact.CreateRepositoryOutput, error)
	UpdateRepository(ctx context.Context, params *awscodeartifact.UpdateRepositoryInput, optFns ...func(*awscodeartifact.Options)) (*awscodeartifact.UpdateRepositoryOutput, error)
	DeleteRepository(ctx context.Context, params *awscodeartifact.DeleteRepositoryInput, optFns ...func(*awscodeartifact.Options)) (*awscodeartifact.DeleteRepositoryOutput, error)
	AssociateExternalConnection(ctx context.Context, params *awscodeartifact.AssociateExternalConnectionInput, optFns ...func(*awscodeartifact.Options)) (*awscodeartifact.AssociateExternalConnectionOutput, error)
	DisassociateExternalConnection(ctx context.Context, params *awscodeartifact.DisassociateExternalConnectionInput, optFns ...func(*awscodeartifact.Options)) (*awscodeartifact.DisassociateExternalConnectionOutput, error)
}

// CodeArtifactRepositoryReconciler reconciles CodeArtifactRepository objects.
type CodeArtifactRepositoryReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	CodeArtifactClient CodeArtifactRepositoryAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=codeartifactrepositories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=codeartifactrepositories/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=codeartifactrepositories/finalizers,verbs=update

func (r *CodeArtifactRepositoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	repo := &awsv1alpha1.CodeArtifactRepository{}
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
			if err := r.deleteRepository(ctx, repo); err != nil {
				logger.Error(err, "failed to delete CodeArtifact repository")
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
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileRepository(ctx, repo); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, repo, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *CodeArtifactRepositoryReconciler) resolveDomainName(ctx context.Context, repo *awsv1alpha1.CodeArtifactRepository) (string, error) {
	ref := repo.Spec.DomainRef
	if ref.DomainName != "" {
		return ref.DomainName, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("domainRef requires either name or domainName")
	}
	domainCR := &awsv1alpha1.CodeArtifactDomain{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: repo.Namespace}, domainCR); err != nil {
		return "", err
	}
	if domainCR.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("CodeArtifactDomain %s/%s has no ARN yet", repo.Namespace, ref.Name)}
	}
	return domainCR.Spec.DomainName, nil
}

func (r *CodeArtifactRepositoryReconciler) upstreams(repo *awsv1alpha1.CodeArtifactRepository) []codeartifacttypes.UpstreamRepository {
	ups := make([]codeartifacttypes.UpstreamRepository, 0, len(repo.Spec.Upstreams))
	for _, u := range repo.Spec.Upstreams {
		ups = append(ups, codeartifacttypes.UpstreamRepository{RepositoryName: aws.String(u)})
	}
	return ups
}

func (r *CodeArtifactRepositoryReconciler) reconcileRepository(ctx context.Context, repo *awsv1alpha1.CodeArtifactRepository) error {
	domain, err := r.resolveDomainName(ctx, repo)
	if err != nil {
		return err
	}

	got, err := r.CodeArtifactClient.DescribeRepository(ctx, &awscodeartifact.DescribeRepositoryInput{
		Domain:     aws.String(domain),
		Repository: aws.String(repo.Spec.RepositoryName),
	})

	var existingConnections []string
	if codeartifacthelper.IsNotFound(err) {
		in := &awscodeartifact.CreateRepositoryInput{
			Domain:     aws.String(domain),
			Repository: aws.String(repo.Spec.RepositoryName),
			Upstreams:  r.upstreams(repo),
		}
		if repo.Spec.Description != "" {
			in.Description = aws.String(repo.Spec.Description)
		}
		for k, v := range repo.Spec.Tags {
			in.Tags = append(in.Tags, codeartifacttypes.Tag{Key: aws.String(k), Value: aws.String(v)})
		}
		created, err := r.CodeArtifactClient.CreateRepository(ctx, in)
		if err != nil {
			return fmt.Errorf("create CodeArtifact repository: %w", err)
		}
		if created.Repository != nil {
			repo.Status.ARN = aws.ToString(created.Repository.Arn)
		}
		repo.Status.DomainName = domain
		// Persist the ARN immediately: the AWS resource now exists, and losing
		// the identifier would orphan it on delete.
		if err := persistStatus(ctx, r.Client, repo); err != nil {
			return fmt.Errorf("persist repository ARN after create: %w", err)
		}
	} else if err != nil {
		return err
	} else {
		if got.Repository != nil {
			repo.Status.ARN = aws.ToString(got.Repository.Arn)
			for _, ec := range got.Repository.ExternalConnections {
				existingConnections = append(existingConnections, aws.ToString(ec.ExternalConnectionName))
			}
		}
		repo.Status.DomainName = domain

		if repo.Status.ObservedGeneration != repo.Generation {
			in := &awscodeartifact.UpdateRepositoryInput{
				Domain:     aws.String(domain),
				Repository: aws.String(repo.Spec.RepositoryName),
				Upstreams:  r.upstreams(repo),
			}
			if repo.Spec.Description != "" {
				in.Description = aws.String(repo.Spec.Description)
			}
			if _, err := r.CodeArtifactClient.UpdateRepository(ctx, in); err != nil {
				return fmt.Errorf("update CodeArtifact repository: %w", err)
			}
		}
	}

	if err := r.syncExternalConnections(ctx, repo, domain, existingConnections); err != nil {
		return err
	}

	repo.Status.ObservedGeneration = repo.Generation
	now := metav1.Now()
	repo.Status.LastSyncTime = &now
	return r.setCondition(ctx, repo, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "CodeArtifact repository reconciled")
}

func (r *CodeArtifactRepositoryReconciler) syncExternalConnections(ctx context.Context, repo *awsv1alpha1.CodeArtifactRepository, domain string, existing []string) error {
	want := make(map[string]bool, len(repo.Spec.ExternalConnections))
	for _, ec := range repo.Spec.ExternalConnections {
		want[ec] = true
	}
	have := make(map[string]bool, len(existing))
	for _, ec := range existing {
		have[ec] = true
	}
	for _, ec := range existing {
		if !want[ec] {
			if _, err := r.CodeArtifactClient.DisassociateExternalConnection(ctx, &awscodeartifact.DisassociateExternalConnectionInput{
				Domain:             aws.String(domain),
				Repository:         aws.String(repo.Spec.RepositoryName),
				ExternalConnection: aws.String(ec),
			}); err != nil {
				return fmt.Errorf("disassociate external connection %s: %w", ec, err)
			}
		}
	}
	for _, ec := range repo.Spec.ExternalConnections {
		if !have[ec] {
			if _, err := r.CodeArtifactClient.AssociateExternalConnection(ctx, &awscodeartifact.AssociateExternalConnectionInput{
				Domain:             aws.String(domain),
				Repository:         aws.String(repo.Spec.RepositoryName),
				ExternalConnection: aws.String(ec),
			}); err != nil {
				return fmt.Errorf("associate external connection %s: %w", ec, err)
			}
		}
	}
	return nil
}

func (r *CodeArtifactRepositoryReconciler) deleteRepository(ctx context.Context, repo *awsv1alpha1.CodeArtifactRepository) error {
	domain := repo.Status.DomainName
	if domain == "" {
		// The domain may never have been resolved (status lost or the create
		// never ran); try to derive it from spec without waiting on readiness.
		if repo.Spec.DomainRef.DomainName != "" {
			domain = repo.Spec.DomainRef.DomainName
		} else if repo.Spec.DomainRef.Name != "" {
			domainCR := &awsv1alpha1.CodeArtifactDomain{}
			err := r.Get(ctx, k8stypes.NamespacedName{Name: repo.Spec.DomainRef.Name, Namespace: repo.Namespace}, domainCR)
			if err != nil {
				// Domain CR already gone: nothing we can unambiguously delete.
				return client.IgnoreNotFound(err)
			}
			domain = domainCR.Spec.DomainName
		}
	}
	if domain == "" {
		return nil
	}
	_, err := r.CodeArtifactClient.DeleteRepository(ctx, &awscodeartifact.DeleteRepositoryInput{
		Domain:     aws.String(domain),
		Repository: aws.String(repo.Spec.RepositoryName),
	})
	if codeartifacthelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *CodeArtifactRepositoryReconciler) setCondition(ctx context.Context, repo *awsv1alpha1.CodeArtifactRepository, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *CodeArtifactRepositoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CodeArtifactRepository{}).
		Complete(r)
}
