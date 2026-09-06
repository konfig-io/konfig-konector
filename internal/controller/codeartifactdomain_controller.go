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

// CodeArtifactDomainAWSAPI is the subset of the CodeArtifact API used by this controller.
type CodeArtifactDomainAWSAPI interface {
	DescribeDomain(ctx context.Context, params *awscodeartifact.DescribeDomainInput, optFns ...func(*awscodeartifact.Options)) (*awscodeartifact.DescribeDomainOutput, error)
	CreateDomain(ctx context.Context, params *awscodeartifact.CreateDomainInput, optFns ...func(*awscodeartifact.Options)) (*awscodeartifact.CreateDomainOutput, error)
	DeleteDomain(ctx context.Context, params *awscodeartifact.DeleteDomainInput, optFns ...func(*awscodeartifact.Options)) (*awscodeartifact.DeleteDomainOutput, error)
}

// CodeArtifactDomainReconciler reconciles CodeArtifactDomain objects.
type CodeArtifactDomainReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	CodeArtifactClient CodeArtifactDomainAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=codeartifactdomains,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=codeartifactdomains/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=codeartifactdomains/finalizers,verbs=update

func (r *CodeArtifactDomainReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	d := &awsv1alpha1.CodeArtifactDomain{}
	if err := r.Get(ctx, req.NamespacedName, d); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, d); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !d.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(d, awsv1alpha1.FinalizerName) {
			if shouldAbandon(d) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(d, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, d)
			}
			if err := r.deleteDomain(ctx, d); err != nil {
				logger.Error(err, "failed to delete CodeArtifact domain")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(d, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, d)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(d, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(d, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, d); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileDomain(ctx, d); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, d, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *CodeArtifactDomainReconciler) reconcileDomain(ctx context.Context, d *awsv1alpha1.CodeArtifactDomain) error {
	got, err := r.CodeArtifactClient.DescribeDomain(ctx, &awscodeartifact.DescribeDomainInput{
		Domain: aws.String(d.Spec.DomainName),
	})
	if codeartifacthelper.IsNotFound(err) {
		encryptionKey, keyErr := r.resolveEncryptionKey(ctx, d)
		if keyErr != nil {
			return keyErr
		}
		in := &awscodeartifact.CreateDomainInput{
			Domain: aws.String(d.Spec.DomainName),
		}
		if encryptionKey != "" {
			in.EncryptionKey = aws.String(encryptionKey)
		}
		for k, v := range d.Spec.Tags {
			in.Tags = append(in.Tags, codeartifacttypes.Tag{Key: aws.String(k), Value: aws.String(v)})
		}
		created, err := r.CodeArtifactClient.CreateDomain(ctx, in)
		if err != nil {
			return fmt.Errorf("create CodeArtifact domain: %w", err)
		}
		if created.Domain != nil {
			d.Status.ARN = aws.ToString(created.Domain.Arn)
			d.Status.Owner = aws.ToString(created.Domain.Owner)
		}
		// Persist the ARN immediately: the AWS resource now exists, and losing
		// the identifier would orphan it on delete.
		if err := persistStatus(ctx, r.Client, d); err != nil {
			return fmt.Errorf("persist domain ARN after create: %w", err)
		}
	} else if err != nil {
		return err
	} else if got.Domain != nil {
		d.Status.ARN = aws.ToString(got.Domain.Arn)
		d.Status.Owner = aws.ToString(got.Domain.Owner)
	}

	d.Status.ObservedGeneration = d.Generation
	now := metav1.Now()
	d.Status.LastSyncTime = &now
	return r.setCondition(ctx, d, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "CodeArtifact domain reconciled")
}

func (r *CodeArtifactDomainReconciler) resolveEncryptionKey(ctx context.Context, d *awsv1alpha1.CodeArtifactDomain) (string, error) {
	ref := d.Spec.EncryptionKeyRef
	if ref == nil {
		return "", nil
	}
	if ref.KeyID != "" {
		return ref.KeyID, nil
	}
	keyCR := &awsv1alpha1.KMSKey{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: d.Namespace}, keyCR); err != nil {
		return "", err
	}
	if keyCR.Status.KeyID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("KMSKey %s/%s has no key ID yet", d.Namespace, ref.Name)}
	}
	return keyCR.Status.KeyID, nil
}

func (r *CodeArtifactDomainReconciler) deleteDomain(ctx context.Context, d *awsv1alpha1.CodeArtifactDomain) error {
	// The domain name in spec is the deterministic AWS identifier.
	_, err := r.CodeArtifactClient.DeleteDomain(ctx, &awscodeartifact.DeleteDomainInput{
		Domain: aws.String(d.Spec.DomainName),
	})
	if codeartifacthelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *CodeArtifactDomainReconciler) setCondition(ctx context.Context, d *awsv1alpha1.CodeArtifactDomain, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&d.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: d.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, d); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *CodeArtifactDomainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CodeArtifactDomain{}).
		Complete(r)
}
