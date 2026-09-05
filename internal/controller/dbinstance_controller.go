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

	"github.com/aws/aws-sdk-go-v2/aws"
	awsrds "github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	rdshelper "github.com/konfig-io/konfig-konector/internal/aws/rds"
)

var requeueRDSPolling = ctrl.Result{RequeueAfter: 30 * time.Second}

// DBInstanceAWSAPI is the subset of the RDS SDK client used by this controller.
// *awsrds.Client satisfies it.
type DBInstanceAWSAPI interface {
	DescribeDBInstances(ctx context.Context, params *awsrds.DescribeDBInstancesInput, optFns ...func(*awsrds.Options)) (*awsrds.DescribeDBInstancesOutput, error)
	CreateDBInstance(ctx context.Context, params *awsrds.CreateDBInstanceInput, optFns ...func(*awsrds.Options)) (*awsrds.CreateDBInstanceOutput, error)
	ModifyDBInstance(ctx context.Context, params *awsrds.ModifyDBInstanceInput, optFns ...func(*awsrds.Options)) (*awsrds.ModifyDBInstanceOutput, error)
	DeleteDBInstance(ctx context.Context, params *awsrds.DeleteDBInstanceInput, optFns ...func(*awsrds.Options)) (*awsrds.DeleteDBInstanceOutput, error)
}

// DBInstanceReconciler reconciles DBInstance objects.
type DBInstanceReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	RDSClient DBInstanceAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=dbinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dbinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dbinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

func (r *DBInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	db := &awsv1alpha1.DBInstance{}
	if err := r.Get(ctx, req.NamespacedName, db); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, db); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !db.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(db, awsv1alpha1.FinalizerName) {
			if shouldAbandon(db) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(db, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, db)
			}
			if err := r.deleteDBInstance(ctx, db); err != nil {
				logger.Error(err, "failed to delete DB instance")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(db, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, db)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(db, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(db, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, db); err != nil {
			return ctrl.Result{}, err
		}
	}

	result, err := r.reconcileDBInstance(ctx, db)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, db, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *DBInstanceReconciler) reconcileDBInstance(ctx context.Context, db *awsv1alpha1.DBInstance) (ctrl.Result, error) {
	// Check current state if we have an ARN.
	if db.Status.DBInstanceARN != "" {
		out, err := r.RDSClient.DescribeDBInstances(ctx, &awsrds.DescribeDBInstancesInput{
			DBInstanceIdentifier: aws.String(db.Spec.DBInstanceIdentifier),
		})
		if err != nil && !rdshelper.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err == nil && len(out.DBInstances) > 0 {
			inst := out.DBInstances[0]
			db.Status.DBInstanceStatus = aws.ToString(inst.DBInstanceStatus)
			if inst.Endpoint != nil {
				db.Status.Endpoint = aws.ToString(inst.Endpoint.Address)
				db.Status.Port = aws.ToInt32(inst.Endpoint.Port)
			}

			if rdshelper.IsTransient(db.Status.DBInstanceStatus) {
				_ = r.setCondition(ctx, db, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning",
					fmt.Sprintf("DB instance is %s", db.Status.DBInstanceStatus))
				return requeueRDSPolling, nil
			}

			if db.Status.DBInstanceStatus == "available" {
				// Drift: update if generation changed.
				if db.Status.ObservedGeneration != db.Generation {
					if err := r.updateDBInstance(ctx, db); err != nil {
						return ctrl.Result{}, err
					}
				}
				db.Status.ObservedGeneration = db.Generation
				now := metav1.Now()
				db.Status.LastSyncTime = &now
				return requeueResult(), r.setCondition(ctx, db, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "DB instance available")
			}
		}
	}

	// Create new instance.
	password, err := r.resolveSecret(ctx, db.Namespace, db.Spec.MasterUserPasswordRef)
	if err != nil {
		return ctrl.Result{}, err
	}

	sgIDs, err := r.resolveSGIDs(ctx, db.Namespace, db.Spec.VPCSecurityGroupRefs)
	if err != nil {
		return ctrl.Result{}, err
	}

	input := &awsrds.CreateDBInstanceInput{
		DBInstanceIdentifier: aws.String(db.Spec.DBInstanceIdentifier),
		DBInstanceClass:      aws.String(db.Spec.DBInstanceClass),
		Engine:               aws.String(db.Spec.Engine),
		MasterUsername:       aws.String(db.Spec.MasterUsername),
		MasterUserPassword:   aws.String(password),
		AllocatedStorage:     aws.Int32(db.Spec.AllocatedStorage),
		MultiAZ:              aws.Bool(db.Spec.MultiAZ),
		PubliclyAccessible:   aws.Bool(db.Spec.PubliclyAccessible),
		StorageEncrypted:     aws.Bool(db.Spec.StorageEncrypted),
		DeletionProtection:   aws.Bool(db.Spec.DeletionProtection),
		Tags:                 rdsTagsFromMap(db.Spec.Tags),
	}
	if db.Spec.EngineVersion != "" {
		input.EngineVersion = aws.String(db.Spec.EngineVersion)
	}
	if db.Spec.DBName != "" {
		input.DBName = aws.String(db.Spec.DBName)
	}
	if db.Spec.StorageType != "" {
		input.StorageType = aws.String(db.Spec.StorageType)
	}
	if db.Spec.KMSKeyID != "" {
		input.KmsKeyId = aws.String(db.Spec.KMSKeyID)
	}
	if db.Spec.Port > 0 {
		input.Port = aws.Int32(db.Spec.Port)
	}
	if db.Spec.BackupRetentionPeriod > 0 {
		input.BackupRetentionPeriod = aws.Int32(db.Spec.BackupRetentionPeriod)
	}
	if db.Spec.DBSubnetGroupRef != "" {
		snGroup := &awsv1alpha1.DBSubnetGroup{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: db.Spec.DBSubnetGroupRef, Namespace: db.Namespace}, snGroup); err != nil {
			return ctrl.Result{}, err
		}
		if snGroup.Spec.DBSubnetGroupName == "" {
			return ctrl.Result{}, &dependencyNotReady{msg: fmt.Sprintf("DBSubnetGroup %s/%s not ready", db.Namespace, db.Spec.DBSubnetGroupRef)}
		}
		input.DBSubnetGroupName = aws.String(snGroup.Spec.DBSubnetGroupName)
	}
	if db.Spec.DBParameterGroupRef != "" {
		pg := &awsv1alpha1.DBParameterGroup{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: db.Spec.DBParameterGroupRef, Namespace: db.Namespace}, pg); err != nil {
			return ctrl.Result{}, err
		}
		if pg.Spec.DBParameterGroupName == "" {
			return ctrl.Result{}, &dependencyNotReady{msg: fmt.Sprintf("DBParameterGroup %s/%s not ready", db.Namespace, db.Spec.DBParameterGroupRef)}
		}
		input.DBParameterGroupName = aws.String(pg.Spec.DBParameterGroupName)
	}
	if len(sgIDs) > 0 {
		input.VpcSecurityGroupIds = sgIDs
	}
	if db.Spec.MaxAllocatedStorage != nil {
		input.MaxAllocatedStorage = db.Spec.MaxAllocatedStorage
	}
	if db.Spec.PreferredBackupWindow != "" {
		input.PreferredBackupWindow = aws.String(db.Spec.PreferredBackupWindow)
	}
	if db.Spec.PreferredMaintenanceWindow != "" {
		input.PreferredMaintenanceWindow = aws.String(db.Spec.PreferredMaintenanceWindow)
	}
	if db.Spec.EnablePerformanceInsights != nil {
		input.EnablePerformanceInsights = db.Spec.EnablePerformanceInsights
	}
	if db.Spec.PerformanceInsightsKMSKeyID != "" {
		input.PerformanceInsightsKMSKeyId = aws.String(db.Spec.PerformanceInsightsKMSKeyID)
	}
	if db.Spec.PerformanceInsightsRetentionPeriod != nil {
		input.PerformanceInsightsRetentionPeriod = db.Spec.PerformanceInsightsRetentionPeriod
	}
	if db.Spec.MonitoringInterval != nil {
		input.MonitoringInterval = db.Spec.MonitoringInterval
	}
	if db.Spec.MonitoringRoleARN != "" {
		input.MonitoringRoleArn = aws.String(db.Spec.MonitoringRoleARN)
	}
	if len(db.Spec.EnabledCloudwatchLogsExports) > 0 {
		input.EnableCloudwatchLogsExports = db.Spec.EnabledCloudwatchLogsExports
	}
	if db.Spec.EnableIAMDatabaseAuthentication != nil {
		input.EnableIAMDatabaseAuthentication = db.Spec.EnableIAMDatabaseAuthentication
	}
	if db.Spec.CopyTagsToSnapshot != nil {
		input.CopyTagsToSnapshot = db.Spec.CopyTagsToSnapshot
	}
	if db.Spec.AutoMinorVersionUpgrade != nil {
		input.AutoMinorVersionUpgrade = db.Spec.AutoMinorVersionUpgrade
	}
	if db.Spec.Iops != nil {
		input.Iops = db.Spec.Iops
	}
	if db.Spec.StorageThroughput != nil {
		input.StorageThroughput = db.Spec.StorageThroughput
	}

	out, err := r.RDSClient.CreateDBInstance(ctx, input)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create DB instance: %w", err)
	}

	db.Status.DBInstanceARN = aws.ToString(out.DBInstance.DBInstanceArn)
	db.Status.DBInstanceStatus = aws.ToString(out.DBInstance.DBInstanceStatus)
	_ = r.setCondition(ctx, db, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "DB instance is being created")
	return requeueRDSPolling, nil
}

func (r *DBInstanceReconciler) updateDBInstance(ctx context.Context, db *awsv1alpha1.DBInstance) error {
	sgIDs, err := r.resolveSGIDs(ctx, db.Namespace, db.Spec.VPCSecurityGroupRefs)
	if err != nil {
		return err
	}
	input := &awsrds.ModifyDBInstanceInput{
		DBInstanceIdentifier: aws.String(db.Spec.DBInstanceIdentifier),
		DBInstanceClass:      aws.String(db.Spec.DBInstanceClass),
		AllocatedStorage:     aws.Int32(db.Spec.AllocatedStorage),
		MultiAZ:              aws.Bool(db.Spec.MultiAZ),
		DeletionProtection:   aws.Bool(db.Spec.DeletionProtection),
		ApplyImmediately:     aws.Bool(true),
	}
	if len(sgIDs) > 0 {
		input.VpcSecurityGroupIds = sgIDs
	}
	if db.Spec.BackupRetentionPeriod > 0 {
		input.BackupRetentionPeriod = aws.Int32(db.Spec.BackupRetentionPeriod)
	}
	if db.Spec.MaxAllocatedStorage != nil {
		input.MaxAllocatedStorage = db.Spec.MaxAllocatedStorage
	}
	if db.Spec.PreferredBackupWindow != "" {
		input.PreferredBackupWindow = aws.String(db.Spec.PreferredBackupWindow)
	}
	if db.Spec.PreferredMaintenanceWindow != "" {
		input.PreferredMaintenanceWindow = aws.String(db.Spec.PreferredMaintenanceWindow)
	}
	if db.Spec.EnablePerformanceInsights != nil {
		input.EnablePerformanceInsights = db.Spec.EnablePerformanceInsights
	}
	if db.Spec.PerformanceInsightsKMSKeyID != "" {
		input.PerformanceInsightsKMSKeyId = aws.String(db.Spec.PerformanceInsightsKMSKeyID)
	}
	if db.Spec.PerformanceInsightsRetentionPeriod != nil {
		input.PerformanceInsightsRetentionPeriod = db.Spec.PerformanceInsightsRetentionPeriod
	}
	if db.Spec.MonitoringInterval != nil {
		input.MonitoringInterval = db.Spec.MonitoringInterval
	}
	if db.Spec.MonitoringRoleARN != "" {
		input.MonitoringRoleArn = aws.String(db.Spec.MonitoringRoleARN)
	}
	if db.Spec.EnableIAMDatabaseAuthentication != nil {
		input.EnableIAMDatabaseAuthentication = db.Spec.EnableIAMDatabaseAuthentication
	}
	if db.Spec.CopyTagsToSnapshot != nil {
		input.CopyTagsToSnapshot = db.Spec.CopyTagsToSnapshot
	}
	if db.Spec.AutoMinorVersionUpgrade != nil {
		input.AutoMinorVersionUpgrade = db.Spec.AutoMinorVersionUpgrade
	}
	if db.Spec.Iops != nil {
		input.Iops = db.Spec.Iops
	}
	if db.Spec.StorageThroughput != nil {
		input.StorageThroughput = db.Spec.StorageThroughput
	}
	// Delta sync for CW logs exports.
	if len(db.Spec.EnabledCloudwatchLogsExports) > 0 {
		input.CloudwatchLogsExportConfiguration = &rdstypes.CloudwatchLogsExportConfiguration{
			EnableLogTypes: db.Spec.EnabledCloudwatchLogsExports,
		}
	}
	_, err = r.RDSClient.ModifyDBInstance(ctx, input)
	return err
}

func (r *DBInstanceReconciler) deleteDBInstance(ctx context.Context, db *awsv1alpha1.DBInstance) error {
	skipSnapshot := db.Spec.SkipFinalSnapshot
	input := &awsrds.DeleteDBInstanceInput{
		DBInstanceIdentifier: aws.String(db.Spec.DBInstanceIdentifier),
		SkipFinalSnapshot:    aws.Bool(skipSnapshot),
	}
	if !skipSnapshot {
		input.FinalDBSnapshotIdentifier = aws.String(db.Spec.DBInstanceIdentifier + "-final")
	}
	_, err := r.RDSClient.DeleteDBInstance(ctx, input)
	if rdshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *DBInstanceReconciler) resolveSecret(ctx context.Context, namespace string, ref awsv1alpha1.SecretRef) (string, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, secret); err != nil {
		return "", fmt.Errorf("get secret %s/%s: %w", namespace, ref.Name, err)
	}
	val, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("secret %s/%s has no key %q", namespace, ref.Name, ref.Key)
	}
	return string(val), nil
}

func (r *DBInstanceReconciler) resolveSGIDs(ctx context.Context, namespace string, refs []awsv1alpha1.SecurityGroupRef) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.ID != "" {
			ids = append(ids, ref.ID)
			continue
		}
		sgCR := &awsv1alpha1.SecurityGroup{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, sgCR); err != nil {
			return nil, err
		}
		if sgCR.Status.GroupID == "" {
			return nil, &dependencyNotReady{msg: fmt.Sprintf("SecurityGroup %s/%s has no ID yet", namespace, ref.Name)}
		}
		ids = append(ids, sgCR.Status.GroupID)
	}
	return ids, nil
}

func (r *DBInstanceReconciler) setCondition(ctx context.Context, db *awsv1alpha1.DBInstance, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&db.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: db.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, db); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

// rdsTagList converts RDS tags for ListTagsForResource output comparison.
func rdsTagsToMap(tags []rdstypes.Tag) map[string]string {
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return m
}

func (r *DBInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.DBInstance{}).
		Complete(r)
}
