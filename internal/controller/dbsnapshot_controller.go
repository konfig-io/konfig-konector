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
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
	rdshelper "github.com/konfig-io/konfig-konector/internal/aws/rds"
)

// DBSnapshotReconciler reconciles DBSnapshot objects.
type DBSnapshotReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	RDSClient *multi.RDS
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=dbsnapshots,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dbsnapshots/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dbsnapshots/finalizers,verbs=update

func (r *DBSnapshotReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	snap := &awsv1alpha1.DBSnapshot{}
	if err := r.Get(ctx, req.NamespacedName, snap); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, snap); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !snap.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(snap, awsv1alpha1.FinalizerName) {
			if shouldAbandon(snap) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(snap, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, snap)
			}
			if err := r.deleteDBSnapshot(ctx, snap); err != nil {
				logger.Error(err, "failed to delete DBSnapshot")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(snap, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, snap)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(snap, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(snap, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, snap); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileDBSnapshot(ctx, snap); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionDBS(ctx, snap, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *DBSnapshotReconciler) reconcileDBSnapshot(ctx context.Context, snap *awsv1alpha1.DBSnapshot) error {
	if snap.Status.SnapshotARN != "" {
		out, err := r.RDSClient.DescribeDBSnapshots(ctx, &awsrds.DescribeDBSnapshotsInput{
			DBSnapshotIdentifier: aws.String(snap.Spec.DBSnapshotIdentifier),
		})
		if err != nil && !rdshelper.IsNotFound(err) {
			return fmt.Errorf("describe db snapshot: %w", err)
		}
		if err == nil && len(out.DBSnapshots) > 0 {
			snap.Status.SnapshotStatus = aws.ToString(out.DBSnapshots[0].Status)
			snap.Status.ObservedGeneration = snap.Generation
			now := metav1.Now()
			snap.Status.LastSyncTime = &now
			return r.setConditionDBS(ctx, snap, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "DBSnapshot reconciled")
		}
		snap.Status.SnapshotARN = ""
	}

	tags := make([]rdstypes.Tag, 0, len(snap.Spec.Tags))
	for k, v := range snap.Spec.Tags {
		k, v := k, v
		tags = append(tags, rdstypes.Tag{Key: &k, Value: &v})
	}

	out, err := r.RDSClient.CreateDBSnapshot(ctx, &awsrds.CreateDBSnapshotInput{
		DBInstanceIdentifier: aws.String(snap.Spec.DBInstanceIdentifier),
		DBSnapshotIdentifier: aws.String(snap.Spec.DBSnapshotIdentifier),
		Tags:                 tags,
	})
	if err != nil {
		return fmt.Errorf("create db snapshot: %w", err)
	}

	snap.Status.SnapshotARN = aws.ToString(out.DBSnapshot.DBSnapshotArn)
	snap.Status.SnapshotStatus = aws.ToString(out.DBSnapshot.Status)
	snap.Status.ObservedGeneration = snap.Generation
	now := metav1.Now()
	snap.Status.LastSyncTime = &now
	return r.setConditionDBS(ctx, snap, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "DBSnapshot created")
}

func (r *DBSnapshotReconciler) deleteDBSnapshot(ctx context.Context, snap *awsv1alpha1.DBSnapshot) error {
	if snap.Status.SnapshotARN == "" {
		return nil
	}
	_, err := r.RDSClient.DeleteDBSnapshot(ctx, &awsrds.DeleteDBSnapshotInput{
		DBSnapshotIdentifier: aws.String(snap.Spec.DBSnapshotIdentifier),
	})
	if rdshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *DBSnapshotReconciler) setConditionDBS(ctx context.Context, snap *awsv1alpha1.DBSnapshot, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&snap.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: snap.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, snap); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *DBSnapshotReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.DBSnapshot{}).
		Complete(r)
}
