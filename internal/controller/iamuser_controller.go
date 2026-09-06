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

// IAMUserReconciler reconciles IAMUser objects.
type IAMUserReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	IAMClient *multi.IAM
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamusers/finalizers,verbs=update

func (r *IAMUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	user := &awsv1alpha1.IAMUser{}
	if err := r.Get(ctx, req.NamespacedName, user); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, user); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !user.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(user, awsv1alpha1.FinalizerName) {
			if shouldAbandon(user) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(user, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, user)
			}
			if err := iamhelper.DeleteUser(ctx, r.IAMClient, user.Spec.UserName); err != nil {
				logger.Error(err, "failed to delete IAM user")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(user, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, user)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(user, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(user, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, user); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileUser(ctx, user); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, user, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *IAMUserReconciler) reconcileUser(ctx context.Context, user *awsv1alpha1.IAMUser) error {
	existing, err := iamhelper.GetUser(ctx, r.IAMClient, user.Spec.UserName)
	if err != nil {
		return err
	}

	if existing == nil {
		var boundary *string
		if user.Spec.PermissionsBoundary != "" {
			boundary = aws.String(user.Spec.PermissionsBoundary)
		}
		path := user.Spec.Path
		if path == "" {
			path = "/"
		}
		created, err := iamhelper.CreateUser(ctx, r.IAMClient, &awsiam.CreateUserInput{
			UserName:            aws.String(user.Spec.UserName),
			Path:                aws.String(path),
			PermissionsBoundary: boundary,
		})
		if err != nil {
			return err
		}
		existing = created
	}

	if user.Status.ObservedGeneration != user.Generation {
		path := user.Spec.Path
		if path == "" {
			path = "/"
		}
		if err := iamhelper.UpdateUser(ctx, r.IAMClient, user.Spec.UserName, path); err != nil {
			return err
		}
	}

	if err := iamhelper.SyncUserTags(ctx, r.IAMClient, user.Spec.UserName, user.Spec.Tags); err != nil {
		return err
	}

	user.Status.ARN = aws.ToString(existing.Arn)
	user.Status.UserID = aws.ToString(existing.UserId)
	user.Status.ObservedGeneration = user.Generation
	now := metav1.Now()
	user.Status.LastSyncTime = &now
	return r.setCondition(ctx, user, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "IAM user synced")
}

func (r *IAMUserReconciler) setCondition(ctx context.Context, user *awsv1alpha1.IAMUser, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&user.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: user.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, user); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *IAMUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.IAMUser{}).
		Complete(r)
}
