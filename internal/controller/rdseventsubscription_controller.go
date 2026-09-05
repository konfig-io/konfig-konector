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
	awsrds "github.com/aws/aws-sdk-go-v2/service/rds"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	rdshelper "github.com/konfig-io/konfig-konector/internal/aws/rds"
)

// RDSEventSubscriptionAWSAPI is the subset of the RDS API used by this controller.
type RDSEventSubscriptionAWSAPI interface {
	DescribeEventSubscriptions(ctx context.Context, params *awsrds.DescribeEventSubscriptionsInput, optFns ...func(*awsrds.Options)) (*awsrds.DescribeEventSubscriptionsOutput, error)
	CreateEventSubscription(ctx context.Context, params *awsrds.CreateEventSubscriptionInput, optFns ...func(*awsrds.Options)) (*awsrds.CreateEventSubscriptionOutput, error)
	ModifyEventSubscription(ctx context.Context, params *awsrds.ModifyEventSubscriptionInput, optFns ...func(*awsrds.Options)) (*awsrds.ModifyEventSubscriptionOutput, error)
	DeleteEventSubscription(ctx context.Context, params *awsrds.DeleteEventSubscriptionInput, optFns ...func(*awsrds.Options)) (*awsrds.DeleteEventSubscriptionOutput, error)
}

// RDSEventSubscriptionReconciler reconciles RDSEventSubscription objects.
type RDSEventSubscriptionReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	RDSClient RDSEventSubscriptionAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=rdseventsubscriptions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=rdseventsubscriptions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=rdseventsubscriptions/finalizers,verbs=update

func (r *RDSEventSubscriptionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	es := &awsv1alpha1.RDSEventSubscription{}
	if err := r.Get(ctx, req.NamespacedName, es); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, es); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !es.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(es, awsv1alpha1.FinalizerName) {
			if shouldAbandon(es) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(es, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, es)
			}
			if err := r.deleteSubscription(ctx, es); err != nil {
				logger.Error(err, "failed to delete event subscription")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(es, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, es)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(es, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(es, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, es); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileSubscription(ctx, es); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, es, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

// resolveSNSTopicARN resolves the SNS topic ref to an ARN.
func (r *RDSEventSubscriptionReconciler) resolveSNSTopicARN(ctx context.Context, namespace string, ref awsv1alpha1.SNSTopicRef) (string, error) {
	if ref.ARN != "" {
		return ref.ARN, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("snsTopicRef must specify either name or arn")
	}
	topic := &awsv1alpha1.SNSTopic{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, topic); err != nil {
		return "", err
	}
	if topic.Status.TopicARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("SNSTopic %s/%s has no ARN yet", namespace, ref.Name)}
	}
	return topic.Status.TopicARN, nil
}

func (r *RDSEventSubscriptionReconciler) reconcileSubscription(ctx context.Context, es *awsv1alpha1.RDSEventSubscription) error {
	topicARN, err := r.resolveSNSTopicARN(ctx, es.Namespace, es.Spec.SnsTopicRef)
	if err != nil {
		return err
	}

	descOut, err := r.RDSClient.DescribeEventSubscriptions(ctx, &awsrds.DescribeEventSubscriptionsInput{
		SubscriptionName: aws.String(es.Spec.SubscriptionName),
	})
	if err != nil && !rdshelper.IsNotFound(err) {
		return err
	}

	enabled := true
	if es.Spec.Enabled != nil {
		enabled = *es.Spec.Enabled
	}

	if rdshelper.IsNotFound(err) || len(descOut.EventSubscriptionsList) == 0 {
		input := &awsrds.CreateEventSubscriptionInput{
			SubscriptionName: aws.String(es.Spec.SubscriptionName),
			SnsTopicArn:      aws.String(topicARN),
			Enabled:          aws.Bool(enabled),
			EventCategories:  es.Spec.EventCategories,
			SourceIds:        es.Spec.SourceIds,
		}
		if es.Spec.SourceType != "" {
			input.SourceType = aws.String(es.Spec.SourceType)
		}
		createOut, err := r.RDSClient.CreateEventSubscription(ctx, input)
		if err != nil {
			return fmt.Errorf("create event subscription: %w", err)
		}
		if createOut.EventSubscription != nil {
			es.Status.ARN = aws.ToString(createOut.EventSubscription.EventSubscriptionArn)
		}
		if err := persistStatus(ctx, r.Client, es); err != nil {
			return fmt.Errorf("persist subscription ARN after create: %w", err)
		}
	} else {
		existing := descOut.EventSubscriptionsList[0]
		es.Status.ARN = aws.ToString(existing.EventSubscriptionArn)
		if es.Status.ObservedGeneration != es.Generation {
			input := &awsrds.ModifyEventSubscriptionInput{
				SubscriptionName: aws.String(es.Spec.SubscriptionName),
				SnsTopicArn:      aws.String(topicARN),
				Enabled:          aws.Bool(enabled),
				EventCategories:  es.Spec.EventCategories,
			}
			if es.Spec.SourceType != "" {
				input.SourceType = aws.String(es.Spec.SourceType)
			}
			if _, err := r.RDSClient.ModifyEventSubscription(ctx, input); err != nil {
				return fmt.Errorf("modify event subscription: %w", err)
			}
		}
	}

	es.Status.ObservedGeneration = es.Generation
	now := metav1.Now()
	es.Status.LastSyncTime = &now
	return r.setCondition(ctx, es, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "event subscription reconciled")
}

func (r *RDSEventSubscriptionReconciler) deleteSubscription(ctx context.Context, es *awsv1alpha1.RDSEventSubscription) error {
	// The subscription is identified by its spec name.
	_, err := r.RDSClient.DeleteEventSubscription(ctx, &awsrds.DeleteEventSubscriptionInput{
		SubscriptionName: aws.String(es.Spec.SubscriptionName),
	})
	if rdshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *RDSEventSubscriptionReconciler) setCondition(ctx context.Context, es *awsv1alpha1.RDSEventSubscription, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&es.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: es.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, es); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *RDSEventSubscriptionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.RDSEventSubscription{}).
		Complete(r)
}
