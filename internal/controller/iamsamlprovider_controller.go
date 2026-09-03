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
	"strings"

	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	iamhelper "github.com/konfig-io/konfig-konector/internal/aws/iam"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// IAMSAMLProviderReconciler reconciles IAMSAMLProvider objects.
type IAMSAMLProviderReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	IAMClient *awsiam.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamsamlproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamsamlproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamsamlproviders/finalizers,verbs=update

func (r *IAMSAMLProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	prov := &awsv1alpha1.IAMSAMLProvider{}
	if err := r.Get(ctx, req.NamespacedName, prov); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !prov.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(prov, awsv1alpha1.FinalizerName) {
			if shouldAbandon(prov) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(prov, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, prov)
			}
			arn := prov.Status.ARN
			if arn == "" {
				// Status may have been lost before it was persisted; fall back
				// to matching an existing provider by the spec name.
				found, err := r.findSAMLProviderARNByName(ctx, prov.Spec.Name)
				if err != nil {
					return ctrl.Result{}, err
				}
				arn = found
			}
			if arn != "" {
				if err := iamhelper.DeleteSAMLProvider(ctx, r.IAMClient, arn); err != nil {
					logger.Error(err, "failed to delete SAML provider")
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(prov, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, prov)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(prov, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(prov, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, prov); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileProvider(ctx, prov); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, prov, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *IAMSAMLProviderReconciler) reconcileProvider(ctx context.Context, prov *awsv1alpha1.IAMSAMLProvider) error {
	// If we already have an ARN, check the provider still exists.
	if prov.Status.ARN != "" {
		existing, err := iamhelper.GetSAMLProvider(ctx, r.IAMClient, prov.Status.ARN)
		if err != nil {
			return err
		}
		if existing == nil {
			// Provider was deleted out-of-band; recreate it.
			prov.Status.ARN = ""
		} else if prov.Status.ObservedGeneration != prov.Generation {
			if err := iamhelper.UpdateSAMLProvider(ctx, r.IAMClient, prov.Status.ARN, prov.Spec.SAMLMetadataDocument); err != nil {
				return fmt.Errorf("update SAML provider: %w", err)
			}
			if existing.ValidUntil != nil {
				prov.Status.ValidUntil = existing.ValidUntil.Format("2006-01-02T15:04:05Z")
			}
		}
	}

	if prov.Status.ARN == "" {
		arn, err := iamhelper.CreateSAMLProvider(ctx, r.IAMClient, &awsiam.CreateSAMLProviderInput{
			Name:                 aws.String(prov.Spec.Name),
			SAMLMetadataDocument: aws.String(prov.Spec.SAMLMetadataDocument),
		})
		if err != nil {
			return fmt.Errorf("create SAML provider: %w", err)
		}
		prov.Status.ARN = arn
		// Persist the ARN immediately: the AWS resource now exists, and losing
		// the identifier would orphan it or create a duplicate on retry.
		if err := persistStatus(ctx, r.Client, prov); err != nil {
			return fmt.Errorf("persist SAML provider ARN after create: %w", err)
		}
	}

	if err := iamhelper.SyncSAMLTags(ctx, r.IAMClient, prov.Status.ARN, prov.Spec.Tags); err != nil {
		return err
	}

	prov.Status.ObservedGeneration = prov.Generation
	now := metav1.Now()
	prov.Status.LastSyncTime = &now
	return r.setCondition(ctx, prov, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "SAML provider synced")
}

// findSAMLProviderARNByName lists SAML providers and returns the ARN whose
// final path component matches the spec name exactly, or "" if none matches.
func (r *IAMSAMLProviderReconciler) findSAMLProviderARNByName(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", nil
	}
	out, err := r.IAMClient.ListSAMLProviders(ctx, &awsiam.ListSAMLProvidersInput{})
	if err != nil {
		return "", err
	}
	for _, p := range out.SAMLProviderList {
		arn := aws.ToString(p.Arn)
		if strings.HasSuffix(arn, "/"+name) {
			return arn, nil
		}
	}
	return "", nil
}

func (r *IAMSAMLProviderReconciler) setCondition(ctx context.Context, prov *awsv1alpha1.IAMSAMLProvider, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&prov.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: prov.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, prov); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *IAMSAMLProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.IAMSAMLProvider{}).
		Complete(r)
}
