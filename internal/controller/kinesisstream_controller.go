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
	awskinesis "github.com/aws/aws-sdk-go-v2/service/kinesis"
	kinesistypes "github.com/aws/aws-sdk-go-v2/service/kinesis/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	kinesishelper "github.com/konfig-io/konfig-konector/internal/aws/kinesis"
)

// KinesisStreamReconciler reconciles KinesisStream objects.
type KinesisStreamReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	KinesisClient *awskinesis.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=kinesisstreams,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=kinesisstreams/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=kinesisstreams/finalizers,verbs=update

func (r *KinesisStreamReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.KinesisStream{}
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
			if err := r.deleteStream(ctx, obj); err != nil {
				logger.Error(err, "failed to delete KinesisStream")
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

	if err := r.reconcileStream(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionKS(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *KinesisStreamReconciler) reconcileStream(ctx context.Context, obj *awsv1alpha1.KinesisStream) error {
	descOut, err := r.KinesisClient.DescribeStreamSummary(ctx, &awskinesis.DescribeStreamSummaryInput{
		StreamName: aws.String(obj.Spec.StreamName),
	})
	if err != nil && !kinesishelper.IsNotFound(err) {
		return fmt.Errorf("describe kinesis stream: %w", err)
	}

	if err == nil && descOut.StreamDescriptionSummary != nil {
		summary := descOut.StreamDescriptionSummary
		obj.Status.StreamARN = aws.ToString(summary.StreamARN)
		obj.Status.StreamStatus = string(summary.StreamStatus)

		if summary.StreamStatus != kinesistypes.StreamStatusActive {
			return r.setConditionKS(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonCreated, fmt.Sprintf("stream status: %s", summary.StreamStatus))
		}

		obj.Status.ObservedGeneration = obj.Generation
		now := metav1.Now()
		obj.Status.LastSyncTime = &now
		return r.setConditionKS(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "KinesisStream reconciled")
	}

	input := &awskinesis.CreateStreamInput{
		StreamName: aws.String(obj.Spec.StreamName),
	}
	if obj.Spec.StreamMode != "" {
		mode := kinesistypes.StreamMode(obj.Spec.StreamMode)
		input.StreamModeDetails = &kinesistypes.StreamModeDetails{
			StreamMode: mode,
		}
	}
	if obj.Spec.ShardCount != nil {
		input.ShardCount = obj.Spec.ShardCount
	}

	if _, err := r.KinesisClient.CreateStream(ctx, input); err != nil {
		return fmt.Errorf("create kinesis stream: %w", err)
	}

	if len(obj.Spec.Tags) > 0 {
		tags := make(map[string]string, len(obj.Spec.Tags))
		for k, v := range obj.Spec.Tags {
			tags[k] = v
		}
		if _, err := r.KinesisClient.AddTagsToStream(ctx, &awskinesis.AddTagsToStreamInput{
			StreamName: aws.String(obj.Spec.StreamName),
			Tags:       tags,
		}); err != nil {
			return fmt.Errorf("add tags to kinesis stream: %w", err)
		}
	}

	return r.setConditionKS(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonCreated, "KinesisStream creating")
}

func (r *KinesisStreamReconciler) deleteStream(ctx context.Context, obj *awsv1alpha1.KinesisStream) error {
	_, err := r.KinesisClient.DeleteStream(ctx, &awskinesis.DeleteStreamInput{
		StreamName: aws.String(obj.Spec.StreamName),
	})
	if kinesishelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *KinesisStreamReconciler) setConditionKS(ctx context.Context, obj *awsv1alpha1.KinesisStream, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *KinesisStreamReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.KinesisStream{}).
		Complete(r)
}
