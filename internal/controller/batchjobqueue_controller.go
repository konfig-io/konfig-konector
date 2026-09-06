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
	awsbatch "github.com/aws/aws-sdk-go-v2/service/batch"
	batchtypes "github.com/aws/aws-sdk-go-v2/service/batch/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	batchhelper "github.com/konfig-io/konfig-konector/internal/aws/batch"
)

// BatchJobQueueAWSAPI is the subset of the Batch API used by this controller.
type BatchJobQueueAWSAPI interface {
	CreateJobQueue(ctx context.Context, params *awsbatch.CreateJobQueueInput, optFns ...func(*awsbatch.Options)) (*awsbatch.CreateJobQueueOutput, error)
	DescribeJobQueues(ctx context.Context, params *awsbatch.DescribeJobQueuesInput, optFns ...func(*awsbatch.Options)) (*awsbatch.DescribeJobQueuesOutput, error)
	UpdateJobQueue(ctx context.Context, params *awsbatch.UpdateJobQueueInput, optFns ...func(*awsbatch.Options)) (*awsbatch.UpdateJobQueueOutput, error)
	DeleteJobQueue(ctx context.Context, params *awsbatch.DeleteJobQueueInput, optFns ...func(*awsbatch.Options)) (*awsbatch.DeleteJobQueueOutput, error)
}

// BatchJobQueueReconciler reconciles BatchJobQueue objects.
type BatchJobQueueReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	BatchClient BatchJobQueueAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=batchjobqueues,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=batchjobqueues/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=batchjobqueues/finalizers,verbs=update

func (r *BatchJobQueueReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	jq := &awsv1alpha1.BatchJobQueue{}
	if err := r.Get(ctx, req.NamespacedName, jq); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, jq); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !jq.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(jq, awsv1alpha1.FinalizerName) {
			if shouldAbandon(jq) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(jq, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, jq)
			}
			if err := r.deleteJobQueue(ctx, jq); err != nil {
				logger.Error(err, "failed to delete Batch job queue")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(jq, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, jq)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(jq, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(jq, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, jq); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	result, err := r.reconcileJobQueue(ctx, jq)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, jq, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *BatchJobQueueReconciler) reconcileJobQueue(ctx context.Context, jq *awsv1alpha1.BatchJobQueue) (ctrl.Result, error) {
	ceOrder, err := r.resolveComputeEnvironmentOrder(ctx, jq)
	if err != nil {
		return ctrl.Result{}, err
	}

	out, err := r.BatchClient.DescribeJobQueues(ctx, &awsbatch.DescribeJobQueuesInput{
		JobQueues: []string{jq.Spec.Name},
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	if len(out.JobQueues) == 0 {
		createIn := &awsbatch.CreateJobQueueInput{
			JobQueueName:            aws.String(jq.Spec.Name),
			Priority:                aws.Int32(jq.Spec.Priority),
			ComputeEnvironmentOrder: ceOrder,
		}
		if jq.Spec.State != "" {
			createIn.State = batchtypes.JQState(jq.Spec.State)
		}
		if len(jq.Spec.Tags) > 0 {
			createIn.Tags = jq.Spec.Tags
		}
		created, err := r.BatchClient.CreateJobQueue(ctx, createIn)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("create Batch job queue: %w", err)
		}
		jq.Status.JobQueueARN = aws.ToString(created.JobQueueArn)
		jq.Status.Status = string(batchtypes.JQStatusCreating)
		// Persist the ARN immediately: the AWS resource now exists, and
		// losing the identifier would orphan it on delete.
		if err := persistStatus(ctx, r.Client, jq); err != nil {
			return ctrl.Result{}, fmt.Errorf("persist job queue ARN after create: %w", err)
		}
		_ = r.setCondition(ctx, jq, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "Batch job queue is CREATING")
		return requeueBatchPolling, nil
	}

	detail := out.JobQueues[0]
	jq.Status.JobQueueARN = aws.ToString(detail.JobQueueArn)
	jq.Status.Status = string(detail.Status)

	if detail.Status != batchtypes.JQStatusValid {
		if detail.Status == batchtypes.JQStatusInvalid {
			_ = r.setCondition(ctx, jq, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError,
				fmt.Sprintf("Batch job queue is INVALID: %s", aws.ToString(detail.StatusReason)))
			return requeueResult(), nil
		}
		_ = r.setCondition(ctx, jq, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning",
			fmt.Sprintf("Batch job queue is %s", detail.Status))
		return requeueBatchPolling, nil
	}

	if jq.Status.ObservedGeneration != jq.Generation {
		updateIn := &awsbatch.UpdateJobQueueInput{
			JobQueue:                aws.String(jq.Spec.Name),
			Priority:                aws.Int32(jq.Spec.Priority),
			ComputeEnvironmentOrder: ceOrder,
		}
		if jq.Spec.State != "" {
			updateIn.State = batchtypes.JQState(jq.Spec.State)
		}
		if _, err := r.BatchClient.UpdateJobQueue(ctx, updateIn); err != nil {
			return ctrl.Result{}, fmt.Errorf("update Batch job queue: %w", err)
		}
	}

	jq.Status.ObservedGeneration = jq.Generation
	now := metav1.Now()
	jq.Status.LastSyncTime = &now
	return requeueResult(), r.setCondition(ctx, jq, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Batch job queue valid")
}

func (r *BatchJobQueueReconciler) resolveComputeEnvironmentOrder(ctx context.Context, jq *awsv1alpha1.BatchJobQueue) ([]batchtypes.ComputeEnvironmentOrder, error) {
	order := make([]batchtypes.ComputeEnvironmentOrder, 0, len(jq.Spec.ComputeEnvironmentOrder))
	for _, ceo := range jq.Spec.ComputeEnvironmentOrder {
		arn := ceo.ComputeEnvironmentRef.ARN
		if arn == "" {
			ceCR := &awsv1alpha1.BatchComputeEnvironment{}
			if err := r.Get(ctx, k8stypes.NamespacedName{Name: ceo.ComputeEnvironmentRef.Name, Namespace: jq.Namespace}, ceCR); err != nil {
				return nil, err
			}
			if ceCR.Status.ComputeEnvironmentARN == "" {
				return nil, &dependencyNotReady{msg: fmt.Sprintf("BatchComputeEnvironment %s/%s has no ARN yet", jq.Namespace, ceo.ComputeEnvironmentRef.Name)}
			}
			if ceCR.Status.Status != string(batchtypes.CEStatusValid) {
				return nil, &dependencyNotReady{msg: fmt.Sprintf("BatchComputeEnvironment %s/%s is not yet VALID (status: %s)", jq.Namespace, ceo.ComputeEnvironmentRef.Name, ceCR.Status.Status)}
			}
			arn = ceCR.Status.ComputeEnvironmentARN
		}
		order = append(order, batchtypes.ComputeEnvironmentOrder{
			ComputeEnvironment: aws.String(arn),
			Order:              aws.Int32(ceo.Order),
		})
	}
	return order, nil
}

func (r *BatchJobQueueReconciler) deleteJobQueue(ctx context.Context, jq *awsv1alpha1.BatchJobQueue) error {
	// The job queue name is deterministic from the spec, so no status
	// identifier is needed for deletion.
	_, err := r.BatchClient.DeleteJobQueue(ctx, &awsbatch.DeleteJobQueueInput{
		JobQueue: aws.String(jq.Spec.Name),
	})
	if batchhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *BatchJobQueueReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.BatchJobQueue, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *BatchJobQueueReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.BatchJobQueue{}).
		Complete(r)
}
