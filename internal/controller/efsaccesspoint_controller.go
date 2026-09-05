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

// EFSAccessPointReconciler reconciles EFSAccessPoint objects.
type EFSAccessPointReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EFSClient *multi.EFS
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=efsaccesspoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=efsaccesspoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=efsaccesspoints/finalizers,verbs=update

func (r *EFSAccessPointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.EFSAccessPoint{}
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
			if err := r.deleteAccessPoint(ctx, obj); err != nil {
				logger.Error(err, "failed to delete EFSAccessPoint")
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

	if err := r.reconcileAccessPoint(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionEAP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *EFSAccessPointReconciler) reconcileAccessPoint(ctx context.Context, obj *awsv1alpha1.EFSAccessPoint) error {
	if obj.Status.AccessPointID != "" {
		out, err := r.EFSClient.DescribeAccessPoints(ctx, &awsefs.DescribeAccessPointsInput{
			AccessPointId: aws.String(obj.Status.AccessPointID),
		})
		if err != nil && !efshelper.IsNotFound(err) {
			return fmt.Errorf("describe efs access point: %w", err)
		}
		if err == nil && len(out.AccessPoints) > 0 {
			ap := out.AccessPoints[0]
			obj.Status.LifeCycleState = string(ap.LifeCycleState)
			obj.Status.AccessPointARN = aws.ToString(ap.AccessPointArn)
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionEAP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "EFSAccessPoint reconciled")
		}
		obj.Status.AccessPointID = ""
	}

	tags := make([]efstypes.Tag, 0, len(obj.Spec.Tags))
	for k, v := range obj.Spec.Tags {
		k, v := k, v
		tags = append(tags, efstypes.Tag{Key: &k, Value: &v})
	}

	input := &awsefs.CreateAccessPointInput{
		FileSystemId: aws.String(obj.Spec.FileSystemID),
		Tags:         tags,
	}

	if obj.Spec.PosixUser != nil {
		pu := &efstypes.PosixUser{
			Uid: aws.Int64(obj.Spec.PosixUser.UID),
			Gid: aws.Int64(obj.Spec.PosixUser.GID),
		}
		if len(obj.Spec.PosixUser.SecondaryGIDs) > 0 {
			pu.SecondaryGids = obj.Spec.PosixUser.SecondaryGIDs
		}
		input.PosixUser = pu
	}

	if obj.Spec.RootDirectory != nil {
		rd := &efstypes.RootDirectory{}
		if obj.Spec.RootDirectory.Path != "" {
			rd.Path = aws.String(obj.Spec.RootDirectory.Path)
		}
		if obj.Spec.RootDirectory.CreationInfo != nil {
			rd.CreationInfo = &efstypes.CreationInfo{
				OwnerUid:    aws.Int64(obj.Spec.RootDirectory.CreationInfo.OwnerUID),
				OwnerGid:    aws.Int64(obj.Spec.RootDirectory.CreationInfo.OwnerGID),
				Permissions: aws.String(obj.Spec.RootDirectory.CreationInfo.Permissions),
			}
		}
		input.RootDirectory = rd
	}

	out, err := r.EFSClient.CreateAccessPoint(ctx, input)
	if err != nil {
		return fmt.Errorf("create efs access point: %w", err)
	}

	obj.Status.AccessPointID = aws.ToString(out.AccessPointId)
	obj.Status.AccessPointARN = aws.ToString(out.AccessPointArn)
	obj.Status.LifeCycleState = string(out.LifeCycleState)
	// Persist the ID immediately: the AWS resource now exists, and losing the
	// identifier would orphan it or create a duplicate on retry.
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist access point ID after create: %w", err)
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionEAP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "EFSAccessPoint created")
}

func (r *EFSAccessPointReconciler) deleteAccessPoint(ctx context.Context, obj *awsv1alpha1.EFSAccessPoint) error {
	if obj.Status.AccessPointID == "" {
		return nil
	}
	_, err := r.EFSClient.DeleteAccessPoint(ctx, &awsefs.DeleteAccessPointInput{
		AccessPointId: aws.String(obj.Status.AccessPointID),
	})
	if efshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *EFSAccessPointReconciler) setConditionEAP(ctx context.Context, obj *awsv1alpha1.EFSAccessPoint, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *EFSAccessPointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.EFSAccessPoint{}).
		Complete(r)
}
