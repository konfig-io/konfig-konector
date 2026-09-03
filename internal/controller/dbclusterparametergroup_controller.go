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

// DBClusterParameterGroupReconciler reconciles DBClusterParameterGroup objects.
type DBClusterParameterGroupReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	RDSClient *awsrds.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=dbclusterparametergroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dbclusterparametergroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dbclusterparametergroups/finalizers,verbs=update

func (r *DBClusterParameterGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	pg := &awsv1alpha1.DBClusterParameterGroup{}
	if err := r.Get(ctx, req.NamespacedName, pg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !pg.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(pg, awsv1alpha1.FinalizerName) {
			if shouldAbandon(pg) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(pg, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, pg)
			}
			if _, err := r.RDSClient.DeleteDBClusterParameterGroup(ctx, &awsrds.DeleteDBClusterParameterGroupInput{
				DBClusterParameterGroupName: aws.String(pg.Spec.DBClusterParameterGroupName),
			}); err != nil && !rdshelper.IsNotFound(err) {
				logger.Error(err, "failed to delete DB cluster parameter group")
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

	if err := r.reconcile(ctx, pg); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, pg, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *DBClusterParameterGroupReconciler) reconcile(ctx context.Context, pg *awsv1alpha1.DBClusterParameterGroup) error {
	out, err := r.RDSClient.DescribeDBClusterParameterGroups(ctx, &awsrds.DescribeDBClusterParameterGroupsInput{
		DBClusterParameterGroupName: aws.String(pg.Spec.DBClusterParameterGroupName),
	})
	if err != nil && !rdshelper.IsNotFound(err) {
		return err
	}

	if rdshelper.IsNotFound(err) || len(out.DBClusterParameterGroups) == 0 {
		createOut, err := r.RDSClient.CreateDBClusterParameterGroup(ctx, &awsrds.CreateDBClusterParameterGroupInput{
			DBClusterParameterGroupName: aws.String(pg.Spec.DBClusterParameterGroupName),
			DBParameterGroupFamily:      aws.String(pg.Spec.DBParameterGroupFamily),
			Description:                 aws.String(pg.Spec.Description),
			Tags:                        rdsTagsFromMap(pg.Spec.Tags),
		})
		if err != nil {
			return fmt.Errorf("create DB cluster parameter group: %w", err)
		}
		pg.Status.ARN = aws.ToString(createOut.DBClusterParameterGroup.DBClusterParameterGroupArn)
	} else {
		pg.Status.ARN = aws.ToString(out.DBClusterParameterGroups[0].DBClusterParameterGroupArn)
	}

	// Apply parameter overrides.
	if len(pg.Spec.Parameters) > 0 {
		params := make([]rdstypes.Parameter, 0, len(pg.Spec.Parameters))
		for _, p := range pg.Spec.Parameters {
			p := p
			am := rdstypes.ApplyMethodImmediate
			if p.ApplyMethod == "pending-reboot" {
				am = rdstypes.ApplyMethodPendingReboot
			}
			params = append(params, rdstypes.Parameter{
				ParameterName:  &p.ParameterName,
				ParameterValue: &p.ParameterValue,
				ApplyMethod:    am,
			})
		}
		if _, err := r.RDSClient.ModifyDBClusterParameterGroup(ctx, &awsrds.ModifyDBClusterParameterGroupInput{
			DBClusterParameterGroupName: aws.String(pg.Spec.DBClusterParameterGroupName),
			Parameters:                  params,
		}); err != nil {
			return fmt.Errorf("modify DB cluster parameter group: %w", err)
		}
	}

	pg.Status.ObservedGeneration = pg.Generation
	now := metav1.Now()
	pg.Status.LastSyncTime = &now
	return r.setCondition(ctx, pg, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "DB cluster parameter group reconciled")
}

func (r *DBClusterParameterGroupReconciler) setCondition(ctx context.Context, pg *awsv1alpha1.DBClusterParameterGroup, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *DBClusterParameterGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.DBClusterParameterGroup{}).
		Complete(r)
}
