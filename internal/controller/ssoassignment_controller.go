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
	awssso "github.com/aws/aws-sdk-go-v2/service/ssoadmin"
	ssotypes "github.com/aws/aws-sdk-go-v2/service/ssoadmin/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ssohelper "github.com/konfig-io/konfig-konector/internal/aws/ssoadmin"
)

// SSOAssignmentAWSAPI is the subset of the SSO Admin API used by this controller.
type SSOAssignmentAWSAPI interface {
	CreateAccountAssignment(ctx context.Context, params *awssso.CreateAccountAssignmentInput, optFns ...func(*awssso.Options)) (*awssso.CreateAccountAssignmentOutput, error)
	DeleteAccountAssignment(ctx context.Context, params *awssso.DeleteAccountAssignmentInput, optFns ...func(*awssso.Options)) (*awssso.DeleteAccountAssignmentOutput, error)
	DescribeAccountAssignmentCreationStatus(ctx context.Context, params *awssso.DescribeAccountAssignmentCreationStatusInput, optFns ...func(*awssso.Options)) (*awssso.DescribeAccountAssignmentCreationStatusOutput, error)
	ListAccountAssignments(ctx context.Context, params *awssso.ListAccountAssignmentsInput, optFns ...func(*awssso.Options)) (*awssso.ListAccountAssignmentsOutput, error)
}

// SSOAssignmentReconciler reconciles SSOAssignment objects. Assignment
// creation is asynchronous: the controller polls the creation status via
// DescribeAccountAssignmentCreationStatus.
type SSOAssignmentReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	SSOAdminClient SSOAssignmentAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=ssoassignments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ssoassignments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ssoassignments/finalizers,verbs=update

func (r *SSOAssignmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	sa := &awsv1alpha1.SSOAssignment{}
	if err := r.Get(ctx, req.NamespacedName, sa); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !sa.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(sa, awsv1alpha1.FinalizerName) {
			if shouldAbandon(sa) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(sa, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, sa)
			}
			if err := r.deleteAssignment(ctx, sa); err != nil {
				logger.Error(err, "failed to delete account assignment")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(sa, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, sa)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(sa, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(sa, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, sa); err != nil {
			return ctrl.Result{}, err
		}
	}

	result, err := r.reconcileAssignment(ctx, sa)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, sa, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *SSOAssignmentReconciler) resolvePermissionSetArn(ctx context.Context, sa *awsv1alpha1.SSOAssignment) (string, error) {
	if sa.Spec.PermissionSetRef.ARN != "" {
		return sa.Spec.PermissionSetRef.ARN, nil
	}
	if sa.Spec.PermissionSetRef.Name == "" {
		return "", fmt.Errorf("permissionSetRef must set either name or arn")
	}
	ps := &awsv1alpha1.PermissionSet{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: sa.Spec.PermissionSetRef.Name, Namespace: sa.Namespace}, ps); err != nil {
		return "", err
	}
	if ps.Status.PermissionSetArn == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("PermissionSet %s/%s has no ARN yet", sa.Namespace, sa.Spec.PermissionSetRef.Name)}
	}
	return ps.Status.PermissionSetArn, nil
}

// assignmentExists checks whether the assignment already exists in AWS.
func (r *SSOAssignmentReconciler) assignmentExists(ctx context.Context, sa *awsv1alpha1.SSOAssignment, psArn string) (bool, error) {
	var next *string
	for {
		out, err := r.SSOAdminClient.ListAccountAssignments(ctx, &awssso.ListAccountAssignmentsInput{
			InstanceArn:      aws.String(sa.Spec.InstanceArn),
			AccountId:        aws.String(sa.Spec.TargetId),
			PermissionSetArn: aws.String(psArn),
			NextToken:        next,
		})
		if err != nil {
			return false, fmt.Errorf("list account assignments: %w", err)
		}
		for _, a := range out.AccountAssignments {
			if aws.ToString(a.PrincipalId) == sa.Spec.PrincipalId && string(a.PrincipalType) == sa.Spec.PrincipalType {
				return true, nil
			}
		}
		if out.NextToken == nil {
			return false, nil
		}
		next = out.NextToken
	}
}

func (r *SSOAssignmentReconciler) reconcileAssignment(ctx context.Context, sa *awsv1alpha1.SSOAssignment) (ctrl.Result, error) {
	psArn, err := r.resolvePermissionSetArn(ctx, sa)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Poll an in-flight create operation first.
	if sa.Status.RequestId != "" && sa.Status.State == string(ssotypes.StatusValuesInProgress) {
		out, err := r.SSOAdminClient.DescribeAccountAssignmentCreationStatus(ctx, &awssso.DescribeAccountAssignmentCreationStatusInput{
			InstanceArn:                        aws.String(sa.Spec.InstanceArn),
			AccountAssignmentCreationRequestId: aws.String(sa.Status.RequestId),
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("describe assignment creation status: %w", err)
		}
		st := out.AccountAssignmentCreationStatus
		sa.Status.State = string(st.Status)
		switch st.Status {
		case ssotypes.StatusValuesInProgress:
			if err := r.setCondition(ctx, sa, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "account assignment creation in progress"); err != nil {
				return ctrl.Result{}, err
			}
			return requeueOrgPolling, nil
		case ssotypes.StatusValuesFailed:
			reason := aws.ToString(st.FailureReason)
			sa.Status.RequestId = ""
			_ = persistStatus(ctx, r.Client, sa)
			return ctrl.Result{}, fmt.Errorf("account assignment creation failed: %s", reason)
		}
		// SUCCEEDED falls through to steady state below.
	}

	if sa.Status.PermissionSetArn == "" || sa.Status.State != string(ssotypes.StatusValuesSucceeded) {
		exists, err := r.assignmentExists(ctx, sa, psArn)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !exists {
			out, err := r.SSOAdminClient.CreateAccountAssignment(ctx, &awssso.CreateAccountAssignmentInput{
				InstanceArn:      aws.String(sa.Spec.InstanceArn),
				PermissionSetArn: aws.String(psArn),
				PrincipalType:    ssotypes.PrincipalType(sa.Spec.PrincipalType),
				PrincipalId:      aws.String(sa.Spec.PrincipalId),
				TargetType:       ssotypes.TargetTypeAwsAccount,
				TargetId:         aws.String(sa.Spec.TargetId),
			})
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("create account assignment: %w", err)
			}
			// Persist identifiers immediately after the create call.
			sa.Status.PermissionSetArn = psArn
			sa.Status.RequestId = aws.ToString(out.AccountAssignmentCreationStatus.RequestId)
			sa.Status.State = string(out.AccountAssignmentCreationStatus.Status)
			if err := persistStatus(ctx, r.Client, sa); err != nil {
				return ctrl.Result{}, fmt.Errorf("persist assignment request ID after create: %w", err)
			}
			if sa.Status.State == string(ssotypes.StatusValuesInProgress) {
				if err := r.setCondition(ctx, sa, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "account assignment creation in progress"); err != nil {
					return ctrl.Result{}, err
				}
				return requeueOrgPolling, nil
			}
		} else {
			sa.Status.PermissionSetArn = psArn
			sa.Status.State = string(ssotypes.StatusValuesSucceeded)
			if err := persistStatus(ctx, r.Client, sa); err != nil {
				return ctrl.Result{}, fmt.Errorf("persist assignment identifiers: %w", err)
			}
		}
	}

	sa.Status.PermissionSetArn = psArn
	sa.Status.ObservedGeneration = sa.Generation
	now := metav1.Now()
	sa.Status.LastSyncTime = &now
	if err := r.setCondition(ctx, sa, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "account assignment reconciled"); err != nil {
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *SSOAssignmentReconciler) deleteAssignment(ctx context.Context, sa *awsv1alpha1.SSOAssignment) error {
	psArn := sa.Status.PermissionSetArn
	if psArn == "" {
		psArn = sa.Spec.PermissionSetRef.ARN
	}
	if psArn == "" {
		// Assignment was never created and the ARN cannot be derived.
		return nil
	}
	_, err := r.SSOAdminClient.DeleteAccountAssignment(ctx, &awssso.DeleteAccountAssignmentInput{
		InstanceArn:      aws.String(sa.Spec.InstanceArn),
		PermissionSetArn: aws.String(psArn),
		PrincipalType:    ssotypes.PrincipalType(sa.Spec.PrincipalType),
		PrincipalId:      aws.String(sa.Spec.PrincipalId),
		TargetType:       ssotypes.TargetTypeAwsAccount,
		TargetId:         aws.String(sa.Spec.TargetId),
	})
	if ssohelper.IsNotFound(err) {
		return nil
	}
	// Deletion is asynchronous too, but the operation continues server-side
	// after the CR is gone; no need to poll before removing the finalizer.
	return err
}

func (r *SSOAssignmentReconciler) setCondition(ctx context.Context, sa *awsv1alpha1.SSOAssignment, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&sa.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: sa.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, sa); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *SSOAssignmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.SSOAssignment{}).
		Complete(r)
}
