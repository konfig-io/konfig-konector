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
	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	iamhelper "github.com/konfig-io/konfig-konector/internal/aws/iam"
)

// IAMInstanceProfileAWSAPI is the subset of the IAM API used by this controller.
type IAMInstanceProfileAWSAPI interface {
	GetInstanceProfile(ctx context.Context, params *awsiam.GetInstanceProfileInput, optFns ...func(*awsiam.Options)) (*awsiam.GetInstanceProfileOutput, error)
	CreateInstanceProfile(ctx context.Context, params *awsiam.CreateInstanceProfileInput, optFns ...func(*awsiam.Options)) (*awsiam.CreateInstanceProfileOutput, error)
	DeleteInstanceProfile(ctx context.Context, params *awsiam.DeleteInstanceProfileInput, optFns ...func(*awsiam.Options)) (*awsiam.DeleteInstanceProfileOutput, error)
	AddRoleToInstanceProfile(ctx context.Context, params *awsiam.AddRoleToInstanceProfileInput, optFns ...func(*awsiam.Options)) (*awsiam.AddRoleToInstanceProfileOutput, error)
	RemoveRoleFromInstanceProfile(ctx context.Context, params *awsiam.RemoveRoleFromInstanceProfileInput, optFns ...func(*awsiam.Options)) (*awsiam.RemoveRoleFromInstanceProfileOutput, error)
}

// IAMInstanceProfileReconciler reconciles IAMInstanceProfile objects.
type IAMInstanceProfileReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	IAMClient IAMInstanceProfileAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=iaminstanceprofiles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=iaminstanceprofiles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=iaminstanceprofiles/finalizers,verbs=update

func (r *IAMInstanceProfileReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ip := &awsv1alpha1.IAMInstanceProfile{}
	if err := r.Get(ctx, req.NamespacedName, ip); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, ip); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !ip.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ip, awsv1alpha1.FinalizerName) {
			if shouldAbandon(ip) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(ip, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, ip)
			}
			if err := r.deleteProfile(ctx, ip); err != nil {
				logger.Error(err, "failed to delete instance profile")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(ip, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, ip)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(ip, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(ip, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, ip); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileProfile(ctx, ip); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, ip, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

// desiredRoleName resolves the spec roleRef to an IAM role name, or "" when
// no role is desired.
func (r *IAMInstanceProfileReconciler) desiredRoleName(ctx context.Context, ip *awsv1alpha1.IAMInstanceProfile) (string, error) {
	if ip.Spec.RoleRef == nil {
		return "", nil
	}
	arn, err := resolveIAMRoleARN(ctx, r.Client, ip.Namespace, *ip.Spec.RoleRef)
	if err != nil {
		return "", err
	}
	return roleNameFromARN(arn), nil
}

func (r *IAMInstanceProfileReconciler) reconcileProfile(ctx context.Context, ip *awsv1alpha1.IAMInstanceProfile) error {
	desiredRole, err := r.desiredRoleName(ctx, ip)
	if err != nil {
		return err
	}

	var profile *iamtypes.InstanceProfile
	getOut, err := r.IAMClient.GetInstanceProfile(ctx, &awsiam.GetInstanceProfileInput{
		InstanceProfileName: aws.String(ip.Spec.InstanceProfileName),
	})
	if iamhelper.IsNotFound(err) {
		input := &awsiam.CreateInstanceProfileInput{
			InstanceProfileName: aws.String(ip.Spec.InstanceProfileName),
		}
		if ip.Spec.Path != "" {
			input.Path = aws.String(ip.Spec.Path)
		}
		for k, v := range ip.Spec.Tags {
			input.Tags = append(input.Tags, iamtypes.Tag{Key: aws.String(k), Value: aws.String(v)})
		}
		createOut, err := r.IAMClient.CreateInstanceProfile(ctx, input)
		if err != nil {
			return fmt.Errorf("create instance profile: %w", err)
		}
		profile = createOut.InstanceProfile
		if profile != nil {
			ip.Status.ARN = aws.ToString(profile.Arn)
		}
		if err := persistStatus(ctx, r.Client, ip); err != nil {
			return fmt.Errorf("persist instance profile ARN after create: %w", err)
		}
	} else if err != nil {
		return err
	} else {
		profile = getOut.InstanceProfile
		if profile != nil {
			ip.Status.ARN = aws.ToString(profile.Arn)
		}
	}

	// Sync the (at most one) role in the profile.
	currentRole := ""
	if profile != nil && len(profile.Roles) > 0 {
		currentRole = aws.ToString(profile.Roles[0].RoleName)
	}
	if currentRole != desiredRole {
		if currentRole != "" {
			if _, err := r.IAMClient.RemoveRoleFromInstanceProfile(ctx, &awsiam.RemoveRoleFromInstanceProfileInput{
				InstanceProfileName: aws.String(ip.Spec.InstanceProfileName),
				RoleName:            aws.String(currentRole),
			}); err != nil && !iamhelper.IsNotFound(err) {
				return fmt.Errorf("remove role from instance profile: %w", err)
			}
		}
		if desiredRole != "" {
			if _, err := r.IAMClient.AddRoleToInstanceProfile(ctx, &awsiam.AddRoleToInstanceProfileInput{
				InstanceProfileName: aws.String(ip.Spec.InstanceProfileName),
				RoleName:            aws.String(desiredRole),
			}); err != nil {
				return fmt.Errorf("add role to instance profile: %w", err)
			}
		}
	}
	ip.Status.RoleName = desiredRole

	ip.Status.ObservedGeneration = ip.Generation
	now := metav1.Now()
	ip.Status.LastSyncTime = &now
	return r.setCondition(ctx, ip, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "instance profile reconciled")
}

func (r *IAMInstanceProfileReconciler) deleteProfile(ctx context.Context, ip *awsv1alpha1.IAMInstanceProfile) error {
	// Roles must be removed before the profile can be deleted.
	getOut, err := r.IAMClient.GetInstanceProfile(ctx, &awsiam.GetInstanceProfileInput{
		InstanceProfileName: aws.String(ip.Spec.InstanceProfileName),
	})
	if iamhelper.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if getOut.InstanceProfile != nil {
		for _, role := range getOut.InstanceProfile.Roles {
			if _, err := r.IAMClient.RemoveRoleFromInstanceProfile(ctx, &awsiam.RemoveRoleFromInstanceProfileInput{
				InstanceProfileName: aws.String(ip.Spec.InstanceProfileName),
				RoleName:            role.RoleName,
			}); err != nil && !iamhelper.IsNotFound(err) {
				return fmt.Errorf("remove role before delete: %w", err)
			}
		}
	}
	_, err = r.IAMClient.DeleteInstanceProfile(ctx, &awsiam.DeleteInstanceProfileInput{
		InstanceProfileName: aws.String(ip.Spec.InstanceProfileName),
	})
	if iamhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *IAMInstanceProfileReconciler) setCondition(ctx context.Context, ip *awsv1alpha1.IAMInstanceProfile, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&ip.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: ip.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, ip); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *IAMInstanceProfileReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.IAMInstanceProfile{}).
		Complete(r)
}
