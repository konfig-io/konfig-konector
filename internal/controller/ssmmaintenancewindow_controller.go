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
	awsssm "github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ssmhelper "github.com/konfig-io/konfig-konector/internal/aws/ssm"
)

// SSMMaintenanceWindowAWSAPI is the subset of the SSM API used by this controller.
type SSMMaintenanceWindowAWSAPI interface {
	CreateMaintenanceWindow(ctx context.Context, params *awsssm.CreateMaintenanceWindowInput, optFns ...func(*awsssm.Options)) (*awsssm.CreateMaintenanceWindowOutput, error)
	UpdateMaintenanceWindow(ctx context.Context, params *awsssm.UpdateMaintenanceWindowInput, optFns ...func(*awsssm.Options)) (*awsssm.UpdateMaintenanceWindowOutput, error)
	DeleteMaintenanceWindow(ctx context.Context, params *awsssm.DeleteMaintenanceWindowInput, optFns ...func(*awsssm.Options)) (*awsssm.DeleteMaintenanceWindowOutput, error)
}

// SSMMaintenanceWindowReconciler reconciles SSMMaintenanceWindow objects.
type SSMMaintenanceWindowReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	SSMClient SSMMaintenanceWindowAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=ssmmaintenancewindows,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ssmmaintenancewindows/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ssmmaintenancewindows/finalizers,verbs=update

func (r *SSMMaintenanceWindowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	mw := &awsv1alpha1.SSMMaintenanceWindow{}
	if err := r.Get(ctx, req.NamespacedName, mw); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !mw.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(mw, awsv1alpha1.FinalizerName) {
			if shouldAbandon(mw) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(mw, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, mw)
			}
			if err := r.deleteWindow(ctx, mw); err != nil {
				logger.Error(err, "failed to delete maintenance window")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(mw, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, mw)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(mw, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(mw, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, mw); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileWindow(ctx, mw); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, mw, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *SSMMaintenanceWindowReconciler) reconcileWindow(ctx context.Context, mw *awsv1alpha1.SSMMaintenanceWindow) error {
	if mw.Status.WindowID == "" {
		input := &awsssm.CreateMaintenanceWindowInput{
			Name:                     aws.String(mw.Spec.Name),
			Schedule:                 aws.String(mw.Spec.Schedule),
			Duration:                 mw.Spec.Duration,
			Cutoff:                   mw.Spec.Cutoff,
			AllowUnassociatedTargets: mw.Spec.AllowUnassociatedTargets,
		}
		if mw.Spec.Timezone != "" {
			input.ScheduleTimezone = aws.String(mw.Spec.Timezone)
		}
		if mw.Spec.Description != "" {
			input.Description = aws.String(mw.Spec.Description)
		}
		for k, v := range mw.Spec.Tags {
			input.Tags = append(input.Tags, ssmtypes.Tag{Key: aws.String(k), Value: aws.String(v)})
		}
		out, err := r.SSMClient.CreateMaintenanceWindow(ctx, input)
		if err != nil {
			return fmt.Errorf("create maintenance window: %w", err)
		}
		mw.Status.WindowID = aws.ToString(out.WindowId)
		if err := persistStatus(ctx, r.Client, mw); err != nil {
			return fmt.Errorf("persist window ID after create: %w", err)
		}
	} else if mw.Status.ObservedGeneration != mw.Generation {
		input := &awsssm.UpdateMaintenanceWindowInput{
			WindowId:                 aws.String(mw.Status.WindowID),
			Name:                     aws.String(mw.Spec.Name),
			Schedule:                 aws.String(mw.Spec.Schedule),
			Duration:                 aws.Int32(mw.Spec.Duration),
			Cutoff:                   aws.Int32(mw.Spec.Cutoff),
			AllowUnassociatedTargets: aws.Bool(mw.Spec.AllowUnassociatedTargets),
		}
		if mw.Spec.Timezone != "" {
			input.ScheduleTimezone = aws.String(mw.Spec.Timezone)
		}
		if mw.Spec.Description != "" {
			input.Description = aws.String(mw.Spec.Description)
		}
		if _, err := r.SSMClient.UpdateMaintenanceWindow(ctx, input); err != nil {
			return fmt.Errorf("update maintenance window: %w", err)
		}
	}

	mw.Status.ObservedGeneration = mw.Generation
	now := metav1.Now()
	mw.Status.LastSyncTime = &now
	return r.setCondition(ctx, mw, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "maintenance window reconciled")
}

func (r *SSMMaintenanceWindowReconciler) deleteWindow(ctx context.Context, mw *awsv1alpha1.SSMMaintenanceWindow) error {
	if mw.Status.WindowID == "" {
		// Window IDs are AWS-generated and window names are not unique, so
		// there is no unambiguous spec-based lookup. Nothing to delete.
		return nil
	}
	_, err := r.SSMClient.DeleteMaintenanceWindow(ctx, &awsssm.DeleteMaintenanceWindowInput{
		WindowId: aws.String(mw.Status.WindowID),
	})
	if ssmhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SSMMaintenanceWindowReconciler) setCondition(ctx context.Context, mw *awsv1alpha1.SSMMaintenanceWindow, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&mw.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: mw.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, mw); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *SSMMaintenanceWindowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.SSMMaintenanceWindow{}).
		Complete(r)
}
