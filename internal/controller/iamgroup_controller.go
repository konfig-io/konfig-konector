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

	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	iamhelper "github.com/konfig-io/konfig-konector/internal/aws/iam"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// IAMGroupReconciler reconciles IAMGroup objects.
type IAMGroupReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	IAMClient *multi.IAM
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamgroups/finalizers,verbs=update

func (r *IAMGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	grp := &awsv1alpha1.IAMGroup{}
	if err := r.Get(ctx, req.NamespacedName, grp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, grp); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !grp.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(grp, awsv1alpha1.FinalizerName) {
			if shouldAbandon(grp) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(grp, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, grp)
			}
			if err := iamhelper.DeleteGroup(ctx, r.IAMClient, grp.Spec.GroupName); err != nil {
				logger.Error(err, "failed to delete IAM group")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(grp, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, grp)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(grp, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(grp, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, grp); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileGroup(ctx, grp); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, grp, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *IAMGroupReconciler) reconcileGroup(ctx context.Context, grp *awsv1alpha1.IAMGroup) error {
	existing, err := iamhelper.GetGroup(ctx, r.IAMClient, grp.Spec.GroupName)
	if err != nil {
		return err
	}

	if existing == nil {
		path := grp.Spec.Path
		if path == "" {
			path = "/"
		}
		created, err := iamhelper.CreateGroup(ctx, r.IAMClient, &awsiam.CreateGroupInput{
			GroupName: aws.String(grp.Spec.GroupName),
			Path:      aws.String(path),
		})
		if err != nil {
			return err
		}
		existing = created
	}

	grp.Status.ARN = aws.ToString(existing.Arn)
	grp.Status.GroupID = aws.ToString(existing.GroupId)
	grp.Status.ObservedGeneration = grp.Generation
	now := metav1.Now()
	grp.Status.LastSyncTime = &now
	return r.setCondition(ctx, grp, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "IAM group synced")
}

func (r *IAMGroupReconciler) setCondition(ctx context.Context, grp *awsv1alpha1.IAMGroup, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&grp.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: grp.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, grp); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *IAMGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.IAMGroup{}).
		Complete(r)
}
