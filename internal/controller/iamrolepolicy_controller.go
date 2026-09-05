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
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	iamhelper "github.com/konfig-io/konfig-konector/internal/aws/iam"
)

// IAMRolePolicyAWSAPI is the subset of the IAM SDK client used by this
// controller (directly and via iamhelper.GetInlinePolicy). *iam.Client satisfies it.
type IAMRolePolicyAWSAPI interface {
	iamhelper.InlinePolicyAPI
	PutRolePolicy(ctx context.Context, params *awsiam.PutRolePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.PutRolePolicyOutput, error)
	DeleteRolePolicy(ctx context.Context, params *awsiam.DeleteRolePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.DeleteRolePolicyOutput, error)
}

// IAMRolePolicyReconciler reconciles IAMRolePolicy objects (inline policies).
type IAMRolePolicyReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	IAMClient IAMRolePolicyAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamrolepolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamrolepolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamrolepolicies/finalizers,verbs=update

func (r *IAMRolePolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	rp := &awsv1alpha1.IAMRolePolicy{}
	if err := r.Get(ctx, req.NamespacedName, rp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, rp); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !rp.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(rp, awsv1alpha1.FinalizerName) {
			if shouldAbandon(rp) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(rp, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, rp)
			}
			roleName := rp.Spec.RoleRef.Name
			if rp.Status.RoleARN != "" {
				roleName = roleNameFromARN(rp.Status.RoleARN)
			}
			if _, err := r.IAMClient.DeleteRolePolicy(ctx, &awsiam.DeleteRolePolicyInput{
				RoleName:   aws.String(roleName),
				PolicyName: aws.String(rp.Spec.PolicyName),
			}); err != nil && !iamhelper.IsNotFound(err) {
				logger.Error(err, "failed to delete inline policy")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(rp, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, rp)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(rp, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(rp, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, rp); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileInlinePolicy(ctx, rp); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setRPCondition(ctx, rp, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *IAMRolePolicyReconciler) reconcileInlinePolicy(ctx context.Context, rp *awsv1alpha1.IAMRolePolicy) error {
	roleARN, err := r.resolveRoleARNForRP(ctx, rp)
	if err != nil {
		return err
	}
	roleName := roleNameFromARN(roleARN)
	rp.Status.RoleARN = roleARN

	current, err := iamhelper.GetInlinePolicy(ctx, r.IAMClient, roleName, rp.Spec.PolicyName)
	if err != nil {
		return err
	}

	decoded, _ := url.QueryUnescape(current)
	if decoded == rp.Spec.PolicyDocument {
		// No change needed.
	} else {
		if _, err := r.IAMClient.PutRolePolicy(ctx, &awsiam.PutRolePolicyInput{
			RoleName:       aws.String(roleName),
			PolicyName:     aws.String(rp.Spec.PolicyName),
			PolicyDocument: aws.String(rp.Spec.PolicyDocument),
		}); err != nil {
			return fmt.Errorf("put role policy: %w", err)
		}
	}

	rp.Status.ObservedGeneration = rp.Generation
	now := metav1.Now()
	rp.Status.LastSyncTime = &now
	return r.setRPCondition(ctx, rp, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "inline policy reconciled")
}

func (r *IAMRolePolicyReconciler) resolveRoleARNForRP(ctx context.Context, rp *awsv1alpha1.IAMRolePolicy) (string, error) {
	if rp.Spec.RoleRef.ARN != "" {
		return rp.Spec.RoleRef.ARN, nil
	}
	ns := rp.Spec.RoleRef.Namespace
	if ns == "" {
		ns = rp.Namespace
	}
	role := &awsv1alpha1.IAMRole{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: rp.Spec.RoleRef.Name, Namespace: ns}, role); err != nil {
		return "", err
	}
	if role.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("IAMRole %s/%s has no ARN yet", ns, rp.Spec.RoleRef.Name)}
	}
	return role.Status.ARN, nil
}

func (r *IAMRolePolicyReconciler) setRPCondition(ctx context.Context, rp *awsv1alpha1.IAMRolePolicy, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&rp.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: rp.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, rp); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *IAMRolePolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.IAMRolePolicy{}).
		Complete(r)
}
