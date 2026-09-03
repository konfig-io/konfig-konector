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
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	sqshelper "github.com/konfig-io/konfig-konector/internal/aws/sqs"
)

// SQSQueueAWSAPI is the subset of the SQS API used by this controller.
type SQSQueueAWSAPI interface {
	GetQueueUrl(ctx context.Context, params *awssqs.GetQueueUrlInput, optFns ...func(*awssqs.Options)) (*awssqs.GetQueueUrlOutput, error)
	CreateQueue(ctx context.Context, params *awssqs.CreateQueueInput, optFns ...func(*awssqs.Options)) (*awssqs.CreateQueueOutput, error)
	SetQueueAttributes(ctx context.Context, params *awssqs.SetQueueAttributesInput, optFns ...func(*awssqs.Options)) (*awssqs.SetQueueAttributesOutput, error)
	TagQueue(ctx context.Context, params *awssqs.TagQueueInput, optFns ...func(*awssqs.Options)) (*awssqs.TagQueueOutput, error)
	GetQueueAttributes(ctx context.Context, params *awssqs.GetQueueAttributesInput, optFns ...func(*awssqs.Options)) (*awssqs.GetQueueAttributesOutput, error)
	DeleteQueue(ctx context.Context, params *awssqs.DeleteQueueInput, optFns ...func(*awssqs.Options)) (*awssqs.DeleteQueueOutput, error)
}

// SQSQueueReconciler reconciles SQSQueue objects.
type SQSQueueReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	SQSClient SQSQueueAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=sqsqueues,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=sqsqueues/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=sqsqueues/finalizers,verbs=update

func (r *SQSQueueReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	q := &awsv1alpha1.SQSQueue{}
	if err := r.Get(ctx, req.NamespacedName, q); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !q.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(q, awsv1alpha1.FinalizerName) {
			if shouldAbandon(q) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(q, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, q)
			}
			if err := r.deleteQueue(ctx, q); err != nil {
				logger.Error(err, "failed to delete SQS queue")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(q, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, q)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(q, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(q, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, q); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileQueue(ctx, q); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, q, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *SQSQueueReconciler) reconcileQueue(ctx context.Context, q *awsv1alpha1.SQSQueue) error {
	// Try to get existing queue URL.
	getOut, err := r.SQSClient.GetQueueUrl(ctx, &awssqs.GetQueueUrlInput{
		QueueName: aws.String(q.Spec.QueueName),
	})

	// Resolve the redrive policy up front so a missing/not-ready DLQ is
	// reported before any AWS mutation.
	var redrive string
	if q.Spec.RedrivePolicy != nil {
		rp, rpErr := r.resolveRedrivePolicy(ctx, q.Namespace, q.Spec.RedrivePolicy)
		if rpErr != nil {
			return rpErr
		}
		redrive = rp
	}

	var queueURL string
	if sqshelper.IsNotFound(err) {
		// Create the queue.
		attrs := r.buildAttributes(q, redrive)
		createInput := &awssqs.CreateQueueInput{
			QueueName:  aws.String(q.Spec.QueueName),
			Attributes: attrs,
			Tags:       q.Spec.Tags,
		}
		createOut, err := r.SQSClient.CreateQueue(ctx, createInput)
		if err != nil {
			return fmt.Errorf("create SQS queue: %w", err)
		}
		queueURL = aws.ToString(createOut.QueueUrl)
		// Persist the queue URL immediately: the AWS resource now exists, and
		// losing the identifier would orphan it on delete.
		q.Status.QueueURL = queueURL
		if err := persistStatus(ctx, r.Client, q); err != nil {
			return fmt.Errorf("persist queue URL after create: %w", err)
		}
	} else if err != nil {
		return err
	} else {
		queueURL = aws.ToString(getOut.QueueUrl)

		// Update attributes.
		attrs := r.buildAttributes(q, redrive)
		if _, err := r.SQSClient.SetQueueAttributes(ctx, &awssqs.SetQueueAttributesInput{
			QueueUrl:   aws.String(queueURL),
			Attributes: attrs,
		}); err != nil {
			return fmt.Errorf("set queue attributes: %w", err)
		}

		// Sync tags.
		if len(q.Spec.Tags) > 0 {
			if _, err := r.SQSClient.TagQueue(ctx, &awssqs.TagQueueInput{
				QueueUrl: aws.String(queueURL),
				Tags:     q.Spec.Tags,
			}); err != nil {
				return fmt.Errorf("tag queue: %w", err)
			}
		}
	}

	// Get ARN to store in status.
	attrsOut, err := r.SQSClient.GetQueueAttributes(ctx, &awssqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameQueueArn},
	})
	if err != nil {
		return err
	}

	q.Status.QueueURL = queueURL
	q.Status.QueueARN = attrsOut.Attributes[string(sqstypes.QueueAttributeNameQueueArn)]
	q.Status.ObservedGeneration = q.Generation
	now := metav1.Now()
	q.Status.LastSyncTime = &now
	return r.setCondition(ctx, q, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "SQS queue reconciled")
}

func (r *SQSQueueReconciler) buildAttributes(q *awsv1alpha1.SQSQueue, redrivePolicy string) map[string]string {
	attrs := make(map[string]string)

	if q.Spec.VisibilityTimeout > 0 {
		attrs[string(sqstypes.QueueAttributeNameVisibilityTimeout)] = strconv.Itoa(int(q.Spec.VisibilityTimeout))
	}
	if q.Spec.MessageRetentionPeriod > 0 {
		attrs[string(sqstypes.QueueAttributeNameMessageRetentionPeriod)] = strconv.Itoa(int(q.Spec.MessageRetentionPeriod))
	}
	if q.Spec.DelaySeconds > 0 {
		attrs[string(sqstypes.QueueAttributeNameDelaySeconds)] = strconv.Itoa(int(q.Spec.DelaySeconds))
	}
	if q.Spec.ReceiveMessageWaitTime > 0 {
		attrs[string(sqstypes.QueueAttributeNameReceiveMessageWaitTimeSeconds)] = strconv.Itoa(int(q.Spec.ReceiveMessageWaitTime))
	}
	if q.Spec.KMSKeyID != "" {
		attrs["KmsMasterKeyId"] = q.Spec.KMSKeyID
	}
	if q.Spec.Policy != "" {
		attrs[string(sqstypes.QueueAttributeNamePolicy)] = q.Spec.Policy
	}
	if q.Spec.FIFO {
		attrs[string(sqstypes.QueueAttributeNameFifoQueue)] = "true"
	}

	if redrivePolicy != "" {
		attrs[string(sqstypes.QueueAttributeNameRedrivePolicy)] = redrivePolicy
	}

	return attrs
}

func (r *SQSQueueReconciler) resolveRedrivePolicy(ctx context.Context, namespace string, rp *awsv1alpha1.SQSRedrivePolicy) (string, error) {
	dlqCR := &awsv1alpha1.SQSQueue{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: rp.DeadLetterQueueRef, Namespace: namespace}, dlqCR); err != nil {
		return "", err
	}
	if dlqCR.Status.QueueARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("SQSQueue %s/%s has no ARN yet", namespace, rp.DeadLetterQueueRef)}
	}
	policy := map[string]interface{}{
		"deadLetterTargetArn": dlqCR.Status.QueueARN,
		"maxReceiveCount":     rp.MaxReceiveCount,
	}
	b, err := json.Marshal(policy)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (r *SQSQueueReconciler) deleteQueue(ctx context.Context, q *awsv1alpha1.SQSQueue) error {
	queueURL := q.Status.QueueURL
	if queueURL == "" {
		// Status may have been lost before it was persisted; the queue URL is
		// deterministically derivable from the spec queue name.
		getOut, err := r.SQSClient.GetQueueUrl(ctx, &awssqs.GetQueueUrlInput{
			QueueName: aws.String(q.Spec.QueueName),
		})
		if sqshelper.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		queueURL = aws.ToString(getOut.QueueUrl)
	}
	_, err := r.SQSClient.DeleteQueue(ctx, &awssqs.DeleteQueueInput{
		QueueUrl: aws.String(queueURL),
	})
	if sqshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SQSQueueReconciler) setCondition(ctx context.Context, q *awsv1alpha1.SQSQueue, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&q.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: q.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, q); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *SQSQueueReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.SQSQueue{}).
		Complete(r)
}
