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
	awssecurityhub "github.com/aws/aws-sdk-go-v2/service/securityhub"
	securityhubtypes "github.com/aws/aws-sdk-go-v2/service/securityhub/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	securityhubhelper "github.com/konfig-io/konfig-konector/internal/aws/securityhub"
)

// SecurityHubStandardAWSAPI is the subset of the Security Hub API used by this controller.
type SecurityHubStandardAWSAPI interface {
	GetEnabledStandards(ctx context.Context, params *awssecurityhub.GetEnabledStandardsInput, optFns ...func(*awssecurityhub.Options)) (*awssecurityhub.GetEnabledStandardsOutput, error)
	BatchEnableStandards(ctx context.Context, params *awssecurityhub.BatchEnableStandardsInput, optFns ...func(*awssecurityhub.Options)) (*awssecurityhub.BatchEnableStandardsOutput, error)
	BatchDisableStandards(ctx context.Context, params *awssecurityhub.BatchDisableStandardsInput, optFns ...func(*awssecurityhub.Options)) (*awssecurityhub.BatchDisableStandardsOutput, error)
}

// SecurityHubStandardReconciler reconciles SecurityHubStandard objects.
type SecurityHubStandardReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	SecurityHubClient SecurityHubStandardAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=securityhubstandards,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=securityhubstandards/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=securityhubstandards/finalizers,verbs=update

func (r *SecurityHubStandardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	std := &awsv1alpha1.SecurityHubStandard{}
	if err := r.Get(ctx, req.NamespacedName, std); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, std); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !std.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(std, awsv1alpha1.FinalizerName) {
			if shouldAbandon(std) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(std, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, std)
			}
			if err := r.disableStandard(ctx, std); err != nil {
				logger.Error(err, "failed to disable standard")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(std, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, std)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(std, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(std, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, std); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileStandard(ctx, std); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, std, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *SecurityHubStandardReconciler) reconcileStandard(ctx context.Context, std *awsv1alpha1.SecurityHubStandard) error {
	sub, err := r.findSubscription(ctx, std.Spec.StandardsARN)
	if err != nil {
		return err
	}

	if sub == nil {
		enableOut, err := r.SecurityHubClient.BatchEnableStandards(ctx, &awssecurityhub.BatchEnableStandardsInput{
			StandardsSubscriptionRequests: []securityhubtypes.StandardsSubscriptionRequest{
				{StandardsArn: aws.String(std.Spec.StandardsARN)},
			},
		})
		if err != nil {
			return fmt.Errorf("enable standard: %w", err)
		}
		if len(enableOut.StandardsSubscriptions) > 0 {
			sub = &enableOut.StandardsSubscriptions[0]
		}
		if sub != nil {
			// Persist the subscription ARN immediately after enable.
			std.Status.SubscriptionARN = aws.ToString(sub.StandardsSubscriptionArn)
			if err := persistStatus(ctx, r.Client, std); err != nil {
				return fmt.Errorf("persist subscription ARN after enable: %w", err)
			}
		}
	}

	if sub != nil {
		std.Status.SubscriptionARN = aws.ToString(sub.StandardsSubscriptionArn)
		std.Status.StandardsStatus = string(sub.StandardsStatus)
	}
	std.Status.ObservedGeneration = std.Generation
	now := metav1.Now()
	std.Status.LastSyncTime = &now
	return r.setCondition(ctx, std, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "standards subscription reconciled")
}

// findSubscription returns the enabled subscription for a standards ARN, or nil.
func (r *SecurityHubStandardReconciler) findSubscription(ctx context.Context, standardsARN string) (*securityhubtypes.StandardsSubscription, error) {
	p := awssecurityhub.NewGetEnabledStandardsPaginator(r.SecurityHubClient, &awssecurityhub.GetEnabledStandardsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			if securityhubhelper.IsNotFound(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("get enabled standards: %w", err)
		}
		for i := range page.StandardsSubscriptions {
			if aws.ToString(page.StandardsSubscriptions[i].StandardsArn) == standardsARN {
				return &page.StandardsSubscriptions[i], nil
			}
		}
	}
	return nil, nil
}

func (r *SecurityHubStandardReconciler) disableStandard(ctx context.Context, std *awsv1alpha1.SecurityHubStandard) error {
	subARN := std.Status.SubscriptionARN
	if subARN == "" {
		// The subscription is deterministically derivable from the standards ARN.
		sub, err := r.findSubscription(ctx, std.Spec.StandardsARN)
		if err != nil {
			return err
		}
		if sub == nil {
			return nil
		}
		subARN = aws.ToString(sub.StandardsSubscriptionArn)
	}
	_, err := r.SecurityHubClient.BatchDisableStandards(ctx, &awssecurityhub.BatchDisableStandardsInput{
		StandardsSubscriptionArns: []string{subARN},
	})
	if securityhubhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SecurityHubStandardReconciler) setCondition(ctx context.Context, std *awsv1alpha1.SecurityHubStandard, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&std.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: std.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, std); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *SecurityHubStandardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.SecurityHubStandard{}).
		Complete(r)
}
