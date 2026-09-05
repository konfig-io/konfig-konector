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
	awsconfigservice "github.com/aws/aws-sdk-go-v2/service/configservice"
	configtypes "github.com/aws/aws-sdk-go-v2/service/configservice/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	confighelper "github.com/konfig-io/konfig-konector/internal/aws/configservice"
)

// ConfigDeliveryChannelAWSAPI is the subset of the AWS Config API used by this controller.
type ConfigDeliveryChannelAWSAPI interface {
	DescribeDeliveryChannels(ctx context.Context, params *awsconfigservice.DescribeDeliveryChannelsInput, optFns ...func(*awsconfigservice.Options)) (*awsconfigservice.DescribeDeliveryChannelsOutput, error)
	PutDeliveryChannel(ctx context.Context, params *awsconfigservice.PutDeliveryChannelInput, optFns ...func(*awsconfigservice.Options)) (*awsconfigservice.PutDeliveryChannelOutput, error)
	DeleteDeliveryChannel(ctx context.Context, params *awsconfigservice.DeleteDeliveryChannelInput, optFns ...func(*awsconfigservice.Options)) (*awsconfigservice.DeleteDeliveryChannelOutput, error)
}

// ConfigDeliveryChannelReconciler reconciles ConfigDeliveryChannel objects.
type ConfigDeliveryChannelReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	ConfigClient ConfigDeliveryChannelAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=configdeliverychannels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=configdeliverychannels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=configdeliverychannels/finalizers,verbs=update

func (r *ConfigDeliveryChannelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ch := &awsv1alpha1.ConfigDeliveryChannel{}
	if err := r.Get(ctx, req.NamespacedName, ch); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, ch); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !ch.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ch, awsv1alpha1.FinalizerName) {
			if shouldAbandon(ch) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(ch, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, ch)
			}
			if err := r.deleteChannel(ctx, ch); err != nil {
				logger.Error(err, "failed to delete delivery channel")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(ch, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, ch)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(ch, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(ch, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, ch); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileChannel(ctx, ch); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, ch, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ConfigDeliveryChannelReconciler) reconcileChannel(ctx context.Context, ch *awsv1alpha1.ConfigDeliveryChannel) error {
	channel := configtypes.DeliveryChannel{
		Name:         aws.String(ch.Spec.ChannelName),
		S3BucketName: aws.String(ch.Spec.S3BucketName),
		S3KeyPrefix:  govOptionalStr(ch.Spec.S3KeyPrefix),
		SnsTopicARN:  govOptionalStr(ch.Spec.SNSTopicARN),
	}
	if ch.Spec.DeliveryFrequency != "" {
		channel.ConfigSnapshotDeliveryProperties = &configtypes.ConfigSnapshotDeliveryProperties{
			DeliveryFrequency: configtypes.MaximumExecutionFrequency(ch.Spec.DeliveryFrequency),
		}
	}

	// PutDeliveryChannel is an idempotent upsert.
	if _, err := r.ConfigClient.PutDeliveryChannel(ctx, &awsconfigservice.PutDeliveryChannelInput{
		DeliveryChannel: &channel,
	}); err != nil {
		return fmt.Errorf("put delivery channel: %w", err)
	}
	if ch.Status.ChannelName == "" {
		ch.Status.ChannelName = ch.Spec.ChannelName
		if err := persistStatus(ctx, r.Client, ch); err != nil {
			return fmt.Errorf("persist channel name after put: %w", err)
		}
	}

	ch.Status.ChannelName = ch.Spec.ChannelName
	ch.Status.ObservedGeneration = ch.Generation
	now := metav1.Now()
	ch.Status.LastSyncTime = &now
	return r.setCondition(ctx, ch, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "delivery channel reconciled")
}

func (r *ConfigDeliveryChannelReconciler) deleteChannel(ctx context.Context, ch *awsv1alpha1.ConfigDeliveryChannel) error {
	// The channel name is a deterministic spec-based identifier.
	name := ch.Status.ChannelName
	if name == "" {
		name = ch.Spec.ChannelName
	}
	_, err := r.ConfigClient.DeleteDeliveryChannel(ctx, &awsconfigservice.DeleteDeliveryChannelInput{
		DeliveryChannelName: aws.String(name),
	})
	if confighelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ConfigDeliveryChannelReconciler) setCondition(ctx context.Context, ch *awsv1alpha1.ConfigDeliveryChannel, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&ch.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: ch.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, ch); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *ConfigDeliveryChannelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ConfigDeliveryChannel{}).
		Complete(r)
}
