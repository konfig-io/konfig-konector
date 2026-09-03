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
	awsglue "github.com/aws/aws-sdk-go-v2/service/glue"
	gluetypes "github.com/aws/aws-sdk-go-v2/service/glue/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	gluehelper "github.com/konfig-io/konfig-konector/internal/aws/glue"
)

// GlueJobAWSAPI is the subset of the Glue SDK client used by this controller.
// *awsglue.Client satisfies it.
type GlueJobAWSAPI interface {
	GetJob(ctx context.Context, params *awsglue.GetJobInput, optFns ...func(*awsglue.Options)) (*awsglue.GetJobOutput, error)
	CreateJob(ctx context.Context, params *awsglue.CreateJobInput, optFns ...func(*awsglue.Options)) (*awsglue.CreateJobOutput, error)
	UpdateJob(ctx context.Context, params *awsglue.UpdateJobInput, optFns ...func(*awsglue.Options)) (*awsglue.UpdateJobOutput, error)
	DeleteJob(ctx context.Context, params *awsglue.DeleteJobInput, optFns ...func(*awsglue.Options)) (*awsglue.DeleteJobOutput, error)
}

// GlueJobReconciler reconciles GlueJob objects.
type GlueJobReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	GlueClient GlueJobAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=gluejobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=gluejobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=gluejobs/finalizers,verbs=update

func (r *GlueJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	job := &awsv1alpha1.GlueJob{}
	if err := r.Get(ctx, req.NamespacedName, job); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !job.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(job, awsv1alpha1.FinalizerName) {
			if shouldAbandon(job) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(job, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, job)
			}
			if err := r.deleteJob(ctx, job); err != nil {
				logger.Error(err, "failed to delete Glue job")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(job, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, job)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(job, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(job, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, job); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileJob(ctx, job); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, job, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func glueJobCommand(spec *awsv1alpha1.GlueJobCommand) *gluetypes.JobCommand {
	cmd := &gluetypes.JobCommand{
		Name:           aws.String(spec.Name),
		ScriptLocation: aws.String(spec.ScriptLocation),
	}
	if spec.PythonVersion != "" {
		cmd.PythonVersion = aws.String(spec.PythonVersion)
	}
	return cmd
}

func (r *GlueJobReconciler) reconcileJob(ctx context.Context, job *awsv1alpha1.GlueJob) error {
	roleARN, err := resolveIAMRoleARN(ctx, r.Client, job.Namespace, job.Spec.RoleRef)
	if err != nil {
		return err
	}

	_, err = r.GlueClient.GetJob(ctx, &awsglue.GetJobInput{
		JobName: aws.String(job.Spec.Name),
	})
	if gluehelper.IsNotFound(err) {
		input := &awsglue.CreateJobInput{
			Name:       aws.String(job.Spec.Name),
			Role:       aws.String(roleARN),
			Command:    glueJobCommand(&job.Spec.Command),
			MaxRetries: job.Spec.MaxRetries,
			Tags:       job.Spec.Tags,
		}
		if len(job.Spec.DefaultArguments) > 0 {
			input.DefaultArguments = job.Spec.DefaultArguments
		}
		if job.Spec.Timeout != nil {
			input.Timeout = job.Spec.Timeout
		}
		if job.Spec.GlueVersion != "" {
			input.GlueVersion = aws.String(job.Spec.GlueVersion)
		}
		if job.Spec.NumberOfWorkers != nil {
			input.NumberOfWorkers = job.Spec.NumberOfWorkers
		}
		if job.Spec.WorkerType != "" {
			input.WorkerType = gluetypes.WorkerType(job.Spec.WorkerType)
		}
		if job.Spec.Description != "" {
			input.Description = aws.String(job.Spec.Description)
		}
		if _, err := r.GlueClient.CreateJob(ctx, input); err != nil {
			return fmt.Errorf("create Glue job: %w", err)
		}
		// Persist the identifier immediately: the AWS resource now exists.
		job.Status.JobName = job.Spec.Name
		if err := persistStatus(ctx, r.Client, job); err != nil {
			return fmt.Errorf("persist job name after create: %w", err)
		}
	} else if err != nil {
		return err
	} else if job.Status.ObservedGeneration != job.Generation {
		update := &gluetypes.JobUpdate{
			Role:       aws.String(roleARN),
			Command:    glueJobCommand(&job.Spec.Command),
			MaxRetries: job.Spec.MaxRetries,
		}
		if len(job.Spec.DefaultArguments) > 0 {
			update.DefaultArguments = job.Spec.DefaultArguments
		}
		if job.Spec.Timeout != nil {
			update.Timeout = job.Spec.Timeout
		}
		if job.Spec.GlueVersion != "" {
			update.GlueVersion = aws.String(job.Spec.GlueVersion)
		}
		if job.Spec.NumberOfWorkers != nil {
			update.NumberOfWorkers = job.Spec.NumberOfWorkers
		}
		if job.Spec.WorkerType != "" {
			update.WorkerType = gluetypes.WorkerType(job.Spec.WorkerType)
		}
		if job.Spec.Description != "" {
			update.Description = aws.String(job.Spec.Description)
		}
		if _, err := r.GlueClient.UpdateJob(ctx, &awsglue.UpdateJobInput{
			JobName:   aws.String(job.Spec.Name),
			JobUpdate: update,
		}); err != nil {
			return fmt.Errorf("update Glue job: %w", err)
		}
	}

	job.Status.JobName = job.Spec.Name
	job.Status.ObservedGeneration = job.Generation
	now := metav1.Now()
	job.Status.LastSyncTime = &now
	return r.setCondition(ctx, job, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Glue job reconciled")
}

func (r *GlueJobReconciler) deleteJob(ctx context.Context, job *awsv1alpha1.GlueJob) error {
	name := job.Status.JobName
	if name == "" {
		name = job.Spec.Name
	}
	_, err := r.GlueClient.DeleteJob(ctx, &awsglue.DeleteJobInput{
		JobName: aws.String(name),
	})
	if gluehelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *GlueJobReconciler) setCondition(ctx context.Context, job *awsv1alpha1.GlueJob, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&job.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: job.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, job); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *GlueJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.GlueJob{}).
		Complete(r)
}
