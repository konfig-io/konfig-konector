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
	awsce "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	cehelper "github.com/konfig-io/konfig-konector/internal/aws/costexplorer"
)

// CostAnomalySubscriptionAWSAPI is the subset of the Cost Explorer API used by this controller.
type CostAnomalySubscriptionAWSAPI interface {
	GetAnomalySubscriptions(ctx context.Context, params *awsce.GetAnomalySubscriptionsInput, optFns ...func(*awsce.Options)) (*awsce.GetAnomalySubscriptionsOutput, error)
	CreateAnomalySubscription(ctx context.Context, params *awsce.CreateAnomalySubscriptionInput, optFns ...func(*awsce.Options)) (*awsce.CreateAnomalySubscriptionOutput, error)
	UpdateAnomalySubscription(ctx context.Context, params *awsce.UpdateAnomalySubscriptionInput, optFns ...func(*awsce.Options)) (*awsce.UpdateAnomalySubscriptionOutput, error)
	DeleteAnomalySubscription(ctx context.Context, params *awsce.DeleteAnomalySubscriptionInput, optFns ...func(*awsce.Options)) (*awsce.DeleteAnomalySubscriptionOutput, error)
}

// CostAnomalySubscriptionReconciler reconciles CostAnomalySubscription objects.
type CostAnomalySubscriptionReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	CostExplorerClient CostAnomalySubscriptionAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=costanomalysubscriptions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=costanomalysubscriptions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=costanomalysubscriptions/finalizers,verbs=update

func (r *CostAnomalySubscriptionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	s := &awsv1alpha1.CostAnomalySubscription{}
	if err := r.Get(ctx, req.NamespacedName, s); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, s); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !s.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(s, awsv1alpha1.FinalizerName) {
			if shouldAbandon(s) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(s, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, s)
			}
			if err := r.deleteSubscription(ctx, s); err != nil {
				logger.Error(err, "failed to delete cost anomaly subscription")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(s, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, s)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(s, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(s, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, s); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileSubscription(ctx, s); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, s, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *CostAnomalySubscriptionReconciler) resolveMonitorARNs(ctx context.Context, s *awsv1alpha1.CostAnomalySubscription) ([]string, error) {
	arns := make([]string, 0, len(s.Spec.MonitorRefs))
	for _, ref := range s.Spec.MonitorRefs {
		if ref.ARN != "" {
			arns = append(arns, ref.ARN)
			continue
		}
		if ref.Name == "" {
			return nil, fmt.Errorf("monitorRef requires either name or arn")
		}
		m := &awsv1alpha1.CostAnomalyMonitor{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: s.Namespace}, m); err != nil {
			return nil, err
		}
		if m.Status.ARN == "" {
			return nil, &dependencyNotReady{msg: fmt.Sprintf("CostAnomalyMonitor %s/%s has no ARN yet", s.Namespace, ref.Name)}
		}
		arns = append(arns, m.Status.ARN)
	}
	return arns, nil
}

func (r *CostAnomalySubscriptionReconciler) subscribers(s *awsv1alpha1.CostAnomalySubscription) []cetypes.Subscriber {
	subs := make([]cetypes.Subscriber, 0, len(s.Spec.Subscribers))
	for _, sub := range s.Spec.Subscribers {
		subs = append(subs, cetypes.Subscriber{
			Address: aws.String(sub.Address),
			Type:    cetypes.SubscriberType(sub.Type),
		})
	}
	return subs
}

// thresholdExpression builds the absolute-impact threshold expression the
// anomaly subscription API expects.
func thresholdExpression(threshold string) *cetypes.Expression {
	return &cetypes.Expression{
		Dimensions: &cetypes.DimensionValues{
			Key:          cetypes.DimensionAnomalyTotalImpactAbsolute,
			MatchOptions: []cetypes.MatchOption{cetypes.MatchOptionGreaterThanOrEqual},
			Values:       []string{threshold},
		},
	}
}

// findSubscriptionByName scans all anomaly subscriptions for one with the spec name.
func (r *CostAnomalySubscriptionReconciler) findSubscriptionByName(ctx context.Context, name string) (*cetypes.AnomalySubscription, error) {
	var next *string
	for {
		out, err := r.CostExplorerClient.GetAnomalySubscriptions(ctx, &awsce.GetAnomalySubscriptionsInput{NextPageToken: next})
		if err != nil {
			return nil, err
		}
		for i := range out.AnomalySubscriptions {
			if aws.ToString(out.AnomalySubscriptions[i].SubscriptionName) == name {
				return &out.AnomalySubscriptions[i], nil
			}
		}
		if out.NextPageToken == nil {
			return nil, nil
		}
		next = out.NextPageToken
	}
}

func (r *CostAnomalySubscriptionReconciler) reconcileSubscription(ctx context.Context, s *awsv1alpha1.CostAnomalySubscription) error {
	monitorARNs, err := r.resolveMonitorARNs(ctx, s)
	if err != nil {
		return err
	}

	var existing *cetypes.AnomalySubscription
	if s.Status.ARN != "" {
		out, err := r.CostExplorerClient.GetAnomalySubscriptions(ctx, &awsce.GetAnomalySubscriptionsInput{
			SubscriptionArnList: []string{s.Status.ARN},
		})
		if err != nil && !cehelper.IsNotFound(err) {
			return err
		}
		if err == nil && len(out.AnomalySubscriptions) > 0 {
			existing = &out.AnomalySubscriptions[0]
		}
	} else {
		found, err := r.findSubscriptionByName(ctx, s.Spec.SubscriptionName)
		if err != nil {
			return err
		}
		existing = found
	}

	if existing == nil {
		created, err := r.CostExplorerClient.CreateAnomalySubscription(ctx, &awsce.CreateAnomalySubscriptionInput{
			AnomalySubscription: &cetypes.AnomalySubscription{
				SubscriptionName:    aws.String(s.Spec.SubscriptionName),
				MonitorArnList:      monitorARNs,
				Frequency:           cetypes.AnomalySubscriptionFrequency(s.Spec.Frequency),
				Subscribers:         r.subscribers(s),
				ThresholdExpression: thresholdExpression(s.Spec.Threshold),
			},
		})
		if err != nil {
			return fmt.Errorf("create cost anomaly subscription: %w", err)
		}
		s.Status.ARN = aws.ToString(created.SubscriptionArn)
		// Persist the ARN immediately: the AWS resource now exists, and losing
		// the identifier would orphan it on delete.
		if err := persistStatus(ctx, r.Client, s); err != nil {
			return fmt.Errorf("persist subscription ARN after create: %w", err)
		}
	} else {
		s.Status.ARN = aws.ToString(existing.SubscriptionArn)
		if s.Status.ObservedGeneration != s.Generation {
			if _, err := r.CostExplorerClient.UpdateAnomalySubscription(ctx, &awsce.UpdateAnomalySubscriptionInput{
				SubscriptionArn:     existing.SubscriptionArn,
				SubscriptionName:    aws.String(s.Spec.SubscriptionName),
				MonitorArnList:      monitorARNs,
				Frequency:           cetypes.AnomalySubscriptionFrequency(s.Spec.Frequency),
				Subscribers:         r.subscribers(s),
				ThresholdExpression: thresholdExpression(s.Spec.Threshold),
			}); err != nil {
				return fmt.Errorf("update cost anomaly subscription: %w", err)
			}
		}
	}

	s.Status.ObservedGeneration = s.Generation
	now := metav1.Now()
	s.Status.LastSyncTime = &now
	return r.setCondition(ctx, s, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "cost anomaly subscription reconciled")
}

func (r *CostAnomalySubscriptionReconciler) deleteSubscription(ctx context.Context, s *awsv1alpha1.CostAnomalySubscription) error {
	arn := s.Status.ARN
	if arn == "" {
		// Status may have been lost before it was persisted; fall back to a
		// deterministic name lookup.
		found, err := r.findSubscriptionByName(ctx, s.Spec.SubscriptionName)
		if err != nil {
			return err
		}
		if found == nil {
			return nil
		}
		arn = aws.ToString(found.SubscriptionArn)
	}
	_, err := r.CostExplorerClient.DeleteAnomalySubscription(ctx, &awsce.DeleteAnomalySubscriptionInput{
		SubscriptionArn: aws.String(arn),
	})
	if cehelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *CostAnomalySubscriptionReconciler) setCondition(ctx context.Context, s *awsv1alpha1.CostAnomalySubscription, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: s.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, s); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *CostAnomalySubscriptionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CostAnomalySubscription{}).
		Complete(r)
}
