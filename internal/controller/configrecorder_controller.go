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

// ConfigRecorderAWSAPI is the subset of the AWS Config API used by this controller.
type ConfigRecorderAWSAPI interface {
	DescribeConfigurationRecorders(ctx context.Context, params *awsconfigservice.DescribeConfigurationRecordersInput, optFns ...func(*awsconfigservice.Options)) (*awsconfigservice.DescribeConfigurationRecordersOutput, error)
	DescribeConfigurationRecorderStatus(ctx context.Context, params *awsconfigservice.DescribeConfigurationRecorderStatusInput, optFns ...func(*awsconfigservice.Options)) (*awsconfigservice.DescribeConfigurationRecorderStatusOutput, error)
	PutConfigurationRecorder(ctx context.Context, params *awsconfigservice.PutConfigurationRecorderInput, optFns ...func(*awsconfigservice.Options)) (*awsconfigservice.PutConfigurationRecorderOutput, error)
	DeleteConfigurationRecorder(ctx context.Context, params *awsconfigservice.DeleteConfigurationRecorderInput, optFns ...func(*awsconfigservice.Options)) (*awsconfigservice.DeleteConfigurationRecorderOutput, error)
	StartConfigurationRecorder(ctx context.Context, params *awsconfigservice.StartConfigurationRecorderInput, optFns ...func(*awsconfigservice.Options)) (*awsconfigservice.StartConfigurationRecorderOutput, error)
	StopConfigurationRecorder(ctx context.Context, params *awsconfigservice.StopConfigurationRecorderInput, optFns ...func(*awsconfigservice.Options)) (*awsconfigservice.StopConfigurationRecorderOutput, error)
}

// ConfigRecorderReconciler reconciles ConfigRecorder objects.
type ConfigRecorderReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	ConfigClient ConfigRecorderAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=configrecorders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=configrecorders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=configrecorders/finalizers,verbs=update

func (r *ConfigRecorderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	rec := &awsv1alpha1.ConfigRecorder{}
	if err := r.Get(ctx, req.NamespacedName, rec); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !rec.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(rec, awsv1alpha1.FinalizerName) {
			if shouldAbandon(rec) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(rec, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, rec)
			}
			if err := r.deleteRecorder(ctx, rec); err != nil {
				logger.Error(err, "failed to delete configuration recorder")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(rec, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, rec)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(rec, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(rec, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, rec); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileRecorder(ctx, rec); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, rec, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ConfigRecorderReconciler) reconcileRecorder(ctx context.Context, rec *awsv1alpha1.ConfigRecorder) error {
	roleARN := rec.Spec.RoleARN
	if roleARN == "" {
		if rec.Spec.RoleRef == nil {
			return fmt.Errorf("either roleARN or roleRef must be set")
		}
		arn, err := resolveIAMRoleARN(ctx, r.Client, rec.Namespace, *rec.Spec.RoleRef)
		if err != nil {
			return err
		}
		roleARN = arn
	}

	recorder := configtypes.ConfigurationRecorder{
		Name:    aws.String(rec.Spec.RecorderName),
		RoleARN: aws.String(roleARN),
	}
	if rg := rec.Spec.RecordingGroup; rg != nil {
		recorder.RecordingGroup = &configtypes.RecordingGroup{
			AllSupported:               rg.AllSupported,
			IncludeGlobalResourceTypes: rg.IncludeGlobalResourceTypes,
			ResourceTypes:              configResourceTypes(rg.ResourceTypes),
		}
	}

	// PutConfigurationRecorder is an idempotent upsert.
	if _, err := r.ConfigClient.PutConfigurationRecorder(ctx, &awsconfigservice.PutConfigurationRecorderInput{
		ConfigurationRecorder: &recorder,
	}); err != nil {
		return fmt.Errorf("put configuration recorder: %w", err)
	}
	if rec.Status.RecorderName == "" {
		rec.Status.RecorderName = rec.Spec.RecorderName
		if err := persistStatus(ctx, r.Client, rec); err != nil {
			return fmt.Errorf("persist recorder name after put: %w", err)
		}
	}

	// Converge the recording state.
	statusOut, err := r.ConfigClient.DescribeConfigurationRecorderStatus(ctx, &awsconfigservice.DescribeConfigurationRecorderStatusInput{
		ConfigurationRecorderNames: []string{rec.Spec.RecorderName},
	})
	if err != nil {
		return fmt.Errorf("describe recorder status: %w", err)
	}
	recording := false
	if len(statusOut.ConfigurationRecordersStatus) > 0 {
		recording = statusOut.ConfigurationRecordersStatus[0].Recording
	}
	wantEnabled := rec.Spec.Enabled == nil || *rec.Spec.Enabled
	if wantEnabled && !recording {
		if _, err := r.ConfigClient.StartConfigurationRecorder(ctx, &awsconfigservice.StartConfigurationRecorderInput{
			ConfigurationRecorderName: aws.String(rec.Spec.RecorderName),
		}); err != nil {
			return fmt.Errorf("start configuration recorder: %w", err)
		}
		recording = true
	} else if !wantEnabled && recording {
		if _, err := r.ConfigClient.StopConfigurationRecorder(ctx, &awsconfigservice.StopConfigurationRecorderInput{
			ConfigurationRecorderName: aws.String(rec.Spec.RecorderName),
		}); err != nil {
			return fmt.Errorf("stop configuration recorder: %w", err)
		}
		recording = false
	}

	rec.Status.RecorderName = rec.Spec.RecorderName
	rec.Status.Recording = recording
	rec.Status.ObservedGeneration = rec.Generation
	now := metav1.Now()
	rec.Status.LastSyncTime = &now
	return r.setCondition(ctx, rec, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "configuration recorder reconciled")
}

func (r *ConfigRecorderReconciler) deleteRecorder(ctx context.Context, rec *awsv1alpha1.ConfigRecorder) error {
	// The recorder name is a deterministic spec-based identifier.
	name := rec.Status.RecorderName
	if name == "" {
		name = rec.Spec.RecorderName
	}
	_, err := r.ConfigClient.DeleteConfigurationRecorder(ctx, &awsconfigservice.DeleteConfigurationRecorderInput{
		ConfigurationRecorderName: aws.String(name),
	})
	if confighelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ConfigRecorderReconciler) setCondition(ctx context.Context, rec *awsv1alpha1.ConfigRecorder, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&rec.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: rec.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, rec); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func configResourceTypes(in []string) []configtypes.ResourceType {
	if len(in) == 0 {
		return nil
	}
	out := make([]configtypes.ResourceType, 0, len(in))
	for _, t := range in {
		out = append(out, configtypes.ResourceType(t))
	}
	return out
}

func (r *ConfigRecorderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ConfigRecorder{}).
		Complete(r)
}
