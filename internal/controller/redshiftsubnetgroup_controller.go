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

// RedshiftSubnetGroupAWSAPI is the subset of the Redshift SDK client used by
// this controller. *awsredshift.Client satisfies it.
type RedshiftSubnetGroupAWSAPI interface {
	DescribeClusterSubnetGroups(ctx context.Context, params *awsredshift.DescribeClusterSubnetGroupsInput, optFns ...func(*awsredshift.Options)) (*awsredshift.DescribeClusterSubnetGroupsOutput, error)
	CreateClusterSubnetGroup(ctx context.Context, params *awsredshift.CreateClusterSubnetGroupInput, optFns ...func(*awsredshift.Options)) (*awsredshift.CreateClusterSubnetGroupOutput, error)
	ModifyClusterSubnetGroup(ctx context.Context, params *awsredshift.ModifyClusterSubnetGroupInput, optFns ...func(*awsredshift.Options)) (*awsredshift.ModifyClusterSubnetGroupOutput, error)
	DeleteClusterSubnetGroup(ctx context.Context, params *awsredshift.DeleteClusterSubnetGroupInput, optFns ...func(*awsredshift.Options)) (*awsredshift.DeleteClusterSubnetGroupOutput, error)
}

// RedshiftSubnetGroupReconciler reconciles RedshiftSubnetGroup objects.
type RedshiftSubnetGroupReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	RedshiftClient RedshiftSubnetGroupAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=redshiftsubnetgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=redshiftsubnetgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=redshiftsubnetgroups/finalizers,verbs=update

func (r *RedshiftSubnetGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	sg := &awsv1alpha1.RedshiftSubnetGroup{}
	if err := r.Get(ctx, req.NamespacedName, sg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, sg); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !sg.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(sg, awsv1alpha1.FinalizerName) {
			if shouldAbandon(sg) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(sg, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, sg)
			}
			if err := r.deleteSubnetGroup(ctx, sg); err != nil {
				logger.Error(err, "failed to delete Redshift subnet group")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(sg, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, sg)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(sg, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(sg, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, sg); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileSubnetGroup(ctx, sg); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, sg, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *RedshiftSubnetGroupReconciler) reconcileSubnetGroup(ctx context.Context, sg *awsv1alpha1.RedshiftSubnetGroup) error {
	subnetIDs, err := resolveSubnetIDs(ctx, r.Client, sg.Namespace, sg.Spec.SubnetRefs)
	if err != nil {
		return err
	}

	out, err := r.RedshiftClient.DescribeClusterSubnetGroups(ctx, &awsredshift.DescribeClusterSubnetGroupsInput{
		ClusterSubnetGroupName: aws.String(sg.Spec.Name),
	})
	if redshifthelper.IsNotFound(err) {
		if _, err := r.RedshiftClient.CreateClusterSubnetGroup(ctx, &awsredshift.CreateClusterSubnetGroupInput{
			ClusterSubnetGroupName: aws.String(sg.Spec.Name),
			Description:            aws.String(sg.Spec.Description),
			SubnetIds:              subnetIDs,
			Tags:                   redshiftTags(sg.Spec.Tags),
		}); err != nil {
			return fmt.Errorf("create Redshift subnet group: %w", err)
		}
		// Persist the identifier immediately: the AWS resource now exists.
		sg.Status.SubnetGroupName = sg.Spec.Name
		if err := persistStatus(ctx, r.Client, sg); err != nil {
			return fmt.Errorf("persist subnet group name after create: %w", err)
		}
	} else if err != nil {
		return err
	} else {
		if len(out.ClusterSubnetGroups) > 0 {
			sg.Status.VPCID = aws.ToString(out.ClusterSubnetGroups[0].VpcId)
		}
		if sg.Status.ObservedGeneration != sg.Generation {
			if _, err := r.RedshiftClient.ModifyClusterSubnetGroup(ctx, &awsredshift.ModifyClusterSubnetGroupInput{
				ClusterSubnetGroupName: aws.String(sg.Spec.Name),
				SubnetIds:              subnetIDs,
				Description:            aws.String(sg.Spec.Description),
			}); err != nil {
				return fmt.Errorf("modify Redshift subnet group: %w", err)
			}
		}
	}

	sg.Status.SubnetGroupName = sg.Spec.Name
	sg.Status.ObservedGeneration = sg.Generation
	now := metav1.Now()
	sg.Status.LastSyncTime = &now
	return r.setCondition(ctx, sg, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Redshift subnet group reconciled")
}

func (r *RedshiftSubnetGroupReconciler) deleteSubnetGroup(ctx context.Context, sg *awsv1alpha1.RedshiftSubnetGroup) error {
	name := sg.Status.SubnetGroupName
	if name == "" {
		name = sg.Spec.Name
	}
	_, err := r.RedshiftClient.DeleteClusterSubnetGroup(ctx, &awsredshift.DeleteClusterSubnetGroupInput{
		ClusterSubnetGroupName: aws.String(name),
	})
	if redshifthelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *RedshiftSubnetGroupReconciler) setCondition(ctx context.Context, sg *awsv1alpha1.RedshiftSubnetGroup, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&sg.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: sg.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, sg); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *RedshiftSubnetGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.RedshiftSubnetGroup{}).
		Complete(r)
}
