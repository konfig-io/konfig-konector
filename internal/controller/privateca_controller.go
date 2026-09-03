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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsacmpca "github.com/aws/aws-sdk-go-v2/service/acmpca"
	acmpcatypes "github.com/aws/aws-sdk-go-v2/service/acmpca/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	acmpcahelper "github.com/konfig-io/konfig-konector/internal/aws/acmpca"
)

var requeuePCAPolling = ctrl.Result{RequeueAfter: 30 * time.Second}

// PrivateCAAWSAPI is the subset of the ACM PCA API used by this controller.
type PrivateCAAWSAPI interface {
	CreateCertificateAuthority(ctx context.Context, params *awsacmpca.CreateCertificateAuthorityInput, optFns ...func(*awsacmpca.Options)) (*awsacmpca.CreateCertificateAuthorityOutput, error)
	DescribeCertificateAuthority(ctx context.Context, params *awsacmpca.DescribeCertificateAuthorityInput, optFns ...func(*awsacmpca.Options)) (*awsacmpca.DescribeCertificateAuthorityOutput, error)
	UpdateCertificateAuthority(ctx context.Context, params *awsacmpca.UpdateCertificateAuthorityInput, optFns ...func(*awsacmpca.Options)) (*awsacmpca.UpdateCertificateAuthorityOutput, error)
	DeleteCertificateAuthority(ctx context.Context, params *awsacmpca.DeleteCertificateAuthorityInput, optFns ...func(*awsacmpca.Options)) (*awsacmpca.DeleteCertificateAuthorityOutput, error)
}

// PrivateCAReconciler reconciles PrivateCA objects.
type PrivateCAReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	ACMPCAClient PrivateCAAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=privatecas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=privatecas/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=privatecas/finalizers,verbs=update

func (r *PrivateCAReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ca := &awsv1alpha1.PrivateCA{}
	if err := r.Get(ctx, req.NamespacedName, ca); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !ca.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ca, awsv1alpha1.FinalizerName) {
			if shouldAbandon(ca) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(ca, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, ca)
			}
			if err := r.deleteCertificateAuthority(ctx, ca); err != nil {
				logger.Error(err, "failed to delete private CA")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(ca, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, ca)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(ca, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(ca, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, ca); err != nil {
			return ctrl.Result{}, err
		}
	}

	result, err := r.reconcileCertificateAuthority(ctx, ca)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, ca, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *PrivateCAReconciler) reconcileCertificateAuthority(ctx context.Context, ca *awsv1alpha1.PrivateCA) (ctrl.Result, error) {
	if ca.Status.ARN != "" {
		out, err := r.ACMPCAClient.DescribeCertificateAuthority(ctx, &awsacmpca.DescribeCertificateAuthorityInput{
			CertificateAuthorityArn: aws.String(ca.Status.ARN),
		})
		if err != nil && !acmpcahelper.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err == nil && out.CertificateAuthority != nil {
			detail := out.CertificateAuthority
			ca.Status.Status = string(detail.Status)

			switch detail.Status {
			case acmpcatypes.CertificateAuthorityStatusCreating:
				_ = r.setCondition(ctx, ca, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "private CA is CREATING")
				return requeuePCAPolling, nil
			case acmpcatypes.CertificateAuthorityStatusFailed:
				_ = r.setCondition(ctx, ca, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError,
					fmt.Sprintf("private CA failed: %s", detail.FailureReason))
				return requeueResult(), nil
			}

			if ca.Status.ObservedGeneration != ca.Generation {
				updateIn := &awsacmpca.UpdateCertificateAuthorityInput{
					CertificateAuthorityArn: aws.String(ca.Status.ARN),
				}
				if rc := buildPCARevocationConfiguration(ca.Spec.RevocationConfiguration); rc != nil {
					updateIn.RevocationConfiguration = rc
				}
				if _, err := r.ACMPCAClient.UpdateCertificateAuthority(ctx, updateIn); err != nil {
					return ctrl.Result{}, fmt.Errorf("update private CA: %w", err)
				}
			}

			ca.Status.ObservedGeneration = ca.Generation
			now := metav1.Now()
			ca.Status.LastSyncTime = &now
			// PENDING_CERTIFICATE is the expected steady state until a CA
			// certificate is issued and imported out-of-band; the resource is
			// considered reconciled at that point.
			return requeueResult(), r.setCondition(ctx, ca, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced,
				fmt.Sprintf("private CA is %s", detail.Status))
		}
		// NotFound: fall through to create.
	}

	subject := &acmpcatypes.ASN1Subject{}
	if ca.Spec.Subject.CommonName != "" {
		subject.CommonName = aws.String(ca.Spec.Subject.CommonName)
	}
	if ca.Spec.Subject.Organization != "" {
		subject.Organization = aws.String(ca.Spec.Subject.Organization)
	}
	if ca.Spec.Subject.OrganizationalUnit != "" {
		subject.OrganizationalUnit = aws.String(ca.Spec.Subject.OrganizationalUnit)
	}
	if ca.Spec.Subject.Country != "" {
		subject.Country = aws.String(ca.Spec.Subject.Country)
	}
	if ca.Spec.Subject.State != "" {
		subject.State = aws.String(ca.Spec.Subject.State)
	}
	if ca.Spec.Subject.Locality != "" {
		subject.Locality = aws.String(ca.Spec.Subject.Locality)
	}

	createIn := &awsacmpca.CreateCertificateAuthorityInput{
		CertificateAuthorityType: acmpcatypes.CertificateAuthorityType(ca.Spec.Type),
		CertificateAuthorityConfiguration: &acmpcatypes.CertificateAuthorityConfiguration{
			KeyAlgorithm:     acmpcatypes.KeyAlgorithm(ca.Spec.KeyAlgorithm),
			SigningAlgorithm: acmpcatypes.SigningAlgorithm(ca.Spec.SigningAlgorithm),
			Subject:          subject,
		},
	}
	if rc := buildPCARevocationConfiguration(ca.Spec.RevocationConfiguration); rc != nil {
		createIn.RevocationConfiguration = rc
	}
	if ca.Spec.UsageMode != "" {
		createIn.UsageMode = acmpcatypes.CertificateAuthorityUsageMode(ca.Spec.UsageMode)
	}
	for k, v := range ca.Spec.Tags {
		k, v := k, v
		createIn.Tags = append(createIn.Tags, acmpcatypes.Tag{Key: &k, Value: &v})
	}

	created, err := r.ACMPCAClient.CreateCertificateAuthority(ctx, createIn)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create private CA: %w", err)
	}
	ca.Status.ARN = aws.ToString(created.CertificateAuthorityArn)
	ca.Status.Status = string(acmpcatypes.CertificateAuthorityStatusCreating)
	// Persist the ARN immediately: the AWS resource now exists, and losing
	// the identifier would orphan it or create a duplicate on retry.
	if err := persistStatus(ctx, r.Client, ca); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist CA ARN after create: %w", err)
	}
	_ = r.setCondition(ctx, ca, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "private CA is CREATING")
	return requeuePCAPolling, nil
}

func buildPCARevocationConfiguration(rc *awsv1alpha1.PrivateCARevocationConfiguration) *acmpcatypes.RevocationConfiguration {
	if rc == nil || rc.Crl == nil {
		return nil
	}
	crl := &acmpcatypes.CrlConfiguration{
		Enabled: aws.Bool(rc.Crl.Enabled),
	}
	if rc.Crl.Enabled {
		if rc.Crl.S3BucketName != "" {
			crl.S3BucketName = aws.String(rc.Crl.S3BucketName)
		}
		if rc.Crl.ExpirationInDays > 0 {
			crl.ExpirationInDays = aws.Int32(rc.Crl.ExpirationInDays)
		}
	}
	return &acmpcatypes.RevocationConfiguration{CrlConfiguration: crl}
}

func (r *PrivateCAReconciler) deleteCertificateAuthority(ctx context.Context, ca *awsv1alpha1.PrivateCA) error {
	if ca.Status.ARN == "" {
		// The CA ARN is server-generated; without it there is no unambiguous
		// spec-based lookup, so nothing to delete.
		return nil
	}
	// An ACTIVE CA must be disabled before it can be deleted.
	if ca.Status.Status == string(acmpcatypes.CertificateAuthorityStatusActive) {
		if _, err := r.ACMPCAClient.UpdateCertificateAuthority(ctx, &awsacmpca.UpdateCertificateAuthorityInput{
			CertificateAuthorityArn: aws.String(ca.Status.ARN),
			Status:                  acmpcatypes.CertificateAuthorityStatusDisabled,
		}); err != nil && !acmpcahelper.IsNotFound(err) {
			return fmt.Errorf("disable private CA before delete: %w", err)
		}
	}
	deletionDays := ca.Spec.PermanentDeletionTimeInDays
	if deletionDays == 0 {
		deletionDays = 30
	}
	_, err := r.ACMPCAClient.DeleteCertificateAuthority(ctx, &awsacmpca.DeleteCertificateAuthorityInput{
		CertificateAuthorityArn:     aws.String(ca.Status.ARN),
		PermanentDeletionTimeInDays: aws.Int32(deletionDays),
	})
	if acmpcahelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *PrivateCAReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.PrivateCA, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *PrivateCAReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.PrivateCA{}).
		Complete(r)
}
