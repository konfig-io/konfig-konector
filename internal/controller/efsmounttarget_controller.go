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
)

// EFSMountTargetReconciler reconciles EFSMountTarget objects.
type EFSMountTargetReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EFSClient *awsefs.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=efsmounttargets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=efsmounttargets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=efsmounttargets/finalizers,verbs=update

func (r *EFSMountTargetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.EFSMountTarget{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
			if shouldAbandon(obj) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, obj)
			}
			if err := r.deleteMountTarget(ctx, obj); err != nil {
				logger.Error(err, "failed to delete EFSMountTarget")
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

	if err := r.reconcileMountTarget(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionEMT(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *EFSMountTargetReconciler) reconcileMountTarget(ctx context.Context, obj *awsv1alpha1.EFSMountTarget) error {
	if obj.Status.MountTargetID != "" {
		out, err := r.EFSClient.DescribeMountTargets(ctx, &awsefs.DescribeMountTargetsInput{
			MountTargetId: aws.String(obj.Status.MountTargetID),
		})
		if err != nil && !efshelper.IsNotFound(err) {
			return fmt.Errorf("describe efs mount target: %w", err)
		}
		if err == nil && len(out.MountTargets) > 0 {
			mt := out.MountTargets[0]
			obj.Status.LifeCycleState = string(mt.LifeCycleState)
			obj.Status.IPAddress = aws.ToString(mt.IpAddress)
			if mt.LifeCycleState != efstypes.LifeCycleStateAvailable {
				return nil
			}
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionEMT(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "EFSMountTarget reconciled")
		}
		obj.Status.MountTargetID = ""
	}

	input := &awsefs.CreateMountTargetInput{
		FileSystemId: aws.String(obj.Spec.FileSystemID),
		SubnetId:     aws.String(obj.Spec.SubnetID),
	}
	if len(obj.Spec.SecurityGroups) > 0 {
		input.SecurityGroups = obj.Spec.SecurityGroups
	}
	if obj.Spec.IPAddress != "" {
		input.IpAddress = aws.String(obj.Spec.IPAddress)
	}

	out, err := r.EFSClient.CreateMountTarget(ctx, input)
	if err != nil {
		return fmt.Errorf("create efs mount target: %w", err)
	}

	obj.Status.MountTargetID = aws.ToString(out.MountTargetId)
	obj.Status.LifeCycleState = string(out.LifeCycleState)
	obj.Status.IPAddress = aws.ToString(out.IpAddress)
	// Persist the ID immediately: the AWS resource now exists, and losing the
	// identifier would orphan it or create a duplicate on retry.
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist mount target ID after create: %w", err)
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionEMT(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "EFSMountTarget created")
}

func (r *EFSMountTargetReconciler) deleteMountTarget(ctx context.Context, obj *awsv1alpha1.EFSMountTarget) error {
	if obj.Status.MountTargetID == "" {
		return nil
	}
	_, err := r.EFSClient.DeleteMountTarget(ctx, &awsefs.DeleteMountTargetInput{
		MountTargetId: aws.String(obj.Status.MountTargetID),
	})
	if efshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *EFSMountTargetReconciler) setConditionEMT(ctx context.Context, obj *awsv1alpha1.EFSMountTarget, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *EFSMountTargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.EFSMountTarget{}).
		Complete(r)
}
