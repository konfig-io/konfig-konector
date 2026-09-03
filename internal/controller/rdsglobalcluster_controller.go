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

// RDSGlobalClusterAWSAPI is the subset of the RDS API used by this controller.
type RDSGlobalClusterAWSAPI interface {
	DescribeGlobalClusters(ctx context.Context, params *awsrds.DescribeGlobalClustersInput, optFns ...func(*awsrds.Options)) (*awsrds.DescribeGlobalClustersOutput, error)
	CreateGlobalCluster(ctx context.Context, params *awsrds.CreateGlobalClusterInput, optFns ...func(*awsrds.Options)) (*awsrds.CreateGlobalClusterOutput, error)
	DeleteGlobalCluster(ctx context.Context, params *awsrds.DeleteGlobalClusterInput, optFns ...func(*awsrds.Options)) (*awsrds.DeleteGlobalClusterOutput, error)
}

// RDSGlobalClusterReconciler reconciles RDSGlobalCluster objects.
type RDSGlobalClusterReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	RDSClient RDSGlobalClusterAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=rdsglobalclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=rdsglobalclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=rdsglobalclusters/finalizers,verbs=update

func (r *RDSGlobalClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	gc := &awsv1alpha1.RDSGlobalCluster{}
	if err := r.Get(ctx, req.NamespacedName, gc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !gc.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(gc, awsv1alpha1.FinalizerName) {
			if shouldAbandon(gc) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(gc, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, gc)
			}
			if err := r.deleteCluster(ctx, gc); err != nil {
				logger.Error(err, "failed to delete global cluster")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(gc, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, gc)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(gc, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(gc, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, gc); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileCluster(ctx, gc); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, gc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *RDSGlobalClusterReconciler) reconcileCluster(ctx context.Context, gc *awsv1alpha1.RDSGlobalCluster) error {
	descOut, err := r.RDSClient.DescribeGlobalClusters(ctx, &awsrds.DescribeGlobalClustersInput{
		GlobalClusterIdentifier: aws.String(gc.Spec.GlobalClusterIdentifier),
	})
	if err != nil && !rdshelper.IsNotFound(err) {
		return err
	}

	if rdshelper.IsNotFound(err) || len(descOut.GlobalClusters) == 0 {
		input := &awsrds.CreateGlobalClusterInput{
			GlobalClusterIdentifier: aws.String(gc.Spec.GlobalClusterIdentifier),
		}
		if gc.Spec.SourceDBClusterIdentifier != "" {
			// Engine settings are inherited from the source cluster; AWS
			// rejects the call if both source and engine are set.
			input.SourceDBClusterIdentifier = aws.String(gc.Spec.SourceDBClusterIdentifier)
		} else {
			if gc.Spec.Engine != "" {
				input.Engine = aws.String(gc.Spec.Engine)
			}
			if gc.Spec.EngineVersion != "" {
				input.EngineVersion = aws.String(gc.Spec.EngineVersion)
			}
			if gc.Spec.StorageEncrypted {
				input.StorageEncrypted = aws.Bool(true)
			}
		}
		input.DeletionProtection = aws.Bool(gc.Spec.DeletionProtection)
		createOut, err := r.RDSClient.CreateGlobalCluster(ctx, input)
		if err != nil {
			return fmt.Errorf("create global cluster: %w", err)
		}
		if createOut.GlobalCluster != nil {
			gc.Status.ARN = aws.ToString(createOut.GlobalCluster.GlobalClusterArn)
			gc.Status.Status = aws.ToString(createOut.GlobalCluster.Status)
		}
		if err := persistStatus(ctx, r.Client, gc); err != nil {
			return fmt.Errorf("persist global cluster ARN after create: %w", err)
		}
	} else {
		existing := descOut.GlobalClusters[0]
		gc.Status.ARN = aws.ToString(existing.GlobalClusterArn)
		gc.Status.Status = aws.ToString(existing.Status)
	}

	gc.Status.ObservedGeneration = gc.Generation
	now := metav1.Now()
	gc.Status.LastSyncTime = &now
	return r.setCondition(ctx, gc, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "global cluster reconciled")
}

func (r *RDSGlobalClusterReconciler) deleteCluster(ctx context.Context, gc *awsv1alpha1.RDSGlobalCluster) error {
	// The global cluster is identified by its spec identifier.
	_, err := r.RDSClient.DeleteGlobalCluster(ctx, &awsrds.DeleteGlobalClusterInput{
		GlobalClusterIdentifier: aws.String(gc.Spec.GlobalClusterIdentifier),
	})
	if rdshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *RDSGlobalClusterReconciler) setCondition(ctx context.Context, gc *awsv1alpha1.RDSGlobalCluster, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&gc.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: gc.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, gc); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *RDSGlobalClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.RDSGlobalCluster{}).
		Complete(r)
}
