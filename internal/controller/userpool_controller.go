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
	awscognito "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	cognitotypes "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	cognitohelper "github.com/konfig-io/konfig-konector/internal/aws/cognito"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// UserPoolReconciler reconciles UserPool objects.
type UserPoolReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	CognitoClient *multi.Cognito
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=userpools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=userpools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=userpools/finalizers,verbs=update

func (r *UserPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	up := &awsv1alpha1.UserPool{}
	if err := r.Get(ctx, req.NamespacedName, up); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, up); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !up.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(up, awsv1alpha1.FinalizerName) {
			if shouldAbandon(up) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(up, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, up)
			}
			if err := r.deleteUserPool(ctx, up); err != nil {
				logger.Error(err, "failed to delete UserPool")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(up, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, up)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(up, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(up, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, up); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileUserPool(ctx, up); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionUP(ctx, up, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *UserPoolReconciler) reconcileUserPool(ctx context.Context, up *awsv1alpha1.UserPool) error {
	if up.Status.UserPoolID != "" {
		out, err := r.CognitoClient.DescribeUserPool(ctx, &awscognito.DescribeUserPoolInput{
			UserPoolId: aws.String(up.Status.UserPoolID),
		})
		if err != nil && !cognitohelper.IsNotFound(err) {
			return fmt.Errorf("describe user pool: %w", err)
		}
		if err == nil {
			up.Status.ARN = aws.ToString(out.UserPool.Arn)
			up.Status.ObservedGeneration = up.Generation
			now := metav1.Now()
			up.Status.LastSyncTime = &now
			return r.setConditionUP(ctx, up, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "UserPool reconciled")
		}
		up.Status.UserPoolID = ""
		up.Status.ARN = ""
	}

	input := &awscognito.CreateUserPoolInput{
		PoolName: aws.String(up.Spec.PoolName),
	}
	if len(up.Spec.AutoVerifiedAttributes) > 0 {
		attrs := make([]cognitotypes.VerifiedAttributeType, 0, len(up.Spec.AutoVerifiedAttributes))
		for _, a := range up.Spec.AutoVerifiedAttributes {
			attrs = append(attrs, cognitotypes.VerifiedAttributeType(a))
		}
		input.AutoVerifiedAttributes = attrs
	}
	if up.Spec.MfaConfiguration != "" {
		input.MfaConfiguration = cognitotypes.UserPoolMfaType(up.Spec.MfaConfiguration)
	}
	if pp := up.Spec.PasswordPolicy; pp != nil {
		input.Policies = &cognitotypes.UserPoolPolicyType{
			PasswordPolicy: &cognitotypes.PasswordPolicyType{
				MinimumLength:    aws.Int32(pp.MinimumLength),
				RequireUppercase: pp.RequireUppercase,
				RequireLowercase: pp.RequireLowercase,
				RequireNumbers:   pp.RequireNumbers,
				RequireSymbols:   pp.RequireSymbols,
			},
		}
	}
	if len(up.Spec.Tags) > 0 {
		input.UserPoolTags = up.Spec.Tags
	}

	out, err := r.CognitoClient.CreateUserPool(ctx, input)
	if err != nil {
		return fmt.Errorf("create user pool: %w", err)
	}

	up.Status.UserPoolID = aws.ToString(out.UserPool.Id)
	up.Status.ARN = aws.ToString(out.UserPool.Arn)
	// Persist the ID immediately: the AWS resource now exists, and losing the
	// identifier would orphan it or create a duplicate on retry.
	if err := persistStatus(ctx, r.Client, up); err != nil {
		return fmt.Errorf("persist user pool ID after create: %w", err)
	}
	up.Status.ObservedGeneration = up.Generation
	now := metav1.Now()
	up.Status.LastSyncTime = &now
	return r.setConditionUP(ctx, up, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "UserPool created")
}

func (r *UserPoolReconciler) deleteUserPool(ctx context.Context, up *awsv1alpha1.UserPool) error {
	if up.Status.UserPoolID == "" {
		return nil
	}
	_, err := r.CognitoClient.DeleteUserPool(ctx, &awscognito.DeleteUserPoolInput{
		UserPoolId: aws.String(up.Status.UserPoolID),
	})
	if cognitohelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *UserPoolReconciler) setConditionUP(ctx context.Context, up *awsv1alpha1.UserPool, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&up.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: up.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, up); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *UserPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.UserPool{}).
		Complete(r)
}
