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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ekshelper "github.com/konfig-io/konfig-konector/internal/aws/eks"
)

// PodIdentityAssociationAWSAPI is the subset of the EKS SDK client used by
// this controller (via the ekshelper Pod Identity functions). *eks.Client
// satisfies it.
type PodIdentityAssociationAWSAPI interface {
	ekshelper.PodIdentityAPI
}

// PodIdentityAssociationReconciler reconciles PodIdentityAssociation objects.
type PodIdentityAssociationReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EKSClient PodIdentityAssociationAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=podidentityassociations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=podidentityassociations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=podidentityassociations/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;update;patch

func (r *PodIdentityAssociationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	pia := &awsv1alpha1.PodIdentityAssociation{}
	if err := r.Get(ctx, req.NamespacedName, pia); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !pia.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(pia, awsv1alpha1.FinalizerName) {
			if shouldAbandon(pia) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(pia, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, pia)
			}
			if pia.Status.AssociationID != "" {
				if err := ekshelper.DeleteAssociation(ctx, r.EKSClient, pia.Spec.ClusterName, pia.Status.AssociationID); err != nil {
					logger.Error(err, "failed to delete Pod Identity association")
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(pia, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, pia)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(pia, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(pia, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, pia); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileAssociation(ctx, pia); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setPIACondition(ctx, pia, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *PodIdentityAssociationReconciler) reconcileAssociation(ctx context.Context, pia *awsv1alpha1.PodIdentityAssociation) error {
	roleARN, err := r.resolveRoleARNForPIA(ctx, pia)
	if err != nil {
		return err
	}
	pia.Status.RoleARN = roleARN

	if pia.Status.AssociationID != "" {
		// Check existing association.
		existing, err := ekshelper.GetAssociation(ctx, r.EKSClient, pia.Spec.ClusterName, pia.Status.AssociationID)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.RoleArn != nil && *existing.RoleArn != roleARN {
				updated, err := ekshelper.UpdateAssociation(ctx, r.EKSClient, pia.Spec.ClusterName, pia.Status.AssociationID, roleARN)
				if err != nil {
					return fmt.Errorf("update association: %w", err)
				}
				if updated.AssociationArn != nil {
					pia.Status.AssociationARN = *updated.AssociationArn
				}
			}
			goto annotate
		}
		// Association was deleted externally — recreate below.
		pia.Status.AssociationID = ""
	}

	{
		// Find by namespace+SA in case we lost state.
		summary, err := ekshelper.FindAssociation(ctx, r.EKSClient, pia.Spec.ClusterName, pia.Spec.TargetNamespace, pia.Spec.ServiceAccountName)
		if err != nil {
			return err
		}
		if summary != nil {
			pia.Status.AssociationID = *summary.AssociationId
			pia.Status.AssociationARN = *summary.AssociationArn
		} else {
			created, err := ekshelper.CreateAssociation(ctx, r.EKSClient, pia.Spec.ClusterName, pia.Spec.TargetNamespace, pia.Spec.ServiceAccountName, roleARN, pia.Spec.Tags)
			if err != nil {
				return fmt.Errorf("create association: %w", err)
			}
			pia.Status.AssociationID = *created.AssociationId
			pia.Status.AssociationARN = *created.AssociationArn
		}
	}

annotate:
	if pia.Spec.AnnotateServiceAccount {
		if err := r.annotateServiceAccount(ctx, pia, roleARN); err != nil {
			return err
		}
	}

	pia.Status.ObservedGeneration = pia.Generation
	now := metav1.Now()
	pia.Status.LastSyncTime = &now
	return r.setPIACondition(ctx, pia, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Pod Identity association reconciled")
}

func (r *PodIdentityAssociationReconciler) resolveRoleARNForPIA(ctx context.Context, pia *awsv1alpha1.PodIdentityAssociation) (string, error) {
	if pia.Spec.RoleRef.ARN != "" {
		return pia.Spec.RoleRef.ARN, nil
	}
	ns := pia.Spec.RoleRef.Namespace
	if ns == "" {
		ns = pia.Namespace
	}
	role := &awsv1alpha1.IAMRole{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: pia.Spec.RoleRef.Name, Namespace: ns}, role); err != nil {
		return "", err
	}
	if role.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("IAMRole %s/%s has no ARN yet", ns, pia.Spec.RoleRef.Name)}
	}
	return role.Status.ARN, nil
}

func (r *PodIdentityAssociationReconciler) annotateServiceAccount(ctx context.Context, pia *awsv1alpha1.PodIdentityAssociation, roleARN string) error {
	sa := &corev1.ServiceAccount{}
	if err := r.Get(ctx, k8stypes.NamespacedName{
		Name:      pia.Spec.ServiceAccountName,
		Namespace: pia.Spec.TargetNamespace,
	}, sa); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // SA may not exist yet; skip
		}
		return err
	}
	if sa.Annotations == nil {
		sa.Annotations = make(map[string]string)
	}
	if sa.Annotations["eks.amazonaws.com/role-arn"] == roleARN {
		return nil
	}
	sa.Annotations["eks.amazonaws.com/role-arn"] = roleARN
	return r.Update(ctx, sa)
}

func (r *PodIdentityAssociationReconciler) setPIACondition(ctx context.Context, pia *awsv1alpha1.PodIdentityAssociation, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&pia.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: pia.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, pia); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *PodIdentityAssociationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.PodIdentityAssociation{}).
		Complete(r)
}
