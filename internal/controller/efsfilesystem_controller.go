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
	awsefs "github.com/aws/aws-sdk-go-v2/service/efs"
	efstypes "github.com/aws/aws-sdk-go-v2/service/efs/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	efshelper "github.com/konfig-io/konfig-konector/internal/aws/efs"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// EFSFileSystemReconciler reconciles EFSFileSystem objects.
type EFSFileSystemReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EFSClient *multi.EFS
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=efsfilesystems,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=efsfilesystems/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=efsfilesystems/finalizers,verbs=update

func (r *EFSFileSystemReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.EFSFileSystem{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, obj); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !obj.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
			if shouldAbandon(obj) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, obj)
			}
			if err := r.deleteFileSystem(ctx, obj); err != nil {
				logger.Error(err, "failed to delete EFSFileSystem")
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

	if err := r.reconcileFileSystem(ctx, obj); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionEFS(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *EFSFileSystemReconciler) reconcileFileSystem(ctx context.Context, obj *awsv1alpha1.EFSFileSystem) error {
	if obj.Status.FileSystemID != "" {
		out, err := r.EFSClient.DescribeFileSystems(ctx, &awsefs.DescribeFileSystemsInput{
			FileSystemId: aws.String(obj.Status.FileSystemID),
		})
		if err != nil && !efshelper.IsNotFound(err) {
			return fmt.Errorf("describe efs file system: %w", err)
		}
		if err == nil && len(out.FileSystems) > 0 {
			fs := out.FileSystems[0]
			obj.Status.LifeCycleState = string(fs.LifeCycleState)
			if fs.LifeCycleState != efstypes.LifeCycleStateAvailable {
				return nil
			}
			obj.Status.FileSystemARN = aws.ToString(fs.FileSystemArn)
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionEFS(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "EFSFileSystem reconciled")
		}
		obj.Status.FileSystemID = ""
	}

	tags := make([]efstypes.Tag, 0, len(obj.Spec.Tags))
	for k, v := range obj.Spec.Tags {
		k, v := k, v
		tags = append(tags, efstypes.Tag{Key: &k, Value: &v})
	}

	input := &awsefs.CreateFileSystemInput{
		Tags: tags,
	}
	if obj.Spec.PerformanceMode != "" {
		input.PerformanceMode = efstypes.PerformanceMode(obj.Spec.PerformanceMode)
	}
	if obj.Spec.ThroughputMode != "" {
		input.ThroughputMode = efstypes.ThroughputMode(obj.Spec.ThroughputMode)
	}
	if obj.Spec.ProvisionedThroughputInMibps != nil {
		input.ProvisionedThroughputInMibps = obj.Spec.ProvisionedThroughputInMibps
	}
	if obj.Spec.Encrypted {
		input.Encrypted = aws.Bool(true)
	}
	if obj.Spec.KMSKeyID != "" {
		input.KmsKeyId = aws.String(obj.Spec.KMSKeyID)
	}

	out, err := r.EFSClient.CreateFileSystem(ctx, input)
	if err != nil {
		return fmt.Errorf("create efs file system: %w", err)
	}

	obj.Status.FileSystemID = aws.ToString(out.FileSystemId)
	obj.Status.FileSystemARN = aws.ToString(out.FileSystemArn)
	obj.Status.LifeCycleState = string(out.LifeCycleState)
	// Persist the ID immediately: the AWS resource now exists, and losing the
	// identifier would orphan it or create a duplicate on retry.
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist file system ID after create: %w", err)
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionEFS(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "EFSFileSystem created")
}

func (r *EFSFileSystemReconciler) deleteFileSystem(ctx context.Context, obj *awsv1alpha1.EFSFileSystem) error {
	if obj.Status.FileSystemID == "" {
		return nil
	}
	_, err := r.EFSClient.DeleteFileSystem(ctx, &awsefs.DeleteFileSystemInput{
		FileSystemId: aws.String(obj.Status.FileSystemID),
	})
	if efshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *EFSFileSystemReconciler) setConditionEFS(ctx context.Context, obj *awsv1alpha1.EFSFileSystem, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *EFSFileSystemReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.EFSFileSystem{}).
		Complete(r)
}
