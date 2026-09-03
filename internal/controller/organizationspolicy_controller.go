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
	awsorgs "github.com/aws/aws-sdk-go-v2/service/organizations"
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	orghelper "github.com/konfig-io/konfig-konector/internal/aws/organizations"
)

// OrganizationsPolicyAWSAPI is the subset of the Organizations API used by this controller.
type OrganizationsPolicyAWSAPI interface {
	CreatePolicy(ctx context.Context, params *awsorgs.CreatePolicyInput, optFns ...func(*awsorgs.Options)) (*awsorgs.CreatePolicyOutput, error)
	UpdatePolicy(ctx context.Context, params *awsorgs.UpdatePolicyInput, optFns ...func(*awsorgs.Options)) (*awsorgs.UpdatePolicyOutput, error)
	DeletePolicy(ctx context.Context, params *awsorgs.DeletePolicyInput, optFns ...func(*awsorgs.Options)) (*awsorgs.DeletePolicyOutput, error)
	DescribePolicy(ctx context.Context, params *awsorgs.DescribePolicyInput, optFns ...func(*awsorgs.Options)) (*awsorgs.DescribePolicyOutput, error)
	TagResource(ctx context.Context, params *awsorgs.TagResourceInput, optFns ...func(*awsorgs.Options)) (*awsorgs.TagResourceOutput, error)
}

// OrganizationsPolicyReconciler reconciles OrganizationsPolicy objects.
type OrganizationsPolicyReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	OrganizationsClient OrganizationsPolicyAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=organizationspolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=organizationspolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=organizationspolicies/finalizers,verbs=update

func (r *OrganizationsPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	pol := &awsv1alpha1.OrganizationsPolicy{}
	if err := r.Get(ctx, req.NamespacedName, pol); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !pol.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(pol, awsv1alpha1.FinalizerName) {
			if shouldAbandon(pol) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(pol, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, pol)
			}
			if err := r.deletePolicy(ctx, pol); err != nil {
				logger.Error(err, "failed to delete organizations policy")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(pol, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, pol)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(pol, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(pol, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, pol); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcilePolicy(ctx, pol); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, pol, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *OrganizationsPolicyReconciler) reconcilePolicy(ctx context.Context, pol *awsv1alpha1.OrganizationsPolicy) error {
	exists := false
	if pol.Status.PolicyID != "" {
		_, err := r.OrganizationsClient.DescribePolicy(ctx, &awsorgs.DescribePolicyInput{
			PolicyId: aws.String(pol.Status.PolicyID),
		})
		if err == nil {
			exists = true
		} else if !orghelper.IsNotFound(err) {
			return fmt.Errorf("describe policy: %w", err)
		}
	}

	if !exists {
		out, err := r.OrganizationsClient.CreatePolicy(ctx, &awsorgs.CreatePolicyInput{
			Name:        aws.String(pol.Spec.Name),
			Type:        orgtypes.PolicyType(pol.Spec.Type),
			Content:     aws.String(pol.Spec.Content),
			Description: aws.String(pol.Spec.Description),
			Tags:        orgTags(pol.Spec.Tags),
		})
		if err != nil {
			return fmt.Errorf("create policy: %w", err)
		}
		// Persist the policy ID immediately: the AWS resource now exists, and
		// losing the identifier would orphan it on delete.
		pol.Status.PolicyID = aws.ToString(out.Policy.PolicySummary.Id)
		pol.Status.ARN = aws.ToString(out.Policy.PolicySummary.Arn)
		if err := persistStatus(ctx, r.Client, pol); err != nil {
			return fmt.Errorf("persist policy ID after create: %w", err)
		}
	} else if pol.Status.ObservedGeneration != pol.Generation {
		if _, err := r.OrganizationsClient.UpdatePolicy(ctx, &awsorgs.UpdatePolicyInput{
			PolicyId:    aws.String(pol.Status.PolicyID),
			Name:        aws.String(pol.Spec.Name),
			Content:     aws.String(pol.Spec.Content),
			Description: aws.String(pol.Spec.Description),
		}); err != nil {
			return fmt.Errorf("update policy: %w", err)
		}
		if len(pol.Spec.Tags) > 0 {
			if _, err := r.OrganizationsClient.TagResource(ctx, &awsorgs.TagResourceInput{
				ResourceId: aws.String(pol.Status.PolicyID),
				Tags:       orgTags(pol.Spec.Tags),
			}); err != nil {
				return fmt.Errorf("tag policy: %w", err)
			}
		}
	}

	pol.Status.ObservedGeneration = pol.Generation
	now := metav1.Now()
	pol.Status.LastSyncTime = &now
	return r.setCondition(ctx, pol, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "organizations policy reconciled")
}

func (r *OrganizationsPolicyReconciler) deletePolicy(ctx context.Context, pol *awsv1alpha1.OrganizationsPolicy) error {
	if pol.Status.PolicyID == "" {
		// Never created (or the identifier was lost). Policy IDs cannot be
		// derived unambiguously from the spec, so do not guess.
		return nil
	}
	_, err := r.OrganizationsClient.DeletePolicy(ctx, &awsorgs.DeletePolicyInput{
		PolicyId: aws.String(pol.Status.PolicyID),
	})
	if orghelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *OrganizationsPolicyReconciler) setCondition(ctx context.Context, pol *awsv1alpha1.OrganizationsPolicy, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&pol.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: pol.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, pol); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *OrganizationsPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.OrganizationsPolicy{}).
		Complete(r)
}
