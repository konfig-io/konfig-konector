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
	awsmq "github.com/aws/aws-sdk-go-v2/service/mq"
	mqtypes "github.com/aws/aws-sdk-go-v2/service/mq/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// MQConfigurationAWSAPI is the subset of the Amazon MQ API used by this controller.
type MQConfigurationAWSAPI interface {
	CreateConfiguration(ctx context.Context, params *awsmq.CreateConfigurationInput, optFns ...func(*awsmq.Options)) (*awsmq.CreateConfigurationOutput, error)
	UpdateConfiguration(ctx context.Context, params *awsmq.UpdateConfigurationInput, optFns ...func(*awsmq.Options)) (*awsmq.UpdateConfigurationOutput, error)
}

// MQConfigurationReconciler reconciles MQConfiguration objects.
type MQConfigurationReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	MQClient MQConfigurationAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=mqconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=mqconfigurations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=mqconfigurations/finalizers,verbs=update

func (r *MQConfigurationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	cfg := &awsv1alpha1.MQConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, cfg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, cfg); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !cfg.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(cfg, awsv1alpha1.FinalizerName) {
			if shouldAbandon(cfg) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(cfg, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, cfg)
			}
			if err := r.deleteConfiguration(ctx, cfg); err != nil {
				logger.Error(err, "failed to delete MQ configuration")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(cfg, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, cfg)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(cfg, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(cfg, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, cfg); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileConfiguration(ctx, cfg); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, cfg, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *MQConfigurationReconciler) reconcileConfiguration(ctx context.Context, cfg *awsv1alpha1.MQConfiguration) error {
	if cfg.Status.ConfigurationID == "" {
		createIn := &awsmq.CreateConfigurationInput{
			Name:       aws.String(cfg.Spec.Name),
			EngineType: mqtypes.EngineType(cfg.Spec.EngineType),
		}
		if cfg.Spec.EngineVersion != "" {
			createIn.EngineVersion = aws.String(cfg.Spec.EngineVersion)
		}
		if len(cfg.Spec.Tags) > 0 {
			createIn.Tags = cfg.Spec.Tags
		}
		created, err := r.MQClient.CreateConfiguration(ctx, createIn)
		if err != nil {
			return fmt.Errorf("create MQ configuration: %w", err)
		}
		cfg.Status.ConfigurationID = aws.ToString(created.Id)
		cfg.Status.ARN = aws.ToString(created.Arn)
		if created.LatestRevision != nil {
			cfg.Status.LatestRevision = aws.ToInt32(created.LatestRevision.Revision)
		}
		// Persist the configuration ID immediately: the AWS resource now
		// exists, and losing the identifier would create a duplicate on retry.
		if err := persistStatus(ctx, r.Client, cfg); err != nil {
			return fmt.Errorf("persist configuration ID after create: %w", err)
		}

		if cfg.Spec.Data != "" {
			if err := r.updateData(ctx, cfg); err != nil {
				return err
			}
		}
	} else if cfg.Status.ObservedGeneration != cfg.Generation && cfg.Spec.Data != "" {
		if err := r.updateData(ctx, cfg); err != nil {
			return err
		}
	}

	cfg.Status.ObservedGeneration = cfg.Generation
	now := metav1.Now()
	cfg.Status.LastSyncTime = &now
	return r.setCondition(ctx, cfg, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "MQ configuration reconciled")
}

func (r *MQConfigurationReconciler) updateData(ctx context.Context, cfg *awsv1alpha1.MQConfiguration) error {
	out, err := r.MQClient.UpdateConfiguration(ctx, &awsmq.UpdateConfigurationInput{
		ConfigurationId: aws.String(cfg.Status.ConfigurationID),
		Data:            aws.String(cfg.Spec.Data),
	})
	if err != nil {
		return fmt.Errorf("update MQ configuration: %w", err)
	}
	if out.LatestRevision != nil {
		cfg.Status.LatestRevision = aws.ToInt32(out.LatestRevision.Revision)
	}
	return nil
}

func (r *MQConfigurationReconciler) deleteConfiguration(_ context.Context, _ *awsv1alpha1.MQConfiguration) error {
	// Amazon MQ configurations cannot be deleted via a public API once
	// created (DeleteConfiguration only works for unreferenced configs and
	// is not part of the supported flow here). Deleting the CR simply stops
	// managing the AWS configuration.
	return nil
}

func (r *MQConfigurationReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.MQConfiguration, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *MQConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.MQConfiguration{}).
		Complete(r)
}
