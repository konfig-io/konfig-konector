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
	"time"

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

var requeueBatchPolling = ctrl.Result{RequeueAfter: 15 * time.Second}

// BatchComputeEnvironmentAWSAPI is the subset of the Batch API used by this controller.
type BatchComputeEnvironmentAWSAPI interface {
	CreateComputeEnvironment(ctx context.Context, params *awsbatch.CreateComputeEnvironmentInput, optFns ...func(*awsbatch.Options)) (*awsbatch.CreateComputeEnvironmentOutput, error)
	DescribeComputeEnvironments(ctx context.Context, params *awsbatch.DescribeComputeEnvironmentsInput, optFns ...func(*awsbatch.Options)) (*awsbatch.DescribeComputeEnvironmentsOutput, error)
	UpdateComputeEnvironment(ctx context.Context, params *awsbatch.UpdateComputeEnvironmentInput, optFns ...func(*awsbatch.Options)) (*awsbatch.UpdateComputeEnvironmentOutput, error)
	DeleteComputeEnvironment(ctx context.Context, params *awsbatch.DeleteComputeEnvironmentInput, optFns ...func(*awsbatch.Options)) (*awsbatch.DeleteComputeEnvironmentOutput, error)
}

// BatchComputeEnvironmentReconciler reconciles BatchComputeEnvironment objects.
type BatchComputeEnvironmentReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	BatchClient BatchComputeEnvironmentAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=batchcomputeenvironments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=batchcomputeenvironments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=batchcomputeenvironments/finalizers,verbs=update

func (r *BatchComputeEnvironmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ce := &awsv1alpha1.BatchComputeEnvironment{}
	if err := r.Get(ctx, req.NamespacedName, ce); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !ce.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ce, awsv1alpha1.FinalizerName) {
			if shouldAbandon(ce) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(ce, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, ce)
			}
			if err := r.deleteComputeEnvironment(ctx, ce); err != nil {
				logger.Error(err, "failed to delete Batch compute environment")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(ce, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, ce)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(ce, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(ce, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, ce); err != nil {
			return ctrl.Result{}, err
		}
	}

	result, err := r.reconcileComputeEnvironment(ctx, ce)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, ce, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *BatchComputeEnvironmentReconciler) reconcileComputeEnvironment(ctx context.Context, ce *awsv1alpha1.BatchComputeEnvironment) (ctrl.Result, error) {
	out, err := r.BatchClient.DescribeComputeEnvironments(ctx, &awsbatch.DescribeComputeEnvironmentsInput{
		ComputeEnvironments: []string{ce.Spec.Name},
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	if len(out.ComputeEnvironments) == 0 {
		// Create.
		createIn := &awsbatch.CreateComputeEnvironmentInput{
			ComputeEnvironmentName: aws.String(ce.Spec.Name),
			Type:                   batchtypes.CEType(ce.Spec.Type),
		}
		if ce.Spec.State != "" {
			createIn.State = batchtypes.CEState(ce.Spec.State)
		}
		if ce.Spec.ComputeResources != nil {
			cr, err := r.buildComputeResources(ctx, ce)
			if err != nil {
				return ctrl.Result{}, err
			}
			createIn.ComputeResources = cr
		}
		if len(ce.Spec.Tags) > 0 {
			createIn.Tags = ce.Spec.Tags
		}
		created, err := r.BatchClient.CreateComputeEnvironment(ctx, createIn)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("create Batch compute environment: %w", err)
		}
		ce.Status.ComputeEnvironmentARN = aws.ToString(created.ComputeEnvironmentArn)
		ce.Status.Status = string(batchtypes.CEStatusCreating)
		// Persist the ARN immediately: the AWS resource now exists, and
		// losing the identifier would orphan it on delete.
		if err := persistStatus(ctx, r.Client, ce); err != nil {
			return ctrl.Result{}, fmt.Errorf("persist compute environment ARN after create: %w", err)
		}
		_ = r.setCondition(ctx, ce, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "Batch compute environment is CREATING")
		return requeueBatchPolling, nil
	}

	detail := out.ComputeEnvironments[0]
	ce.Status.ComputeEnvironmentARN = aws.ToString(detail.ComputeEnvironmentArn)
	ce.Status.Status = string(detail.Status)

	if detail.Status != batchtypes.CEStatusValid {
		if detail.Status == batchtypes.CEStatusInvalid {
			_ = r.setCondition(ctx, ce, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError,
				fmt.Sprintf("Batch compute environment is INVALID: %s", aws.ToString(detail.StatusReason)))
			return requeueResult(), nil
		}
		_ = r.setCondition(ctx, ce, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning",
			fmt.Sprintf("Batch compute environment is %s", detail.Status))
		return requeueBatchPolling, nil
	}

	if ce.Status.ObservedGeneration != ce.Generation {
		updateIn := &awsbatch.UpdateComputeEnvironmentInput{
			ComputeEnvironment: aws.String(ce.Spec.Name),
		}
		if ce.Spec.State != "" {
			updateIn.State = batchtypes.CEState(ce.Spec.State)
		}
		if ce.Spec.ComputeResources != nil {
			cru, err := r.buildComputeResourceUpdate(ctx, ce)
			if err != nil {
				return ctrl.Result{}, err
			}
			updateIn.ComputeResources = cru
		}
		if _, err := r.BatchClient.UpdateComputeEnvironment(ctx, updateIn); err != nil {
			return ctrl.Result{}, fmt.Errorf("update Batch compute environment: %w", err)
		}
		_ = r.setCondition(ctx, ce, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Updating", "Batch compute environment is UPDATING")
		ce.Status.ObservedGeneration = ce.Generation
		if err := persistStatus(ctx, r.Client, ce); err != nil {
			return ctrl.Result{}, err
		}
		return requeueBatchPolling, nil
	}

	ce.Status.ObservedGeneration = ce.Generation
	now := metav1.Now()
	ce.Status.LastSyncTime = &now
	return requeueResult(), r.setCondition(ctx, ce, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Batch compute environment valid")
}

func (r *BatchComputeEnvironmentReconciler) buildComputeResources(ctx context.Context, ce *awsv1alpha1.BatchComputeEnvironment) (*batchtypes.ComputeResource, error) {
	spec := ce.Spec.ComputeResources
	subnetIDs, err := resolveSubnetIDs(ctx, r.Client, ce.Namespace, spec.SubnetRefs)
	if err != nil {
		return nil, err
	}
	sgIDs, err := resolveSGIDs(ctx, r.Client, ce.Namespace, spec.SecurityGroupRefs)
	if err != nil {
		return nil, err
	}
	cr := &batchtypes.ComputeResource{
		Type:     batchtypes.CRType(spec.Type),
		MaxvCpus: aws.Int32(spec.MaxvCpus),
		Subnets:  subnetIDs,
	}
	if len(sgIDs) > 0 {
		cr.SecurityGroupIds = sgIDs
	}
	if spec.MinvCpus != nil {
		cr.MinvCpus = spec.MinvCpus
	}
	if spec.DesiredvCpus != nil {
		cr.DesiredvCpus = spec.DesiredvCpus
	}
	if len(spec.InstanceTypes) > 0 {
		cr.InstanceTypes = spec.InstanceTypes
	}
	if spec.InstanceRoleArn != "" {
		cr.InstanceRole = aws.String(spec.InstanceRoleArn)
	}
	return cr, nil
}

func (r *BatchComputeEnvironmentReconciler) buildComputeResourceUpdate(ctx context.Context, ce *awsv1alpha1.BatchComputeEnvironment) (*batchtypes.ComputeResourceUpdate, error) {
	spec := ce.Spec.ComputeResources
	subnetIDs, err := resolveSubnetIDs(ctx, r.Client, ce.Namespace, spec.SubnetRefs)
	if err != nil {
		return nil, err
	}
	sgIDs, err := resolveSGIDs(ctx, r.Client, ce.Namespace, spec.SecurityGroupRefs)
	if err != nil {
		return nil, err
	}
	cru := &batchtypes.ComputeResourceUpdate{
		MaxvCpus: aws.Int32(spec.MaxvCpus),
	}
	if len(subnetIDs) > 0 {
		cru.Subnets = subnetIDs
	}
	if len(sgIDs) > 0 {
		cru.SecurityGroupIds = sgIDs
	}
	if spec.MinvCpus != nil {
		cru.MinvCpus = spec.MinvCpus
	}
	if spec.DesiredvCpus != nil {
		cru.DesiredvCpus = spec.DesiredvCpus
	}
	return cru, nil
}

func (r *BatchComputeEnvironmentReconciler) deleteComputeEnvironment(ctx context.Context, ce *awsv1alpha1.BatchComputeEnvironment) error {
	// The compute environment name is deterministic from the spec, so no
	// status identifier is needed for deletion.
	_, err := r.BatchClient.DeleteComputeEnvironment(ctx, &awsbatch.DeleteComputeEnvironmentInput{
		ComputeEnvironment: aws.String(ce.Spec.Name),
	})
	if batchhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *BatchComputeEnvironmentReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.BatchComputeEnvironment, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *BatchComputeEnvironmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.BatchComputeEnvironment{}).
		Complete(r)
}
