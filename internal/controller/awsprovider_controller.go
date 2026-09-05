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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssts "github.com/aws/aws-sdk-go-v2/service/sts"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	"github.com/konfig-io/konfig-konector/internal/aws/provider"
)

// AWSProviderSTSAPI is the subset of STS used to verify a provider.
type AWSProviderSTSAPI interface {
	GetCallerIdentity(ctx context.Context, params *awssts.GetCallerIdentityInput, optFns ...func(*awssts.Options)) (*awssts.GetCallerIdentityOutput, error)
}

// AWSProviderReconciler verifies that each AWSProvider's credentials work and
// records the resolved account ID in status. It never creates AWS resources.
type AWSProviderReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	STSClient AWSProviderSTSAPI
	Resolver  *provider.Resolver
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=awsproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=awsproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

func (r *AWSProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	p := &awsv1alpha1.AWSProvider{}
	if err := r.Get(ctx, req.NamespacedName, p); err != nil {
		if r.Resolver != nil {
			r.Resolver.Invalidate(req.Name)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !p.DeletionTimestamp.IsZero() {
		if r.Resolver != nil {
			r.Resolver.Invalidate(p.Name)
		}
		return ctrl.Result{}, nil
	}
	if p.Generation != p.Status.ObservedGeneration && r.Resolver != nil {
		r.Resolver.Invalidate(p.Name)
	}

	if err := r.verify(ctx, p); err != nil {
		logger.Error(err, "provider verification failed")
		meta.SetStatusCondition(&p.Status.Conditions, metav1.Condition{
			Type: awsv1alpha1.ConditionReady, Status: metav1.ConditionFalse,
			ObservedGeneration: p.Generation, Reason: awsv1alpha1.ReasonError, Message: err.Error(),
		})
		_ = persistStatus(ctx, r.Client, p)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	p.Status.ObservedGeneration = p.Generation
	now := metav1.Now()
	p.Status.LastSyncTime = &now
	meta.SetStatusCondition(&p.Status.Conditions, metav1.Condition{
		Type: awsv1alpha1.ConditionReady, Status: metav1.ConditionTrue,
		ObservedGeneration: p.Generation, Reason: awsv1alpha1.ReasonSynced, Message: "credentials verified",
	})
	if err := persistStatus(ctx, r.Client, p); err != nil {
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *AWSProviderReconciler) verify(ctx context.Context, p *awsv1alpha1.AWSProvider) error {
	if r.Resolver == nil || r.STSClient == nil {
		return nil
	}
	scope, err := r.Resolver.ForProvider(ctx, p, 0)
	if err != nil {
		return err
	}
	out, err := r.STSClient.GetCallerIdentity(provider.WithScope(ctx, scope), &awssts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("sts:GetCallerIdentity via AWSProvider %q: %w", p.Name, err)
	}
	p.Status.AccountID = aws.ToString(out.Account)
	p.Status.AssumedRoleARN = aws.ToString(out.Arn)
	return nil
}

func (r *AWSProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.AWSProvider{}).
		Complete(r)
}
