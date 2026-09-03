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

	"github.com/aws/aws-sdk-go-v2/aws"
	awsfirehose "github.com/aws/aws-sdk-go-v2/service/firehose"
	firehosetypes "github.com/aws/aws-sdk-go-v2/service/firehose/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	firehosehelper "github.com/konfig-io/konfig-konector/internal/aws/firehose"
)

// FirehoseDeliveryStreamReconciler reconciles FirehoseDeliveryStream objects.
type FirehoseDeliveryStreamReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	FirehoseClient *awsfirehose.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=firehosedeliverystreams,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=firehosedeliverystreams/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=firehosedeliverystreams/finalizers,verbs=update

func (r *FirehoseDeliveryStreamReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.FirehoseDeliveryStream{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
			if shouldAbandon(obj) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, obj)
			}
			if err := r.deleteDeliveryStream(ctx, obj); err != nil {
				logger.Error(err, "failed to delete FirehoseDeliveryStream")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, obj)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(obj, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileDeliveryStream(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionFDS(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *FirehoseDeliveryStreamReconciler) reconcileDeliveryStream(ctx context.Context, obj *awsv1alpha1.FirehoseDeliveryStream) error {
	descOut, err := r.FirehoseClient.DescribeDeliveryStream(ctx, &awsfirehose.DescribeDeliveryStreamInput{
		DeliveryStreamName: aws.String(obj.Spec.DeliveryStreamName),
	})
	if err != nil && !firehosehelper.IsNotFound(err) {
		return fmt.Errorf("describe firehose delivery stream: %w", err)
	}

	if err == nil && descOut.DeliveryStreamDescription != nil {
		desc := descOut.DeliveryStreamDescription
		obj.Status.DeliveryStreamARN = aws.ToString(desc.DeliveryStreamARN)
		obj.Status.DeliveryStreamStatus = string(desc.DeliveryStreamStatus)

		if desc.DeliveryStreamStatus != firehosetypes.DeliveryStreamStatusActive {
			return r.setConditionFDS(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonCreated, fmt.Sprintf("delivery stream status: %s", desc.DeliveryStreamStatus))
		}

		obj.Status.ObservedGeneration = obj.Generation
		now := metav1.Now()
		obj.Status.LastSyncTime = &now
		return r.setConditionFDS(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "FirehoseDeliveryStream reconciled")
	}

	input := &awsfirehose.CreateDeliveryStreamInput{
		DeliveryStreamName: aws.String(obj.Spec.DeliveryStreamName),
	}
	if obj.Spec.DeliveryStreamType != "" {
		input.DeliveryStreamType = firehosetypes.DeliveryStreamType(obj.Spec.DeliveryStreamType)
	}
	if s3 := obj.Spec.S3DestinationConfiguration; s3 != nil {
		s3cfg := &firehosetypes.S3DestinationConfiguration{
			BucketARN: aws.String(s3.BucketARN),
			RoleARN:   aws.String(s3.RoleARN),
		}
		if s3.Prefix != "" {
			s3cfg.Prefix = aws.String(s3.Prefix)
		}
		if s3.BufferingIntervalSeconds != nil || s3.BufferingSizeMBs != nil {
			s3cfg.BufferingHints = &firehosetypes.BufferingHints{
				IntervalInSeconds: s3.BufferingIntervalSeconds,
				SizeInMBs:         s3.BufferingSizeMBs,
			}
		}
		input.S3DestinationConfiguration = s3cfg
	}
	if len(obj.Spec.Tags) > 0 {
		tags := make([]firehosetypes.Tag, 0, len(obj.Spec.Tags))
		for k, v := range obj.Spec.Tags {
			k, v := k, v
			tags = append(tags, firehosetypes.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.FirehoseClient.CreateDeliveryStream(ctx, input)
	if err != nil {
		return fmt.Errorf("create firehose delivery stream: %w", err)
	}

	obj.Status.DeliveryStreamARN = aws.ToString(out.DeliveryStreamARN)
	obj.Status.DeliveryStreamStatus = "CREATING"
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionFDS(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonCreated, "FirehoseDeliveryStream creating")
}

func (r *FirehoseDeliveryStreamReconciler) deleteDeliveryStream(ctx context.Context, obj *awsv1alpha1.FirehoseDeliveryStream) error {
	_, err := r.FirehoseClient.DeleteDeliveryStream(ctx, &awsfirehose.DeleteDeliveryStreamInput{
		DeliveryStreamName: aws.String(obj.Spec.DeliveryStreamName),
	})
	if firehosehelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *FirehoseDeliveryStreamReconciler) setConditionFDS(ctx context.Context, obj *awsv1alpha1.FirehoseDeliveryStream, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: obj.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *FirehoseDeliveryStreamReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.FirehoseDeliveryStream{}).
		Complete(r)
}
