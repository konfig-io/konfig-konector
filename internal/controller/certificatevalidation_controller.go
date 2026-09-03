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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// CertificateValidationReconciler reconciles CertificateValidation objects.
type CertificateValidationReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	ACMClient *awsacm.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=certificatevalidations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=certificatevalidations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=certificatevalidations/finalizers,verbs=update

func (r *CertificateValidationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	cv := &awsv1alpha1.CertificateValidation{}
	if err := r.Get(ctx, req.NamespacedName, cv); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !cv.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(cv, awsv1alpha1.FinalizerName) {
			if shouldAbandon(cv) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(cv, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, cv)
			}
			controllerutil.RemoveFinalizer(cv, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, cv)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(cv, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(cv, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, cv); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileCertificateValidation(ctx, cv); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionCV(ctx, cv, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *CertificateValidationReconciler) reconcileCertificateValidation(ctx context.Context, cv *awsv1alpha1.CertificateValidation) error {
	// Resolve the Certificate CR.
	certCR := &awsv1alpha1.Certificate{}
	ref := cv.Spec.CertificateRef
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: cv.Namespace}, certCR); err != nil {
		return err
	}
	if certCR.Status.ARN == "" {
		return &dependencyNotReady{msg: fmt.Sprintf("Certificate %s/%s has no ARN yet", cv.Namespace, ref.Name)}
	}

	out, err := r.ACMClient.DescribeCertificate(ctx, &awsacm.DescribeCertificateInput{
		CertificateArn: aws.String(certCR.Status.ARN),
	})
	if err != nil {
		return fmt.Errorf("describe certificate: %w", err)
	}

	cv.Status.ValidationStatus = string(out.Certificate.Status)
	cv.Status.ObservedGeneration = cv.Generation
	now := metav1.Now()
	cv.Status.LastSyncTime = &now

	if out.Certificate.Status == "ISSUED" {
		return r.setConditionCV(ctx, cv, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Certificate is ISSUED")
	}
	return r.setConditionCV(ctx, cv, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "PendingValidation", fmt.Sprintf("certificate status: %s", cv.Status.ValidationStatus))
}

func (r *CertificateValidationReconciler) setConditionCV(ctx context.Context, cv *awsv1alpha1.CertificateValidation, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&cv.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: cv.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, cv); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *CertificateValidationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CertificateValidation{}).
		Complete(r)
}
