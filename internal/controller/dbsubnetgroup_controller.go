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
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
	rdshelper "github.com/konfig-io/konfig-konector/internal/aws/rds"
)

// DBSubnetGroupReconciler reconciles DBSubnetGroup objects.
type DBSubnetGroupReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	RDSClient *multi.RDS
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=dbsubnetgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dbsubnetgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dbsubnetgroups/finalizers,verbs=update

func (r *DBSubnetGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	sg := &awsv1alpha1.DBSubnetGroup{}
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
			if _, err := r.RDSClient.DeleteDBSubnetGroup(ctx, &awsrds.DeleteDBSubnetGroupInput{
				DBSubnetGroupName: aws.String(sg.Spec.DBSubnetGroupName),
			}); err != nil && !rdshelper.IsNotFound(err) {
				logger.Error(err, "failed to delete DB subnet group")
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

	if err := r.reconcileDBSubnetGroup(ctx, sg); err != nil {
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

func (r *DBSubnetGroupReconciler) reconcileDBSubnetGroup(ctx context.Context, sg *awsv1alpha1.DBSubnetGroup) error {
	// Resolve subnet IDs.
	subnetIDs, err := r.resolveSubnetIDs(ctx, sg.Namespace, sg.Spec.SubnetRefs)
	if err != nil {
		return err
	}

	// Check if group already exists.
	out, err := r.RDSClient.DescribeDBSubnetGroups(ctx, &awsrds.DescribeDBSubnetGroupsInput{
		DBSubnetGroupName: aws.String(sg.Spec.DBSubnetGroupName),
	})
	if err != nil && !rdshelper.IsNotFound(err) {
		return err
	}

	tags := rdsTagsFromMap(sg.Spec.Tags)

	if rdshelper.IsNotFound(err) || len(out.DBSubnetGroups) == 0 {
		createOut, err := r.RDSClient.CreateDBSubnetGroup(ctx, &awsrds.CreateDBSubnetGroupInput{
			DBSubnetGroupName:        aws.String(sg.Spec.DBSubnetGroupName),
			DBSubnetGroupDescription: aws.String(sg.Spec.Description),
			SubnetIds:                subnetIDs,
			Tags:                     tags,
		})
		if err != nil {
			return fmt.Errorf("create DB subnet group: %w", err)
		}
		sg.Status.ARN = aws.ToString(createOut.DBSubnetGroup.DBSubnetGroupArn)
		sg.Status.Status = aws.ToString(createOut.DBSubnetGroup.SubnetGroupStatus)
	} else {
		existing := out.DBSubnetGroups[0]
		sg.Status.ARN = aws.ToString(existing.DBSubnetGroupArn)
		sg.Status.Status = aws.ToString(existing.SubnetGroupStatus)

		// Update subnets if changed.
		if _, err := r.RDSClient.ModifyDBSubnetGroup(ctx, &awsrds.ModifyDBSubnetGroupInput{
			DBSubnetGroupName:        aws.String(sg.Spec.DBSubnetGroupName),
			DBSubnetGroupDescription: aws.String(sg.Spec.Description),
			SubnetIds:                subnetIDs,
		}); err != nil {
			return fmt.Errorf("modify DB subnet group: %w", err)
		}
	}

	sg.Status.ObservedGeneration = sg.Generation
	now := metav1.Now()
	sg.Status.LastSyncTime = &now
	return r.setCondition(ctx, sg, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "DB subnet group reconciled")
}

func (r *DBSubnetGroupReconciler) resolveSubnetIDs(ctx context.Context, namespace string, refs []awsv1alpha1.SubnetRef) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.ID != "" {
			ids = append(ids, ref.ID)
			continue
		}
		sn := &awsv1alpha1.Subnet{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, sn); err != nil {
			return nil, err
		}
		if sn.Status.SubnetID == "" {
			return nil, &dependencyNotReady{msg: fmt.Sprintf("Subnet %s/%s has no ID yet", namespace, ref.Name)}
		}
		ids = append(ids, sn.Status.SubnetID)
	}
	return ids, nil
}

func (r *DBSubnetGroupReconciler) setCondition(ctx context.Context, sg *awsv1alpha1.DBSubnetGroup, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *DBSubnetGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.DBSubnetGroup{}).
		Complete(r)
}

// rdsTagsFromMap converts a map to RDS Tag slice.
func rdsTagsFromMap(m map[string]string) []rdstypes.Tag {
	tags := make([]rdstypes.Tag, 0, len(m))
	for k, v := range m {
		k, v := k, v
		tags = append(tags, rdstypes.Tag{Key: &k, Value: &v})
	}
	return tags
}
