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
	awssns "github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
	snshelper "github.com/konfig-io/konfig-konector/internal/aws/sns"
)

// SNSSubscriptionReconciler reconciles SNSSubscription objects.
type SNSSubscriptionReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	SNSClient *multi.SNS
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=snssubscriptions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=snssubscriptions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=snssubscriptions/finalizers,verbs=update

func (r *SNSSubscriptionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	sub := &awsv1alpha1.SNSSubscription{}
	if err := r.Get(ctx, req.NamespacedName, sub); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, sub); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !sub.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(sub, awsv1alpha1.FinalizerName) {
			if shouldAbandon(sub) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(sub, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, sub)
			}
			if err := r.deleteSubscription(ctx, sub); err != nil {
				logger.Error(err, "failed to delete SNS subscription")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(sub, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, sub)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(sub, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(sub, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, sub); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileSubscription(ctx, sub); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, sub, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *SNSSubscriptionReconciler) reconcileSubscription(ctx context.Context, sub *awsv1alpha1.SNSSubscription) error {
	topicARN, err := r.resolveTopicARN(ctx, sub.Namespace, sub.Spec.TopicRef)
	if err != nil {
		return err
	}

	// If we already have a subscription ARN, update attributes.
	if sub.Status.SubscriptionARN != "" && sub.Status.SubscriptionARN != "PendingConfirmation" {
		if err := r.syncAttributes(ctx, sub); err != nil {
			return err
		}
		sub.Status.ObservedGeneration = sub.Generation
		now := metav1.Now()
		sub.Status.LastSyncTime = &now
		return r.setCondition(ctx, sub, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "SNS subscription reconciled")
	}

	// Create new subscription.
	subOut, err := r.SNSClient.Subscribe(ctx, &awssns.SubscribeInput{
		TopicArn: aws.String(topicARN),
		Protocol: aws.String(sub.Spec.Protocol),
		Endpoint: aws.String(sub.Spec.Endpoint),
		Attributes: map[string]string{
			"RawMessageDelivery": boolToStr(sub.Spec.RawMessageDelivery),
		},
		ReturnSubscriptionArn: true,
	})
	if err != nil {
		return fmt.Errorf("subscribe to SNS topic: %w", err)
	}

	sub.Status.SubscriptionARN = aws.ToString(subOut.SubscriptionArn)
	// Persist the ARN immediately: the AWS resource now exists, and losing the
	// identifier would orphan it or create a duplicate on retry.
	if err := persistStatus(ctx, r.Client, sub); err != nil {
		return fmt.Errorf("persist subscription ARN after create: %w", err)
	}
	sub.Status.ObservedGeneration = sub.Generation
	now := metav1.Now()
	sub.Status.LastSyncTime = &now
	return r.setCondition(ctx, sub, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "SNS subscription created")
}

func (r *SNSSubscriptionReconciler) syncAttributes(ctx context.Context, sub *awsv1alpha1.SNSSubscription) error {
	attrs := map[string]string{
		"RawMessageDelivery": boolToStr(sub.Spec.RawMessageDelivery),
	}
	if sub.Spec.FilterPolicy != "" {
		attrs["FilterPolicy"] = sub.Spec.FilterPolicy
	}
	for k, v := range attrs {
		k, v := k, v
		if _, err := r.SNSClient.SetSubscriptionAttributes(ctx, &awssns.SetSubscriptionAttributesInput{
			SubscriptionArn: aws.String(sub.Status.SubscriptionARN),
			AttributeName:   aws.String(k),
			AttributeValue:  aws.String(v),
		}); err != nil {
			return fmt.Errorf("set subscription attribute %s: %w", k, err)
		}
	}
	return nil
}

func (r *SNSSubscriptionReconciler) resolveTopicARN(ctx context.Context, namespace string, ref awsv1alpha1.SNSTopicRef) (string, error) {
	if ref.ARN != "" {
		return ref.ARN, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("topicRef must specify either name or arn")
	}
	topicCR := &awsv1alpha1.SNSTopic{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, topicCR); err != nil {
		return "", err
	}
	if topicCR.Status.TopicARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("SNSTopic %s/%s has no ARN yet", namespace, ref.Name)}
	}
	return topicCR.Status.TopicARN, nil
}

func (r *SNSSubscriptionReconciler) deleteSubscription(ctx context.Context, sub *awsv1alpha1.SNSSubscription) error {
	if sub.Status.SubscriptionARN == "" || sub.Status.SubscriptionARN == "PendingConfirmation" {
		return nil
	}
	_, err := r.SNSClient.Unsubscribe(ctx, &awssns.UnsubscribeInput{
		SubscriptionArn: aws.String(sub.Status.SubscriptionARN),
	})
	if snshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SNSSubscriptionReconciler) setCondition(ctx context.Context, sub *awsv1alpha1.SNSSubscription, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&sub.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: sub.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, sub); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

// snsTagsFromMap converts a map to SNS Tag slice.
func snsTagsFromMap(m map[string]string) []snstypes.Tag {
	tags := make([]snstypes.Tag, 0, len(m))
	for k, v := range m {
		k, v := k, v
		tags = append(tags, snstypes.Tag{Key: &k, Value: &v})
	}
	return tags
}

func boolToStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func (r *SNSSubscriptionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.SNSSubscription{}).
		Complete(r)
}
