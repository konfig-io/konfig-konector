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
	awsrds "github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	rdshelper "github.com/konfig-io/konfig-konector/internal/aws/rds"
)

// DBOptionGroupReconciler reconciles DBOptionGroup objects.
type DBOptionGroupReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	RDSClient *awsrds.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=dboptiongroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dboptiongroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dboptiongroups/finalizers,verbs=update

func (r *DBOptionGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	og := &awsv1alpha1.DBOptionGroup{}
	if err := r.Get(ctx, req.NamespacedName, og); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !og.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(og, awsv1alpha1.FinalizerName) {
			if shouldAbandon(og) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(og, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, og)
			}
			if err := r.deleteDBOptionGroup(ctx, og); err != nil {
				logger.Error(err, "failed to delete DBOptionGroup")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(og, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, og)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(og, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(og, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, og); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileDBOptionGroup(ctx, og); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionOG(ctx, og, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *DBOptionGroupReconciler) reconcileDBOptionGroup(ctx context.Context, og *awsv1alpha1.DBOptionGroup) error {
	// Try to describe existing.
	descOut, err := r.RDSClient.DescribeOptionGroups(ctx, &awsrds.DescribeOptionGroupsInput{
		OptionGroupName: aws.String(og.Spec.OptionGroupName),
	})
	if err != nil && !rdshelper.IsNotFound(err) {
		return fmt.Errorf("describe option group: %w", err)
	}

	if err == nil && len(descOut.OptionGroupsList) > 0 {
		existing := descOut.OptionGroupsList[0]
		og.Status.ARN = aws.ToString(existing.OptionGroupArn)

		// Modify options.
		if err := r.syncOptions(ctx, og); err != nil {
			return err
		}

		// Sync tags.
		if len(og.Spec.Tags) > 0 {
			rdsTags := make([]rdstypes.Tag, 0, len(og.Spec.Tags))
			for k, v := range og.Spec.Tags {
				k, v := k, v
				rdsTags = append(rdsTags, rdstypes.Tag{Key: &k, Value: &v})
			}
			if _, tagErr := r.RDSClient.AddTagsToResource(ctx, &awsrds.AddTagsToResourceInput{
				ResourceName: aws.String(og.Status.ARN),
				Tags:         rdsTags,
			}); tagErr != nil {
				return fmt.Errorf("tag option group: %w", tagErr)
			}
		}
	} else {
		// Create.
		createOut, createErr := r.RDSClient.CreateOptionGroup(ctx, &awsrds.CreateOptionGroupInput{
			OptionGroupName:        aws.String(og.Spec.OptionGroupName),
			OptionGroupDescription: aws.String(og.Spec.OptionGroupDescription),
			EngineName:             aws.String(og.Spec.EngineName),
			MajorEngineVersion:     aws.String(og.Spec.MajorEngineVersion),
		})
		if createErr != nil {
			return fmt.Errorf("create option group: %w", createErr)
		}
		og.Status.ARN = aws.ToString(createOut.OptionGroup.OptionGroupArn)

		if err := r.syncOptions(ctx, og); err != nil {
			return err
		}
	}

	og.Status.ObservedGeneration = og.Generation
	now := metav1.Now()
	og.Status.LastSyncTime = &now
	return r.setConditionOG(ctx, og, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "DBOptionGroup reconciled")
}

func (r *DBOptionGroupReconciler) syncOptions(ctx context.Context, og *awsv1alpha1.DBOptionGroup) error {
	if len(og.Spec.Options) == 0 {
		return nil
	}
	opts := make([]rdstypes.OptionConfiguration, 0, len(og.Spec.Options))
	for _, o := range og.Spec.Options {
		oc := rdstypes.OptionConfiguration{
			OptionName: aws.String(o.OptionName),
		}
		if o.Port > 0 {
			oc.Port = aws.Int32(o.Port)
		}
		for _, s := range o.OptionSettings {
			s := s
			oc.OptionSettings = append(oc.OptionSettings, rdstypes.OptionSetting{
				Name:  &s.Name,
				Value: &s.Value,
			})
		}
		opts = append(opts, oc)
	}
	if _, err := r.RDSClient.ModifyOptionGroup(ctx, &awsrds.ModifyOptionGroupInput{
		OptionGroupName:  aws.String(og.Spec.OptionGroupName),
		OptionsToInclude: opts,
		ApplyImmediately: aws.Bool(true),
	}); err != nil {
		return fmt.Errorf("modify option group: %w", err)
	}
	return nil
}

func (r *DBOptionGroupReconciler) deleteDBOptionGroup(ctx context.Context, og *awsv1alpha1.DBOptionGroup) error {
	if og.Spec.OptionGroupName == "" {
		return nil
	}
	_, err := r.RDSClient.DeleteOptionGroup(ctx, &awsrds.DeleteOptionGroupInput{
		OptionGroupName: aws.String(og.Spec.OptionGroupName),
	})
	if rdshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *DBOptionGroupReconciler) setConditionOG(ctx context.Context, og *awsv1alpha1.DBOptionGroup, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&og.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: og.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, og); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *DBOptionGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.DBOptionGroup{}).
		Complete(r)
}
