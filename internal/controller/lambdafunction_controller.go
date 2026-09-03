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

	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	lambdahelper "github.com/konfig-io/konfig-konector/internal/aws/lambda"
)

var requeueLambdaPolling = ctrl.Result{RequeueAfter: 5 * time.Second}

// LambdaFunctionAWSAPI is the subset of the Lambda SDK client used by this
// controller (via the lambdahelper package). *awslambda.Client satisfies it.
type LambdaFunctionAWSAPI interface {
	GetFunction(ctx context.Context, params *awslambda.GetFunctionInput, optFns ...func(*awslambda.Options)) (*awslambda.GetFunctionOutput, error)
	CreateFunction(ctx context.Context, params *awslambda.CreateFunctionInput, optFns ...func(*awslambda.Options)) (*awslambda.CreateFunctionOutput, error)
	UpdateFunctionConfiguration(ctx context.Context, params *awslambda.UpdateFunctionConfigurationInput, optFns ...func(*awslambda.Options)) (*awslambda.UpdateFunctionConfigurationOutput, error)
	UpdateFunctionCode(ctx context.Context, params *awslambda.UpdateFunctionCodeInput, optFns ...func(*awslambda.Options)) (*awslambda.UpdateFunctionCodeOutput, error)
	PutFunctionConcurrency(ctx context.Context, params *awslambda.PutFunctionConcurrencyInput, optFns ...func(*awslambda.Options)) (*awslambda.PutFunctionConcurrencyOutput, error)
	DeleteFunctionConcurrency(ctx context.Context, params *awslambda.DeleteFunctionConcurrencyInput, optFns ...func(*awslambda.Options)) (*awslambda.DeleteFunctionConcurrencyOutput, error)
	DeleteFunction(ctx context.Context, params *awslambda.DeleteFunctionInput, optFns ...func(*awslambda.Options)) (*awslambda.DeleteFunctionOutput, error)
}

// LambdaFunctionReconciler reconciles LambdaFunction objects.
type LambdaFunctionReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	LambdaClient LambdaFunctionAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdafunctions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdafunctions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=lambdafunctions/finalizers,verbs=update

func (r *LambdaFunctionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	fn := &awsv1alpha1.LambdaFunction{}
	if err := r.Get(ctx, req.NamespacedName, fn); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !fn.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(fn, awsv1alpha1.FinalizerName) {
			if shouldAbandon(fn) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(fn, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, fn)
			}
			if err := lambdahelper.DeleteFunction(ctx, r.LambdaClient, fn.Spec.FunctionName); err != nil && !lambdahelper.IsNotFound(err) {
				logger.Error(err, "failed to delete Lambda function")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(fn, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, fn)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(fn, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(fn, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, fn); err != nil {
			return ctrl.Result{}, err
		}
	}

	result, err := r.reconcileFunction(ctx, fn)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, fn, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *LambdaFunctionReconciler) reconcileFunction(ctx context.Context, fn *awsv1alpha1.LambdaFunction) (ctrl.Result, error) {
	roleArn, err := r.resolveRole(ctx, fn)
	if err != nil {
		return ctrl.Result{}, err
	}

	createIn := r.buildInput(fn, roleArn)

	if fn.Status.FunctionARN != "" {
		existing, err := lambdahelper.GetFunction(ctx, r.LambdaClient, fn.Spec.FunctionName)
		if err != nil && !lambdahelper.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err == nil && existing != nil {
			fn.Status.State = string(existing.State)
			if existing.CodeSha256 != nil {
				fn.Status.CodeSha256 = *existing.CodeSha256
			}

			if existing.State == types.StatePending {
				_ = r.setCondition(ctx, fn, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "Lambda function is Pending")
				return requeueLambdaPolling, nil
			}

			if fn.Status.ObservedGeneration != fn.Generation {
				if err := lambdahelper.UpdateFunctionConfiguration(ctx, r.LambdaClient, createIn); err != nil {
					return ctrl.Result{}, fmt.Errorf("update Lambda config: %w", err)
				}
				if err := lambdahelper.UpdateFunctionCode(ctx, r.LambdaClient, createIn); err != nil {
					return ctrl.Result{}, fmt.Errorf("update Lambda code: %w", err)
				}
				if fn.Spec.ReservedConcurrency != nil {
					rc := *fn.Spec.ReservedConcurrency
					if rc == -1 {
						if err := lambdahelper.DeleteReservedConcurrency(ctx, r.LambdaClient, fn.Spec.FunctionName); err != nil {
							return ctrl.Result{}, fmt.Errorf("delete reserved concurrency: %w", err)
						}
					} else {
						if err := lambdahelper.SetReservedConcurrency(ctx, r.LambdaClient, fn.Spec.FunctionName, rc); err != nil {
							return ctrl.Result{}, fmt.Errorf("set reserved concurrency: %w", err)
						}
					}
				}
			}

			fn.Status.ObservedGeneration = fn.Generation
			now := metav1.Now()
			fn.Status.LastSyncTime = &now
			return requeueResult(), r.setCondition(ctx, fn, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Lambda function active")
		}
	}

	created, err := lambdahelper.CreateFunction(ctx, r.LambdaClient, createIn)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create Lambda function: %w", err)
	}
	if created.FunctionArn != nil {
		fn.Status.FunctionARN = *created.FunctionArn
	}
	fn.Status.State = string(created.State)
	_ = r.setCondition(ctx, fn, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "Lambda function is Pending")
	return requeueLambdaPolling, nil
}

func (r *LambdaFunctionReconciler) resolveRole(ctx context.Context, fn *awsv1alpha1.LambdaFunction) (string, error) {
	if fn.Spec.RoleArn != "" {
		return fn.Spec.RoleArn, nil
	}
	if fn.Spec.RoleRef != nil {
		return resolveIAMRoleARN(ctx, r.Client, fn.Namespace, *fn.Spec.RoleRef)
	}
	return "", fmt.Errorf("either roleArn or roleRef must be set")
}

func (r *LambdaFunctionReconciler) buildInput(fn *awsv1alpha1.LambdaFunction, roleArn string) lambdahelper.CreateFunctionInput {
	in := lambdahelper.CreateFunctionInput{
		FunctionName: fn.Spec.FunctionName,
		RoleArn:      roleArn,
		Description:  fn.Spec.Description,
		Timeout:      fn.Spec.Timeout,
		MemorySize:   fn.Spec.MemorySize,
		Environment:  fn.Spec.Environment,
		Tags:         fn.Spec.Tags,
	}
	if fn.Spec.Code.ImageURI != "" {
		in.ImageURI = fn.Spec.Code.ImageURI
	} else if fn.Spec.Code.S3 != nil {
		in.S3Bucket = fn.Spec.Code.S3.S3Bucket
		in.S3Key = fn.Spec.Code.S3.S3Key
		in.S3ObjectVersion = fn.Spec.Code.S3.S3ObjectVersion
	}
	if fn.Spec.Runtime != "" {
		in.Runtime = types.Runtime(fn.Spec.Runtime)
	}
	in.Handler = fn.Spec.Handler
	if fn.Spec.Architecture != "" {
		in.Architecture = types.Architecture(fn.Spec.Architecture)
	}
	in.EphemeralStorageMB = fn.Spec.EphemeralStorageSize
	in.Layers = fn.Spec.Layers
	if fn.Spec.DeadLetterConfig != nil {
		in.DeadLetterTargetARN = fn.Spec.DeadLetterConfig.TargetARN
	}
	if fn.Spec.TracingConfig != nil {
		in.TracingMode = fn.Spec.TracingConfig.Mode
	}
	if fn.Spec.LoggingConfig != nil {
		in.LogFormat = fn.Spec.LoggingConfig.LogFormat
		in.LogGroup = fn.Spec.LoggingConfig.LogGroup
		in.SystemLogLevel = fn.Spec.LoggingConfig.SystemLogLevel
		in.ApplicationLogLevel = fn.Spec.LoggingConfig.ApplicationLogLevel
	}
	for _, fsc := range fn.Spec.FileSystemConfigs {
		in.FileSystemConfigs = append(in.FileSystemConfigs, lambdahelper.LambdaFileSystemConfig{
			ARN:            fsc.ARN,
			LocalMountPath: fsc.LocalMountPath,
		})
	}
	if fn.Spec.SnapStart != nil {
		in.SnapStartApplyOn = fn.Spec.SnapStart.ApplyOn
	}
	if fn.Spec.ImageConfig != nil {
		in.ImageCommand = fn.Spec.ImageConfig.Command
		in.ImageEntryPoint = fn.Spec.ImageConfig.EntryPoint
		in.ImageWorkingDir = fn.Spec.ImageConfig.WorkingDirectory
	}
	return in
}

func (r *LambdaFunctionReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.LambdaFunction, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *LambdaFunctionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.LambdaFunction{}).
		Complete(r)
}
