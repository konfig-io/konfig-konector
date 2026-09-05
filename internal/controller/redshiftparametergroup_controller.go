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
	awsredshift "github.com/aws/aws-sdk-go-v2/service/redshift"
	redshifttypes "github.com/aws/aws-sdk-go-v2/service/redshift/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	redshifthelper "github.com/konfig-io/konfig-konector/internal/aws/redshift"
)

// RedshiftParameterGroupAWSAPI is the subset of the Redshift SDK client used
// by this controller. *awsredshift.Client satisfies it.
type RedshiftParameterGroupAWSAPI interface {
	DescribeClusterParameterGroups(ctx context.Context, params *awsredshift.DescribeClusterParameterGroupsInput, optFns ...func(*awsredshift.Options)) (*awsredshift.DescribeClusterParameterGroupsOutput, error)
	CreateClusterParameterGroup(ctx context.Context, params *awsredshift.CreateClusterParameterGroupInput, optFns ...func(*awsredshift.Options)) (*awsredshift.CreateClusterParameterGroupOutput, error)
	ModifyClusterParameterGroup(ctx context.Context, params *awsredshift.ModifyClusterParameterGroupInput, optFns ...func(*awsredshift.Options)) (*awsredshift.ModifyClusterParameterGroupOutput, error)
	DeleteClusterParameterGroup(ctx context.Context, params *awsredshift.DeleteClusterParameterGroupInput, optFns ...func(*awsredshift.Options)) (*awsredshift.DeleteClusterParameterGroupOutput, error)
}

// RedshiftParameterGroupReconciler reconciles RedshiftParameterGroup objects.
type RedshiftParameterGroupReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	RedshiftClient RedshiftParameterGroupAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=redshiftparametergroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=redshiftparametergroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=redshiftparametergroups/finalizers,verbs=update

func (r *RedshiftParameterGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	pg := &awsv1alpha1.RedshiftParameterGroup{}
	if err := r.Get(ctx, req.NamespacedName, pg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, pg); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !pg.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(pg, awsv1alpha1.FinalizerName) {
			if shouldAbandon(pg) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(pg, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, pg)
			}
			if err := r.deleteParameterGroup(ctx, pg); err != nil {
				logger.Error(err, "failed to delete Redshift parameter group")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(pg, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, pg)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(pg, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(pg, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, pg); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileParameterGroup(ctx, pg); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, pg, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func redshiftParameters(params []awsv1alpha1.RedshiftParameter) []redshifttypes.Parameter {
	out := make([]redshifttypes.Parameter, 0, len(params))
	for _, p := range params {
		out = append(out, redshifttypes.Parameter{
			ParameterName:  aws.String(p.Name),
			ParameterValue: aws.String(p.Value),
		})
	}
	return out
}

func (r *RedshiftParameterGroupReconciler) reconcileParameterGroup(ctx context.Context, pg *awsv1alpha1.RedshiftParameterGroup) error {
	_, err := r.RedshiftClient.DescribeClusterParameterGroups(ctx, &awsredshift.DescribeClusterParameterGroupsInput{
		ParameterGroupName: aws.String(pg.Spec.Name),
	})
	if redshifthelper.IsNotFound(err) {
		if _, err := r.RedshiftClient.CreateClusterParameterGroup(ctx, &awsredshift.CreateClusterParameterGroupInput{
			ParameterGroupName:   aws.String(pg.Spec.Name),
			ParameterGroupFamily: aws.String(pg.Spec.Family),
			Description:          aws.String(pg.Spec.Description),
			Tags:                 redshiftTags(pg.Spec.Tags),
		}); err != nil {
			return fmt.Errorf("create Redshift parameter group: %w", err)
		}
		// Persist the identifier immediately: the AWS resource now exists.
		pg.Status.ParameterGroupName = pg.Spec.Name
		if err := persistStatus(ctx, r.Client, pg); err != nil {
			return fmt.Errorf("persist parameter group name after create: %w", err)
		}
		// Apply parameter overrides after creation.
		if len(pg.Spec.Parameters) > 0 {
			if _, err := r.RedshiftClient.ModifyClusterParameterGroup(ctx, &awsredshift.ModifyClusterParameterGroupInput{
				ParameterGroupName: aws.String(pg.Spec.Name),
				Parameters:         redshiftParameters(pg.Spec.Parameters),
			}); err != nil {
				return fmt.Errorf("apply Redshift parameter group parameters: %w", err)
			}
		}
	} else if err != nil {
		return err
	} else if pg.Status.ObservedGeneration != pg.Generation && len(pg.Spec.Parameters) > 0 {
		if _, err := r.RedshiftClient.ModifyClusterParameterGroup(ctx, &awsredshift.ModifyClusterParameterGroupInput{
			ParameterGroupName: aws.String(pg.Spec.Name),
			Parameters:         redshiftParameters(pg.Spec.Parameters),
		}); err != nil {
			return fmt.Errorf("modify Redshift parameter group: %w", err)
		}
	}

	pg.Status.ParameterGroupName = pg.Spec.Name
	pg.Status.ObservedGeneration = pg.Generation
	now := metav1.Now()
	pg.Status.LastSyncTime = &now
	return r.setCondition(ctx, pg, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Redshift parameter group reconciled")
}

func (r *RedshiftParameterGroupReconciler) deleteParameterGroup(ctx context.Context, pg *awsv1alpha1.RedshiftParameterGroup) error {
	name := pg.Status.ParameterGroupName
	if name == "" {
		name = pg.Spec.Name
	}
	_, err := r.RedshiftClient.DeleteClusterParameterGroup(ctx, &awsredshift.DeleteClusterParameterGroupInput{
		ParameterGroupName: aws.String(name),
	})
	if redshifthelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *RedshiftParameterGroupReconciler) setCondition(ctx context.Context, pg *awsv1alpha1.RedshiftParameterGroup, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&pg.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: pg.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, pg); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *RedshiftParameterGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.RedshiftParameterGroup{}).
		Complete(r)
}
