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
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsbatch "github.com/aws/aws-sdk-go-v2/service/batch"
	batchtypes "github.com/aws/aws-sdk-go-v2/service/batch/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	batchhelper "github.com/konfig-io/konfig-konector/internal/aws/batch"
)

// BatchJobDefinitionAWSAPI is the subset of the Batch API used by this controller.
type BatchJobDefinitionAWSAPI interface {
	RegisterJobDefinition(ctx context.Context, params *awsbatch.RegisterJobDefinitionInput, optFns ...func(*awsbatch.Options)) (*awsbatch.RegisterJobDefinitionOutput, error)
	DeregisterJobDefinition(ctx context.Context, params *awsbatch.DeregisterJobDefinitionInput, optFns ...func(*awsbatch.Options)) (*awsbatch.DeregisterJobDefinitionOutput, error)
}

// BatchJobDefinitionReconciler reconciles BatchJobDefinition objects.
type BatchJobDefinitionReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	BatchClient BatchJobDefinitionAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=batchjobdefinitions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=batchjobdefinitions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=batchjobdefinitions/finalizers,verbs=update

func (r *BatchJobDefinitionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	jd := &awsv1alpha1.BatchJobDefinition{}
	if err := r.Get(ctx, req.NamespacedName, jd); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, jd); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !jd.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(jd, awsv1alpha1.FinalizerName) {
			if shouldAbandon(jd) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(jd, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, jd)
			}
			if jd.Status.JobDefinitionARN != "" {
				if _, err := r.BatchClient.DeregisterJobDefinition(ctx, &awsbatch.DeregisterJobDefinitionInput{
					JobDefinition: aws.String(jd.Status.JobDefinitionARN),
				}); err != nil && !batchhelper.IsNotFound(err) {
					logger.Error(err, "failed to deregister Batch job definition")
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(jd, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, jd)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(jd, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(jd, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, jd); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileJobDefinition(ctx, jd); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, jd, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *BatchJobDefinitionReconciler) reconcileJobDefinition(ctx context.Context, jd *awsv1alpha1.BatchJobDefinition) error {
	specHash, err := hashBatchJobDefinitionSpec(jd.Spec)
	if err != nil {
		return err
	}

	// Job definitions are immutable revisions (like ECS task definitions):
	// only register a new revision when the spec changed.
	if jd.Status.JobDefinitionARN != "" && jd.Status.SpecHash == specHash {
		jd.Status.ObservedGeneration = jd.Generation
		now := metav1.Now()
		jd.Status.LastSyncTime = &now
		return r.setCondition(ctx, jd, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Batch job definition up to date")
	}

	in, err := r.buildRegisterInput(ctx, jd)
	if err != nil {
		return err
	}

	registered, err := r.BatchClient.RegisterJobDefinition(ctx, in)
	if err != nil {
		return fmt.Errorf("register Batch job definition: %w", err)
	}
	jd.Status.JobDefinitionARN = aws.ToString(registered.JobDefinitionArn)
	jd.Status.Revision = aws.ToInt32(registered.Revision)
	jd.Status.SpecHash = specHash
	// Persist the ARN immediately: the revision now exists in AWS, and losing
	// the identifier would register a duplicate revision on retry.
	if err := persistStatus(ctx, r.Client, jd); err != nil {
		return fmt.Errorf("persist job definition ARN after register: %w", err)
	}
	jd.Status.ObservedGeneration = jd.Generation
	now := metav1.Now()
	jd.Status.LastSyncTime = &now
	return r.setCondition(ctx, jd, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Batch job definition registered")
}

func (r *BatchJobDefinitionReconciler) buildRegisterInput(ctx context.Context, jd *awsv1alpha1.BatchJobDefinition) (*awsbatch.RegisterJobDefinitionInput, error) {
	cp := jd.Spec.ContainerProperties
	container := &batchtypes.ContainerProperties{
		Image: aws.String(cp.Image),
	}
	for _, rr := range cp.ResourceRequirements {
		container.ResourceRequirements = append(container.ResourceRequirements, batchtypes.ResourceRequirement{
			Type:  batchtypes.ResourceType(rr.Type),
			Value: aws.String(rr.Value),
		})
	}
	if len(cp.Command) > 0 {
		container.Command = cp.Command
	}
	for k, v := range cp.Environment {
		k, v := k, v
		container.Environment = append(container.Environment, batchtypes.KeyValuePair{Name: &k, Value: &v})
	}

	if cp.JobRoleArn != "" {
		container.JobRoleArn = aws.String(cp.JobRoleArn)
	} else if cp.JobRoleRef != nil {
		arn, err := resolveIAMRoleARN(ctx, r.Client, jd.Namespace, *cp.JobRoleRef)
		if err != nil {
			return nil, err
		}
		container.JobRoleArn = aws.String(arn)
	}
	if cp.ExecutionRoleArn != "" {
		container.ExecutionRoleArn = aws.String(cp.ExecutionRoleArn)
	} else if cp.ExecutionRoleRef != nil {
		arn, err := resolveIAMRoleARN(ctx, r.Client, jd.Namespace, *cp.ExecutionRoleRef)
		if err != nil {
			return nil, err
		}
		container.ExecutionRoleArn = aws.String(arn)
	}

	in := &awsbatch.RegisterJobDefinitionInput{
		JobDefinitionName:   aws.String(jd.Spec.Name),
		Type:                batchtypes.JobDefinitionType(jd.Spec.Type),
		ContainerProperties: container,
	}
	for _, pc := range jd.Spec.PlatformCapabilities {
		in.PlatformCapabilities = append(in.PlatformCapabilities, batchtypes.PlatformCapability(pc))
	}
	if jd.Spec.RetryStrategy != nil {
		in.RetryStrategy = &batchtypes.RetryStrategy{
			Attempts: aws.Int32(jd.Spec.RetryStrategy.Attempts),
		}
	}
	if jd.Spec.Timeout != nil {
		in.Timeout = &batchtypes.JobTimeout{
			AttemptDurationSeconds: aws.Int32(jd.Spec.Timeout.AttemptDurationSeconds),
		}
	}
	if len(jd.Spec.Tags) > 0 {
		in.Tags = jd.Spec.Tags
	}
	return in, nil
}

func hashBatchJobDefinitionSpec(spec awsv1alpha1.BatchJobDefinitionSpec) (string, error) {
	b, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))[:16], nil
}

func (r *BatchJobDefinitionReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.BatchJobDefinition, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *BatchJobDefinitionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.BatchJobDefinition{}).
		Complete(r)
}
