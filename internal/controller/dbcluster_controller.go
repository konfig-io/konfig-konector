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

// DBClusterReconciler reconciles DBCluster objects.
type DBClusterReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	RDSClient *awsrds.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=dbclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dbclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dbclusters/finalizers,verbs=update

func (r *DBClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	dbc := &awsv1alpha1.DBCluster{}
	if err := r.Get(ctx, req.NamespacedName, dbc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !dbc.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(dbc, awsv1alpha1.FinalizerName) {
			if shouldAbandon(dbc) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(dbc, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, dbc)
			}
			if err := r.deleteDBCluster(ctx, dbc); err != nil {
				logger.Error(err, "failed to delete DB cluster")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(dbc, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, dbc)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(dbc, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(dbc, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, dbc); err != nil {
			return ctrl.Result{}, err
		}
	}

	result, err := r.reconcileDBCluster(ctx, dbc)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, dbc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *DBClusterReconciler) reconcileDBCluster(ctx context.Context, dbc *awsv1alpha1.DBCluster) (ctrl.Result, error) {
	if dbc.Status.DBClusterARN != "" {
		out, err := r.RDSClient.DescribeDBClusters(ctx, &awsrds.DescribeDBClustersInput{
			DBClusterIdentifier: aws.String(dbc.Spec.DBClusterIdentifier),
		})
		if err != nil && !rdshelper.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err == nil && len(out.DBClusters) > 0 {
			cl := out.DBClusters[0]
			dbc.Status.Status = aws.ToString(cl.Status)
			dbc.Status.Endpoint = aws.ToString(cl.Endpoint)
			dbc.Status.ReaderEndpoint = aws.ToString(cl.ReaderEndpoint)

			if rdshelper.IsTransient(dbc.Status.Status) {
				_ = r.setCondition(ctx, dbc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning",
					fmt.Sprintf("DB cluster is %s", dbc.Status.Status))
				return requeueRDSPolling, nil
			}

			if dbc.Status.Status == "available" {
				if dbc.Status.ObservedGeneration != dbc.Generation {
					sgIDs, err := r.resolveSGIDs(ctx, dbc.Namespace, dbc.Spec.VPCSecurityGroupRefs)
					if err != nil {
						return ctrl.Result{}, err
					}
					modInput := &awsrds.ModifyDBClusterInput{
						DBClusterIdentifier: aws.String(dbc.Spec.DBClusterIdentifier),
						DeletionProtection:  aws.Bool(dbc.Spec.DeletionProtection),
						ApplyImmediately:    aws.Bool(true),
					}
					if len(sgIDs) > 0 {
						modInput.VpcSecurityGroupIds = sgIDs
					}
					if dbc.Spec.BackupRetentionPeriod > 0 {
						modInput.BackupRetentionPeriod = aws.Int32(dbc.Spec.BackupRetentionPeriod)
					}
					if dbc.Spec.PreferredBackupWindow != "" {
						modInput.PreferredBackupWindow = aws.String(dbc.Spec.PreferredBackupWindow)
					}
					if dbc.Spec.PreferredMaintenanceWindow != "" {
						modInput.PreferredMaintenanceWindow = aws.String(dbc.Spec.PreferredMaintenanceWindow)
					}
					if dbc.Spec.EnableIAMDatabaseAuthentication != nil {
						modInput.EnableIAMDatabaseAuthentication = dbc.Spec.EnableIAMDatabaseAuthentication
					}
					if dbc.Spec.CopyTagsToSnapshot != nil {
						modInput.CopyTagsToSnapshot = dbc.Spec.CopyTagsToSnapshot
					}
					if dbc.Spec.AutoMinorVersionUpgrade != nil {
						modInput.AutoMinorVersionUpgrade = dbc.Spec.AutoMinorVersionUpgrade
					}
					if dbc.Spec.EnableHttpEndpoint != nil {
						modInput.EnableHttpEndpoint = dbc.Spec.EnableHttpEndpoint
					}
					if dbc.Spec.ServerlessV2ScalingConfig != nil {
						modInput.ServerlessV2ScalingConfiguration = &rdstypes.ServerlessV2ScalingConfiguration{
							MinCapacity: aws.Float64(dbc.Spec.ServerlessV2ScalingConfig.MinCapacity),
							MaxCapacity: aws.Float64(dbc.Spec.ServerlessV2ScalingConfig.MaxCapacity),
						}
					}
					if dbc.Spec.BacktrackWindow != nil {
						modInput.BacktrackWindow = dbc.Spec.BacktrackWindow
					}
					if dbc.Spec.NetworkType != "" {
						modInput.NetworkType = aws.String(dbc.Spec.NetworkType)
					}
					if dbc.Spec.StorageType != "" {
						modInput.StorageType = aws.String(dbc.Spec.StorageType)
					}
					if dbc.Spec.Port != nil {
						modInput.Port = dbc.Spec.Port
					}
					if len(dbc.Spec.EnabledCloudwatchLogsExports) > 0 {
						modInput.CloudwatchLogsExportConfiguration = &rdstypes.CloudwatchLogsExportConfiguration{
							EnableLogTypes: dbc.Spec.EnabledCloudwatchLogsExports,
						}
					}
					if _, err := r.RDSClient.ModifyDBCluster(ctx, modInput); err != nil {
						return ctrl.Result{}, fmt.Errorf("modify DB cluster: %w", err)
					}
				}
				dbc.Status.ObservedGeneration = dbc.Generation
				now := metav1.Now()
				dbc.Status.LastSyncTime = &now
				return requeueResult(), r.setCondition(ctx, dbc, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "DB cluster available")
			}
		}
	}

	password, err := r.resolveSecret(ctx, dbc.Namespace, dbc.Spec.MasterUserPasswordRef)
	if err != nil {
		return ctrl.Result{}, err
	}

	sgIDs, err := r.resolveSGIDs(ctx, dbc.Namespace, dbc.Spec.VPCSecurityGroupRefs)
	if err != nil {
		return ctrl.Result{}, err
	}

	input := &awsrds.CreateDBClusterInput{
		DBClusterIdentifier: aws.String(dbc.Spec.DBClusterIdentifier),
		Engine:              aws.String(dbc.Spec.Engine),
		MasterUsername:      aws.String(dbc.Spec.MasterUsername),
		MasterUserPassword:  aws.String(password),
		StorageEncrypted:    aws.Bool(dbc.Spec.StorageEncrypted),
		DeletionProtection:  aws.Bool(dbc.Spec.DeletionProtection),
		Tags:                rdsTagsFromMap(dbc.Spec.Tags),
	}
	if dbc.Spec.EngineVersion != "" {
		input.EngineVersion = aws.String(dbc.Spec.EngineVersion)
	}
	if dbc.Spec.KMSKeyID != "" {
		input.KmsKeyId = aws.String(dbc.Spec.KMSKeyID)
	}
	if dbc.Spec.BackupRetentionPeriod > 0 {
		input.BackupRetentionPeriod = aws.Int32(dbc.Spec.BackupRetentionPeriod)
	}
	if len(sgIDs) > 0 {
		input.VpcSecurityGroupIds = sgIDs
	}
	if dbc.Spec.DBSubnetGroupRef != "" {
		snGroup := &awsv1alpha1.DBSubnetGroup{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: dbc.Spec.DBSubnetGroupRef, Namespace: dbc.Namespace}, snGroup); err != nil {
			return ctrl.Result{}, err
		}
		if snGroup.Spec.DBSubnetGroupName == "" {
			return ctrl.Result{}, &dependencyNotReady{msg: fmt.Sprintf("DBSubnetGroup %s/%s not ready", dbc.Namespace, dbc.Spec.DBSubnetGroupRef)}
		}
		input.DBSubnetGroupName = aws.String(snGroup.Spec.DBSubnetGroupName)
	}
	if dbc.Spec.DBClusterParameterGroupRef != "" {
		pg := &awsv1alpha1.DBClusterParameterGroup{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: dbc.Spec.DBClusterParameterGroupRef, Namespace: dbc.Namespace}, pg); err != nil {
			return ctrl.Result{}, err
		}
		if pg.Spec.DBClusterParameterGroupName == "" {
			return ctrl.Result{}, &dependencyNotReady{msg: fmt.Sprintf("DBClusterParameterGroup %s/%s not ready", dbc.Namespace, dbc.Spec.DBClusterParameterGroupRef)}
		}
		input.DBClusterParameterGroupName = aws.String(pg.Spec.DBClusterParameterGroupName)
	}

	if dbc.Spec.PreferredBackupWindow != "" {
		input.PreferredBackupWindow = aws.String(dbc.Spec.PreferredBackupWindow)
	}
	if dbc.Spec.PreferredMaintenanceWindow != "" {
		input.PreferredMaintenanceWindow = aws.String(dbc.Spec.PreferredMaintenanceWindow)
	}
	if len(dbc.Spec.EnabledCloudwatchLogsExports) > 0 {
		input.EnableCloudwatchLogsExports = dbc.Spec.EnabledCloudwatchLogsExports
	}
	if dbc.Spec.EnableIAMDatabaseAuthentication != nil {
		input.EnableIAMDatabaseAuthentication = dbc.Spec.EnableIAMDatabaseAuthentication
	}
	if dbc.Spec.CopyTagsToSnapshot != nil {
		input.CopyTagsToSnapshot = dbc.Spec.CopyTagsToSnapshot
	}
	if dbc.Spec.AutoMinorVersionUpgrade != nil {
		input.AutoMinorVersionUpgrade = dbc.Spec.AutoMinorVersionUpgrade
	}
	if dbc.Spec.EnableHttpEndpoint != nil {
		input.EnableHttpEndpoint = dbc.Spec.EnableHttpEndpoint
	}
	if dbc.Spec.EngineMode != "" {
		input.EngineMode = aws.String(dbc.Spec.EngineMode)
	}
	if dbc.Spec.ServerlessV2ScalingConfig != nil {
		input.ServerlessV2ScalingConfiguration = &rdstypes.ServerlessV2ScalingConfiguration{
			MinCapacity: aws.Float64(dbc.Spec.ServerlessV2ScalingConfig.MinCapacity),
			MaxCapacity: aws.Float64(dbc.Spec.ServerlessV2ScalingConfig.MaxCapacity),
		}
	}
	if dbc.Spec.BacktrackWindow != nil {
		input.BacktrackWindow = dbc.Spec.BacktrackWindow
	}
	if dbc.Spec.NetworkType != "" {
		input.NetworkType = aws.String(dbc.Spec.NetworkType)
	}
	if dbc.Spec.PerformanceInsightsEnabled != nil {
		input.EnablePerformanceInsights = dbc.Spec.PerformanceInsightsEnabled
	}
	if dbc.Spec.PerformanceInsightsKMSKeyID != "" {
		input.PerformanceInsightsKMSKeyId = aws.String(dbc.Spec.PerformanceInsightsKMSKeyID)
	}
	if dbc.Spec.PerformanceInsightsRetentionPeriod != nil {
		input.PerformanceInsightsRetentionPeriod = dbc.Spec.PerformanceInsightsRetentionPeriod
	}
	if dbc.Spec.AllocatedStorage != nil {
		input.AllocatedStorage = dbc.Spec.AllocatedStorage
	}
	if dbc.Spec.StorageType != "" {
		input.StorageType = aws.String(dbc.Spec.StorageType)
	}
	if dbc.Spec.Port != nil {
		input.Port = dbc.Spec.Port
	}
	out, err := r.RDSClient.CreateDBCluster(ctx, input)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create DB cluster: %w", err)
	}

	dbc.Status.DBClusterARN = aws.ToString(out.DBCluster.DBClusterArn)
	dbc.Status.Status = aws.ToString(out.DBCluster.Status)
	_ = r.setCondition(ctx, dbc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "DB cluster is being created")
	return requeueRDSPolling, nil
}

func (r *DBClusterReconciler) deleteDBCluster(ctx context.Context, dbc *awsv1alpha1.DBCluster) error {
	input := &awsrds.DeleteDBClusterInput{
		DBClusterIdentifier: aws.String(dbc.Spec.DBClusterIdentifier),
		SkipFinalSnapshot:   aws.Bool(dbc.Spec.SkipFinalSnapshot),
	}
	if !dbc.Spec.SkipFinalSnapshot {
		input.FinalDBSnapshotIdentifier = aws.String(dbc.Spec.DBClusterIdentifier + "-final")
	}
	_, err := r.RDSClient.DeleteDBCluster(ctx, input)
	if rdshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *DBClusterReconciler) resolveSecret(ctx context.Context, namespace string, ref awsv1alpha1.SecretRef) (string, error) {
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

func (r *DBClusterReconciler) resolveSGIDs(ctx context.Context, namespace string, refs []awsv1alpha1.SecurityGroupRef) ([]string, error) {
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

func (r *DBClusterReconciler) setCondition(ctx context.Context, dbc *awsv1alpha1.DBCluster, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&dbc.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: dbc.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, dbc); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *DBClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.DBCluster{}).
		Complete(r)
}
