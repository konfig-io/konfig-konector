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
	awsacm "github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	acmhelper "github.com/konfig-io/konfig-konector/internal/aws/acm"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// CertificateReconciler reconciles Certificate objects.
type CertificateReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	ACMClient *multi.ACM
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=certificates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=certificates/finalizers,verbs=update

func (r *CertificateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	cert := &awsv1alpha1.Certificate{}
	if err := r.Get(ctx, req.NamespacedName, cert); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, cert); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !cert.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(cert, awsv1alpha1.FinalizerName) {
			if shouldAbandon(cert) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(cert, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, cert)
			}
			if err := r.deleteCertificate(ctx, cert); err != nil {
				logger.Error(err, "failed to delete Certificate")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(cert, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, cert)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(cert, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(cert, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, cert); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileCertificate(ctx, cert); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionCert(ctx, cert, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *CertificateReconciler) reconcileCertificate(ctx context.Context, cert *awsv1alpha1.Certificate) error {
	if cert.Status.ARN != "" {
		out, err := r.ACMClient.DescribeCertificate(ctx, &awsacm.DescribeCertificateInput{
			CertificateArn: aws.String(cert.Status.ARN),
		})
		if err != nil && !acmhelper.IsNotFound(err) {
			return fmt.Errorf("describe certificate: %w", err)
		}
		if err == nil {
			cert.Status.CertificateStatus = string(out.Certificate.Status)
			cert.Status.ObservedGeneration = cert.Generation
			now := metav1.Now()
			cert.Status.LastSyncTime = &now
			return r.setConditionCert(ctx, cert, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Certificate reconciled")
		}
		cert.Status.ARN = ""
	}

	validationMethod := acmtypes.ValidationMethodDns
	if cert.Spec.ValidationMethod == "EMAIL" {
		validationMethod = acmtypes.ValidationMethodEmail
	}

	input := &awsacm.RequestCertificateInput{
		DomainName:       aws.String(cert.Spec.DomainName),
		ValidationMethod: validationMethod,
	}
	if len(cert.Spec.SubjectAlternativeNames) > 0 {
		input.SubjectAlternativeNames = cert.Spec.SubjectAlternativeNames
	}
	if len(cert.Spec.Tags) > 0 {
		tags := make([]acmtypes.Tag, 0, len(cert.Spec.Tags))
		for k, v := range cert.Spec.Tags {
			k, v := k, v
			tags = append(tags, acmtypes.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.ACMClient.RequestCertificate(ctx, input)
	if err != nil {
		return fmt.Errorf("request certificate: %w", err)
	}

	cert.Status.ARN = aws.ToString(out.CertificateArn)
	// Persist the ARN immediately: the AWS resource now exists, and losing the
	// identifier would orphan it or create a duplicate on retry.
	if err := persistStatus(ctx, r.Client, cert); err != nil {
		return fmt.Errorf("persist certificate ARN after create: %w", err)
	}
	cert.Status.CertificateStatus = "PENDING_VALIDATION"
	cert.Status.ObservedGeneration = cert.Generation
	now := metav1.Now()
	cert.Status.LastSyncTime = &now
	return r.setConditionCert(ctx, cert, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "Certificate requested")
}

func (r *CertificateReconciler) deleteCertificate(ctx context.Context, cert *awsv1alpha1.Certificate) error {
	if cert.Status.ARN == "" {
		return nil
	}
	_, err := r.ACMClient.DeleteCertificate(ctx, &awsacm.DeleteCertificateInput{
		CertificateArn: aws.String(cert.Status.ARN),
	})
	if acmhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *CertificateReconciler) setConditionCert(ctx context.Context, cert *awsv1alpha1.Certificate, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&cert.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: cert.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, cert); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *CertificateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.Certificate{}).
		Complete(r)
}
