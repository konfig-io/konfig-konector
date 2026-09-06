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
	awsathena "github.com/aws/aws-sdk-go-v2/service/athena"
	athenatypes "github.com/aws/aws-sdk-go-v2/service/athena/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	athenahelper "github.com/konfig-io/konfig-konector/internal/aws/athena"
)

// AthenaWorkGroupAWSAPI is the subset of the Athena SDK client used by this
// controller. *awsathena.Client satisfies it.
type AthenaWorkGroupAWSAPI interface {
	GetWorkGroup(ctx context.Context, params *awsathena.GetWorkGroupInput, optFns ...func(*awsathena.Options)) (*awsathena.GetWorkGroupOutput, error)
	CreateWorkGroup(ctx context.Context, params *awsathena.CreateWorkGroupInput, optFns ...func(*awsathena.Options)) (*awsathena.CreateWorkGroupOutput, error)
	UpdateWorkGroup(ctx context.Context, params *awsathena.UpdateWorkGroupInput, optFns ...func(*awsathena.Options)) (*awsathena.UpdateWorkGroupOutput, error)
	DeleteWorkGroup(ctx context.Context, params *awsathena.DeleteWorkGroupInput, optFns ...func(*awsathena.Options)) (*awsathena.DeleteWorkGroupOutput, error)
}

// AthenaWorkGroupReconciler reconciles AthenaWorkGroup objects.
type AthenaWorkGroupReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	AthenaClient AthenaWorkGroupAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=athenaworkgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=athenaworkgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=athenaworkgroups/finalizers,verbs=update

func (r *AthenaWorkGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	wg := &awsv1alpha1.AthenaWorkGroup{}
	if err := r.Get(ctx, req.NamespacedName, wg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, wg); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !wg.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(wg, awsv1alpha1.FinalizerName) {
			if shouldAbandon(wg) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(wg, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, wg)
			}
			if err := r.deleteWorkGroup(ctx, wg); err != nil {
				logger.Error(err, "failed to delete Athena workgroup")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(wg, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, wg)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(wg, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(wg, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, wg); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileWorkGroup(ctx, wg); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, wg, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func athenaResultConfiguration(spec *awsv1alpha1.AthenaResultConfiguration) *athenatypes.ResultConfiguration {
	if spec == nil {
		return nil
	}
	rc := &athenatypes.ResultConfiguration{}
	if spec.OutputLocation != "" {
		rc.OutputLocation = aws.String(spec.OutputLocation)
	}
	if spec.EncryptionConfiguration != nil {
		ec := &athenatypes.EncryptionConfiguration{
			EncryptionOption: athenatypes.EncryptionOption(spec.EncryptionConfiguration.EncryptionOption),
		}
		if spec.EncryptionConfiguration.KMSKey != "" {
			ec.KmsKey = aws.String(spec.EncryptionConfiguration.KMSKey)
		}
		rc.EncryptionConfiguration = ec
	}
	return rc
}

func athenaTags(tags map[string]string) []athenatypes.Tag {
	if len(tags) == 0 {
		return nil
	}
	out := make([]athenatypes.Tag, 0, len(tags))
	for k, v := range tags {
		out = append(out, athenatypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	return out
}

func (r *AthenaWorkGroupReconciler) reconcileWorkGroup(ctx context.Context, wg *awsv1alpha1.AthenaWorkGroup) error {
	getOut, err := r.AthenaClient.GetWorkGroup(ctx, &awsathena.GetWorkGroupInput{
		WorkGroup: aws.String(wg.Spec.Name),
	})
	if athenahelper.IsNotFound(err) {
		cfg := &athenatypes.WorkGroupConfiguration{
			ResultConfiguration:             athenaResultConfiguration(wg.Spec.ResultConfiguration),
			EnforceWorkGroupConfiguration:   wg.Spec.EnforceWorkGroupConfiguration,
			PublishCloudWatchMetricsEnabled: wg.Spec.PublishCloudWatchMetricsEnabled,
			BytesScannedCutoffPerQuery:      wg.Spec.BytesScannedCutoffPerQuery,
		}
		input := &awsathena.CreateWorkGroupInput{
			Name:          aws.String(wg.Spec.Name),
			Configuration: cfg,
			Tags:          athenaTags(wg.Spec.Tags),
		}
		if wg.Spec.Description != "" {
			input.Description = aws.String(wg.Spec.Description)
		}
		if _, err := r.AthenaClient.CreateWorkGroup(ctx, input); err != nil {
			return fmt.Errorf("create Athena workgroup: %w", err)
		}
		// Persist the identifier immediately: the AWS resource now exists.
		wg.Status.WorkGroupName = wg.Spec.Name
		if err := persistStatus(ctx, r.Client, wg); err != nil {
			return fmt.Errorf("persist workgroup name after create: %w", err)
		}
	} else if err != nil {
		return err
	} else {
		if getOut.WorkGroup != nil {
			wg.Status.State = string(getOut.WorkGroup.State)
		}
		if wg.Status.ObservedGeneration != wg.Generation {
			updates := &athenatypes.WorkGroupConfigurationUpdates{
				EnforceWorkGroupConfiguration:   wg.Spec.EnforceWorkGroupConfiguration,
				PublishCloudWatchMetricsEnabled: wg.Spec.PublishCloudWatchMetricsEnabled,
			}
			if wg.Spec.BytesScannedCutoffPerQuery != nil {
				updates.BytesScannedCutoffPerQuery = wg.Spec.BytesScannedCutoffPerQuery
			} else {
				updates.RemoveBytesScannedCutoffPerQuery = aws.Bool(true)
			}
			if rc := wg.Spec.ResultConfiguration; rc != nil {
				rcu := &athenatypes.ResultConfigurationUpdates{}
				if rc.OutputLocation != "" {
					rcu.OutputLocation = aws.String(rc.OutputLocation)
				}
				if rc.EncryptionConfiguration != nil {
					ec := &athenatypes.EncryptionConfiguration{
						EncryptionOption: athenatypes.EncryptionOption(rc.EncryptionConfiguration.EncryptionOption),
					}
					if rc.EncryptionConfiguration.KMSKey != "" {
						ec.KmsKey = aws.String(rc.EncryptionConfiguration.KMSKey)
					}
					rcu.EncryptionConfiguration = ec
				}
				updates.ResultConfigurationUpdates = rcu
			}
			input := &awsathena.UpdateWorkGroupInput{
				WorkGroup:            aws.String(wg.Spec.Name),
				ConfigurationUpdates: updates,
			}
			if wg.Spec.Description != "" {
				input.Description = aws.String(wg.Spec.Description)
			}
			if _, err := r.AthenaClient.UpdateWorkGroup(ctx, input); err != nil {
				return fmt.Errorf("update Athena workgroup: %w", err)
			}
		}
	}

	wg.Status.WorkGroupName = wg.Spec.Name
	wg.Status.ObservedGeneration = wg.Generation
	now := metav1.Now()
	wg.Status.LastSyncTime = &now
	return r.setCondition(ctx, wg, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Athena workgroup reconciled")
}

func (r *AthenaWorkGroupReconciler) deleteWorkGroup(ctx context.Context, wg *awsv1alpha1.AthenaWorkGroup) error {
	name := wg.Status.WorkGroupName
	if name == "" {
		name = wg.Spec.Name
	}
	_, err := r.AthenaClient.DeleteWorkGroup(ctx, &awsathena.DeleteWorkGroupInput{
		WorkGroup:             aws.String(name),
		RecursiveDeleteOption: aws.Bool(true),
	})
	if athenahelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *AthenaWorkGroupReconciler) setCondition(ctx context.Context, wg *awsv1alpha1.AthenaWorkGroup, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&wg.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: wg.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, wg); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *AthenaWorkGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.AthenaWorkGroup{}).
		Complete(r)
}
