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

	"github.com/aws/aws-sdk-go-v2/aws"
	awssns "github.com/aws/aws-sdk-go-v2/service/sns"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	snshelper "github.com/konfig-io/konfig-konector/internal/aws/sns"
)

// SNSTopicAWSAPI is the subset of the SNS API used by this controller.
type SNSTopicAWSAPI interface {
	CreateTopic(ctx context.Context, params *awssns.CreateTopicInput, optFns ...func(*awssns.Options)) (*awssns.CreateTopicOutput, error)
	SetTopicAttributes(ctx context.Context, params *awssns.SetTopicAttributesInput, optFns ...func(*awssns.Options)) (*awssns.SetTopicAttributesOutput, error)
	TagResource(ctx context.Context, params *awssns.TagResourceInput, optFns ...func(*awssns.Options)) (*awssns.TagResourceOutput, error)
	DeleteTopic(ctx context.Context, params *awssns.DeleteTopicInput, optFns ...func(*awssns.Options)) (*awssns.DeleteTopicOutput, error)
	ListTopics(ctx context.Context, params *awssns.ListTopicsInput, optFns ...func(*awssns.Options)) (*awssns.ListTopicsOutput, error)
}

// SNSTopicReconciler reconciles SNSTopic objects.
type SNSTopicReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	SNSClient SNSTopicAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=snstopics,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=snstopics/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=snstopics/finalizers,verbs=update

func (r *SNSTopicReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	t := &awsv1alpha1.SNSTopic{}
	if err := r.Get(ctx, req.NamespacedName, t); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !t.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(t, awsv1alpha1.FinalizerName) {
			if shouldAbandon(t) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(t, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, t)
			}
			if err := r.deleteTopic(ctx, t); err != nil {
				logger.Error(err, "failed to delete SNS topic")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(t, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, t)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(t, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(t, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, t); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileTopic(ctx, t); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, t, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *SNSTopicReconciler) reconcileTopic(ctx context.Context, t *awsv1alpha1.SNSTopic) error {
	// CreateTopic is idempotent — returns the ARN if already exists.
	attrs := make(map[string]string)
	if t.Spec.FIFO {
		attrs["FifoTopic"] = "true"
	}
	if t.Spec.ContentBasedDeduplication {
		attrs["ContentBasedDeduplication"] = "true"
	}
	if t.Spec.KMSKeyID != "" {
		attrs["KmsMasterKeyId"] = t.Spec.KMSKeyID
	}
	if t.Spec.Policy != "" {
		attrs["Policy"] = t.Spec.Policy
	}

	createOut, err := r.SNSClient.CreateTopic(ctx, &awssns.CreateTopicInput{
		Name:       aws.String(t.Spec.TopicName),
		Attributes: attrs,
		Tags:       snsTagsFromMap(t.Spec.Tags),
	})
	if err != nil {
		return fmt.Errorf("create SNS topic: %w", err)
	}

	topicARN := aws.ToString(createOut.TopicArn)
	hadARN := t.Status.TopicARN != ""

	// Persist the ARN immediately: the AWS resource now exists, and later
	// attribute/tag calls can fail, which would leave the identifier unset.
	if t.Status.TopicARN != topicARN {
		t.Status.TopicARN = topicARN
		if err := persistStatus(ctx, r.Client, t); err != nil {
			return fmt.Errorf("persist topic ARN after create: %w", err)
		}
	}

	// Update attributes on subsequent reconciles (CreateTopic doesn't update attrs on existing topics).
	if hadARN && len(attrs) > 0 {
		for k, v := range attrs {
			k, v := k, v
			if _, err := r.SNSClient.SetTopicAttributes(ctx, &awssns.SetTopicAttributesInput{
				TopicArn:       aws.String(topicARN),
				AttributeName:  aws.String(k),
				AttributeValue: aws.String(v),
			}); err != nil {
				return fmt.Errorf("set topic attribute %s: %w", k, err)
			}
		}
	}

	// Sync tags.
	if len(t.Spec.Tags) > 0 {
		if _, err := r.SNSClient.TagResource(ctx, &awssns.TagResourceInput{
			ResourceArn: aws.String(topicARN),
			Tags:        snsTagsFromMap(t.Spec.Tags),
		}); err != nil {
			return fmt.Errorf("tag SNS topic: %w", err)
		}
	}

	t.Status.TopicARN = topicARN
	t.Status.ObservedGeneration = t.Generation
	now := metav1.Now()
	t.Status.LastSyncTime = &now
	return r.setCondition(ctx, t, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "SNS topic reconciled")
}

func (r *SNSTopicReconciler) deleteTopic(ctx context.Context, t *awsv1alpha1.SNSTopic) error {
	arn := t.Status.TopicARN
	if arn == "" {
		// Status may have been lost before it was persisted; fall back to
		// finding the topic by exact name suffix match.
		found, err := r.findTopicARNByName(ctx, t.Spec.TopicName)
		if err != nil {
			return err
		}
		if found == "" {
			return nil
		}
		arn = found
	}
	_, err := r.SNSClient.DeleteTopic(ctx, &awssns.DeleteTopicInput{
		TopicArn: aws.String(arn),
	})
	if snshelper.IsNotFound(err) {
		return nil
	}
	return err
}

// findTopicARNByName lists SNS topics and returns the ARN whose final
// component matches the topic name exactly, or "" if none matches.
func (r *SNSTopicReconciler) findTopicARNByName(ctx context.Context, topicName string) (string, error) {
	if topicName == "" {
		return "", nil
	}
	suffix := ":" + topicName
	var next *string
	for {
		out, err := r.SNSClient.ListTopics(ctx, &awssns.ListTopicsInput{NextToken: next})
		if err != nil {
			return "", err
		}
		for _, topic := range out.Topics {
			arn := aws.ToString(topic.TopicArn)
			if strings.HasSuffix(arn, suffix) {
				return arn, nil
			}
		}
		if out.NextToken == nil {
			return "", nil
		}
		next = out.NextToken
	}
}

func (r *SNSTopicReconciler) setCondition(ctx context.Context, t *awsv1alpha1.SNSTopic, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&t.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: t.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, t); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *SNSTopicReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.SNSTopic{}).
		Complete(r)
}
