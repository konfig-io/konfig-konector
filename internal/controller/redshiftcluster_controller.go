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
	awsredshift "github.com/aws/aws-sdk-go-v2/service/redshift"
	redshifttypes "github.com/aws/aws-sdk-go-v2/service/redshift/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	redshifthelper "github.com/konfig-io/konfig-konector/internal/aws/redshift"
)

// requeueRedshiftPolling is the family-level poll interval for async Redshift
// cluster state transitions.
var requeueRedshiftPolling = ctrl.Result{RequeueAfter: 30 * time.Second}

// RedshiftClusterAWSAPI is the subset of the Redshift SDK client used by this
// controller. *awsredshift.Client satisfies it.
type RedshiftClusterAWSAPI interface {
	DescribeClusters(ctx context.Context, params *awsredshift.DescribeClustersInput, optFns ...func(*awsredshift.Options)) (*awsredshift.DescribeClustersOutput, error)
	CreateCluster(ctx context.Context, params *awsredshift.CreateClusterInput, optFns ...func(*awsredshift.Options)) (*awsredshift.CreateClusterOutput, error)
	ModifyCluster(ctx context.Context, params *awsredshift.ModifyClusterInput, optFns ...func(*awsredshift.Options)) (*awsredshift.ModifyClusterOutput, error)
	DeleteCluster(ctx context.Context, params *awsredshift.DeleteClusterInput, optFns ...func(*awsredshift.Options)) (*awsredshift.DeleteClusterOutput, error)
}

// RedshiftClusterReconciler reconciles RedshiftCluster objects.
type RedshiftClusterReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	RedshiftClient RedshiftClusterAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=redshiftclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=redshiftclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=redshiftclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

func (r *RedshiftClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	rc := &awsv1alpha1.RedshiftCluster{}
	if err := r.Get(ctx, req.NamespacedName, rc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, rc); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !rc.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(rc, awsv1alpha1.FinalizerName) {
			if shouldAbandon(rc) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(rc, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, rc)
			}
			if err := r.deleteCluster(ctx, rc); err != nil {
				logger.Error(err, "failed to delete Redshift cluster")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(rc, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, rc)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(rc, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(rc, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, rc); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	result, err := r.reconcileCluster(ctx, rc)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, rc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

// resolveSubnetGroupName resolves the cluster subnet group from a direct name
// or a RedshiftSubnetGroup CR reference.
func (r *RedshiftClusterReconciler) resolveSubnetGroupName(ctx context.Context, rc *awsv1alpha1.RedshiftCluster) (string, error) {
	if rc.Spec.ClusterSubnetGroupName != "" {
		return rc.Spec.ClusterSubnetGroupName, nil
	}
	if rc.Spec.ClusterSubnetGroupRef == "" {
		return "", nil
	}
	sg := &awsv1alpha1.RedshiftSubnetGroup{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: rc.Spec.ClusterSubnetGroupRef, Namespace: rc.Namespace}, sg); err != nil {
		return "", err
	}
	if sg.Status.SubnetGroupName == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("RedshiftSubnetGroup %s/%s not synced yet", rc.Namespace, rc.Spec.ClusterSubnetGroupRef)}
	}
	return sg.Status.SubnetGroupName, nil
}

func redshiftTags(tags map[string]string) []redshifttypes.Tag {
	if len(tags) == 0 {
		return nil
	}
	out := make([]redshifttypes.Tag, 0, len(tags))
	for k, v := range tags {
		out = append(out, redshifttypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	return out
}

func (r *RedshiftClusterReconciler) reconcileCluster(ctx context.Context, rc *awsv1alpha1.RedshiftCluster) (ctrl.Result, error) {
	// Check current state if we already created the cluster.
	if rc.Status.ClusterIdentifier != "" {
		out, err := r.RedshiftClient.DescribeClusters(ctx, &awsredshift.DescribeClustersInput{
			ClusterIdentifier: aws.String(rc.Spec.ClusterIdentifier),
		})
		if err != nil && !redshifthelper.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err == nil && len(out.Clusters) > 0 {
			cluster := out.Clusters[0]
			rc.Status.ClusterStatus = aws.ToString(cluster.ClusterStatus)
			if cluster.Endpoint != nil {
				rc.Status.Endpoint = aws.ToString(cluster.Endpoint.Address)
				rc.Status.Port = aws.ToInt32(cluster.Endpoint.Port)
			}

			if redshifthelper.IsTransientClusterStatus(rc.Status.ClusterStatus) {
				_ = r.setCondition(ctx, rc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning",
					fmt.Sprintf("Redshift cluster is %s", rc.Status.ClusterStatus))
				return requeueRedshiftPolling, nil
			}

			if rc.Status.ClusterStatus == "available" {
				if rc.Status.ObservedGeneration != rc.Generation {
					if err := r.modifyCluster(ctx, rc); err != nil {
						return ctrl.Result{}, err
					}
				}
				rc.Status.ObservedGeneration = rc.Generation
				now := metav1.Now()
				rc.Status.LastSyncTime = &now
				return requeueResult(), r.setCondition(ctx, rc, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Redshift cluster available")
			}
		}
	}

	// Create a new cluster. The password is resolved from the Secret and is
	// never written to status, conditions, or logs.
	password, err := resolveSecretValue(ctx, r.Client, rc.Namespace, rc.Spec.MasterUserPasswordRef)
	if err != nil {
		return ctrl.Result{}, err
	}
	sgIDs, err := resolveSGIDs(ctx, r.Client, rc.Namespace, rc.Spec.VPCSecurityGroupRefs)
	if err != nil {
		return ctrl.Result{}, err
	}
	subnetGroup, err := r.resolveSubnetGroupName(ctx, rc)
	if err != nil {
		return ctrl.Result{}, err
	}

	input := &awsredshift.CreateClusterInput{
		ClusterIdentifier:  aws.String(rc.Spec.ClusterIdentifier),
		NodeType:           aws.String(rc.Spec.NodeType),
		MasterUsername:     aws.String(rc.Spec.MasterUsername),
		MasterUserPassword: aws.String(password),
		Encrypted:          aws.Bool(rc.Spec.Encrypted),
		PubliclyAccessible: aws.Bool(rc.Spec.PubliclyAccessible),
		Tags:               redshiftTags(rc.Spec.Tags),
	}
	if rc.Spec.NumberOfNodes > 1 {
		input.ClusterType = aws.String("multi-node")
		input.NumberOfNodes = aws.Int32(rc.Spec.NumberOfNodes)
	} else {
		input.ClusterType = aws.String("single-node")
	}
	if rc.Spec.DBName != "" {
		input.DBName = aws.String(rc.Spec.DBName)
	}
	if subnetGroup != "" {
		input.ClusterSubnetGroupName = aws.String(subnetGroup)
	}
	if len(sgIDs) > 0 {
		input.VpcSecurityGroupIds = sgIDs
	}
	if rc.Spec.KMSKeyID != "" {
		input.KmsKeyId = aws.String(rc.Spec.KMSKeyID)
	}

	out, err := r.RedshiftClient.CreateCluster(ctx, input)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create Redshift cluster: %w", err)
	}

	// Persist the identifier immediately: the AWS resource now exists.
	rc.Status.ClusterIdentifier = rc.Spec.ClusterIdentifier
	if out.Cluster != nil {
		rc.Status.ClusterStatus = aws.ToString(out.Cluster.ClusterStatus)
	}
	if err := persistStatus(ctx, r.Client, rc); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist cluster identifier after create: %w", err)
	}
	_ = r.setCondition(ctx, rc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "Redshift cluster is being created")
	return requeueRedshiftPolling, nil
}

func (r *RedshiftClusterReconciler) modifyCluster(ctx context.Context, rc *awsv1alpha1.RedshiftCluster) error {
	sgIDs, err := resolveSGIDs(ctx, r.Client, rc.Namespace, rc.Spec.VPCSecurityGroupRefs)
	if err != nil {
		return err
	}
	input := &awsredshift.ModifyClusterInput{
		ClusterIdentifier:  aws.String(rc.Spec.ClusterIdentifier),
		NodeType:           aws.String(rc.Spec.NodeType),
		PubliclyAccessible: aws.Bool(rc.Spec.PubliclyAccessible),
	}
	if rc.Spec.NumberOfNodes > 1 {
		input.ClusterType = aws.String("multi-node")
		input.NumberOfNodes = aws.Int32(rc.Spec.NumberOfNodes)
	} else {
		input.ClusterType = aws.String("single-node")
	}
	if len(sgIDs) > 0 {
		input.VpcSecurityGroupIds = sgIDs
	}
	_, err = r.RedshiftClient.ModifyCluster(ctx, input)
	return err
}

func (r *RedshiftClusterReconciler) deleteCluster(ctx context.Context, rc *awsv1alpha1.RedshiftCluster) error {
	skipSnapshot := rc.Spec.SkipFinalClusterSnapshot
	input := &awsredshift.DeleteClusterInput{
		ClusterIdentifier:        aws.String(rc.Spec.ClusterIdentifier),
		SkipFinalClusterSnapshot: aws.Bool(skipSnapshot),
	}
	if !skipSnapshot {
		input.FinalClusterSnapshotIdentifier = aws.String(rc.Spec.ClusterIdentifier + "-final")
	}
	_, err := r.RedshiftClient.DeleteCluster(ctx, input)
	if redshifthelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *RedshiftClusterReconciler) setCondition(ctx context.Context, rc *awsv1alpha1.RedshiftCluster, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&rc.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: rc.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, rc); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *RedshiftClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.RedshiftCluster{}).
		Complete(r)
}
