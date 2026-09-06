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
	"github.com/konfig-io/konfig-konector/internal/aws/multi"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// IAMOIDCProviderReconciler reconciles IAMOIDCProvider objects.
type IAMOIDCProviderReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	IAMClient *multi.IAM
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamoidcproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamoidcproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamoidcproviders/finalizers,verbs=update

func (r *IAMOIDCProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	prov := &awsv1alpha1.IAMOIDCProvider{}
	if err := r.Get(ctx, req.NamespacedName, prov); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, prov); scopeErr != nil {
		return ctrl.Result{}, scopeErr
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
				// to matching an existing provider by the spec URL.
				found, err := r.findOIDCProviderARNByURL(ctx, prov.Spec.URL)
				if err != nil {
					return ctrl.Result{}, err
				}
				arn = found
			}
			if arn != "" {
				if err := iamhelper.DeleteOIDCProvider(ctx, r.IAMClient, arn); err != nil {
					logger.Error(err, "failed to delete OIDC provider")
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
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileProvider(ctx, prov); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, prov, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *IAMOIDCProviderReconciler) reconcileProvider(ctx context.Context, prov *awsv1alpha1.IAMOIDCProvider) error {
	if prov.Status.ARN != "" {
		existing, err := iamhelper.GetOIDCProvider(ctx, r.IAMClient, prov.Status.ARN)
		if err != nil {
			return err
		}
		if existing == nil {
			prov.Status.ARN = ""
		} else if prov.Status.ObservedGeneration != prov.Generation {
			if err := iamhelper.SyncOIDCThumbprints(ctx, r.IAMClient, prov.Status.ARN, prov.Spec.ThumbprintList); err != nil {
				return fmt.Errorf("sync OIDC thumbprints: %w", err)
			}
			currentIDs := make([]string, len(existing.ClientIDList))
			copy(currentIDs, existing.ClientIDList)
			if err := iamhelper.SyncOIDCClientIDs(ctx, r.IAMClient, prov.Status.ARN, currentIDs, prov.Spec.ClientIDList); err != nil {
				return fmt.Errorf("sync OIDC client IDs: %w", err)
			}
		}
	}

	if prov.Status.ARN == "" {
		arn, err := iamhelper.CreateOIDCProvider(ctx, r.IAMClient, &awsiam.CreateOpenIDConnectProviderInput{
			Url:            aws.String(prov.Spec.URL),
			ClientIDList:   prov.Spec.ClientIDList,
			ThumbprintList: prov.Spec.ThumbprintList,
		})
		if err != nil {
			return fmt.Errorf("create OIDC provider: %w", err)
		}
		prov.Status.ARN = arn
		// Persist the ARN immediately: the AWS resource now exists, and losing
		// the identifier would orphan it or create a duplicate on retry.
		if err := persistStatus(ctx, r.Client, prov); err != nil {
			return fmt.Errorf("persist OIDC provider ARN after create: %w", err)
		}
	}

	if err := iamhelper.SyncOIDCTags(ctx, r.IAMClient, prov.Status.ARN, prov.Spec.Tags); err != nil {
		return err
	}

	prov.Status.ObservedGeneration = prov.Generation
	now := metav1.Now()
	prov.Status.LastSyncTime = &now
	return r.setCondition(ctx, prov, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "OIDC provider synced")
}

// findOIDCProviderARNByURL lists OIDC providers and returns the ARN whose
// suffix matches the spec URL (the ARN embeds the URL without its scheme), or
// "" if none matches.
func (r *IAMOIDCProviderReconciler) findOIDCProviderARNByURL(ctx context.Context, url string) (string, error) {
	if url == "" {
		return "", nil
	}
	host := strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")
	out, err := r.IAMClient.ListOpenIDConnectProviders(ctx, &awsiam.ListOpenIDConnectProvidersInput{})
	if err != nil {
		return "", err
	}
	for _, p := range out.OpenIDConnectProviderList {
		arn := aws.ToString(p.Arn)
		if strings.HasSuffix(arn, "/"+host) || strings.HasSuffix(arn, ":oidc-provider/"+host) {
			return arn, nil
		}
	}
	return "", nil
}

func (r *IAMOIDCProviderReconciler) setCondition(ctx context.Context, prov *awsv1alpha1.IAMOIDCProvider, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *IAMOIDCProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.IAMOIDCProvider{}).
		Complete(r)
}
