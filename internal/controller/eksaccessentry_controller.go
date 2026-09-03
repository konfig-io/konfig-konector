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

	awseks "github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ekshelper "github.com/konfig-io/konfig-konector/internal/aws/eks"
)

// EKSAccessEntryReconciler reconciles EKSAccessEntry objects.
type EKSAccessEntryReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EKSClient *awseks.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=eksaccessentries,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eksaccessentries/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eksaccessentries/finalizers,verbs=update

func (r *EKSAccessEntryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ae := &awsv1alpha1.EKSAccessEntry{}
	if err := r.Get(ctx, req.NamespacedName, ae); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !ae.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ae, awsv1alpha1.FinalizerName) {
			if shouldAbandon(ae) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(ae, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, ae)
			}
			if err := r.deleteAccessEntry(ctx, ae); err != nil {
				logger.Error(err, "failed to delete EKS access entry")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(ae, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, ae)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(ae, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(ae, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, ae); err != nil {
			return ctrl.Result{}, err
		}
	}

	result, err := r.reconcileAccessEntry(ctx, ae)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, ae, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *EKSAccessEntryReconciler) reconcileAccessEntry(ctx context.Context, ae *awsv1alpha1.EKSAccessEntry) (ctrl.Result, error) {
	clusterName, err := resolveEKSClusterName(ctx, r.Client, ae.Namespace, ae.Spec.ClusterName, ae.Spec.ClusterRef)
	if err != nil {
		return ctrl.Result{}, err
	}

	var principalArn string
	if ae.Spec.PrincipalArn != "" {
		principalArn = ae.Spec.PrincipalArn
	} else if ae.Spec.PrincipalRef != nil {
		principalArn, err = resolveIAMRoleARN(ctx, r.Client, ae.Namespace, *ae.Spec.PrincipalRef)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		return ctrl.Result{}, fmt.Errorf("either principalArn or principalRef must be set")
	}

	entryIn := ekshelper.AccessEntryInput{
		ClusterName:      clusterName,
		PrincipalArn:     principalArn,
		EntryType:        ae.Spec.Type,
		KubernetesGroups: ae.Spec.KubernetesGroups,
		Username:         ae.Spec.Username,
		Tags:             ae.Spec.Tags,
	}

	if ae.Status.AccessEntryArn != "" {
		existing, err := ekshelper.DescribeAccessEntry(ctx, r.EKSClient, clusterName, principalArn)
		if err != nil && !ekshelper.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err == nil && existing != nil {
			if ae.Status.ObservedGeneration != ae.Generation {
				if err := ekshelper.UpdateAccessEntry(ctx, r.EKSClient, entryIn); err != nil {
					return ctrl.Result{}, fmt.Errorf("update EKS access entry: %w", err)
				}
				if err := r.syncPolicies(ctx, clusterName, principalArn, ae.Spec.AccessPolicies); err != nil {
					return ctrl.Result{}, err
				}
			}

			ae.Status.ObservedGeneration = ae.Generation
			now := metav1.Now()
			ae.Status.LastSyncTime = &now
			return requeueResult(), r.setCondition(ctx, ae, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "EKS access entry synced")
		}
	}

	created, err := ekshelper.CreateAccessEntry(ctx, r.EKSClient, entryIn)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create EKS access entry: %w", err)
	}
	if created.AccessEntryArn != nil {
		ae.Status.AccessEntryArn = *created.AccessEntryArn
	}

	if err := r.syncPolicies(ctx, clusterName, principalArn, ae.Spec.AccessPolicies); err != nil {
		return ctrl.Result{}, err
	}

	ae.Status.ObservedGeneration = ae.Generation
	now := metav1.Now()
	ae.Status.LastSyncTime = &now
	return requeueResult(), r.setCondition(ctx, ae, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "EKS access entry created")
}

func (r *EKSAccessEntryReconciler) syncPolicies(ctx context.Context, clusterName, principalArn string, desired []awsv1alpha1.EKSAccessPolicyAssociation) error {
	existing, err := ekshelper.ListAssociatedAccessPolicies(ctx, r.EKSClient, clusterName, principalArn)
	if err != nil {
		return fmt.Errorf("list access policies: %w", err)
	}

	// Build desired set
	desiredSet := make(map[string]awsv1alpha1.EKSAccessPolicyAssociation, len(desired))
	for _, p := range desired {
		desiredSet[p.PolicyArn] = p
	}

	// Disassociate policies not in desired
	for _, ep := range existing {
		if ep.PolicyArn == nil {
			continue
		}
		if _, ok := desiredSet[*ep.PolicyArn]; !ok {
			if err := ekshelper.DisassociateAccessPolicy(ctx, r.EKSClient, clusterName, principalArn, *ep.PolicyArn); err != nil && !ekshelper.IsNotFound(err) {
				return fmt.Errorf("disassociate policy %s: %w", *ep.PolicyArn, err)
			}
		}
	}

	// Associate desired policies
	for _, p := range desired {
		if err := ekshelper.AssociateAccessPolicy(ctx, r.EKSClient, ekshelper.PolicyAssociationInput{
			ClusterName:  clusterName,
			PrincipalArn: principalArn,
			PolicyArn:    p.PolicyArn,
			ScopeType:    types.AccessScopeType(p.AccessScope.Type),
			Namespaces:   p.AccessScope.Namespaces,
		}); err != nil {
			return fmt.Errorf("associate policy %s: %w", p.PolicyArn, err)
		}
	}
	return nil
}

func (r *EKSAccessEntryReconciler) deleteAccessEntry(ctx context.Context, ae *awsv1alpha1.EKSAccessEntry) error {
	clusterName, err := resolveEKSClusterName(ctx, r.Client, ae.Namespace, ae.Spec.ClusterName, ae.Spec.ClusterRef)
	if err != nil {
		if errors.As(err, new(*dependencyNotReady)) {
			return nil
		}
		return err
	}
	var principalArn string
	if ae.Spec.PrincipalArn != "" {
		principalArn = ae.Spec.PrincipalArn
	} else if ae.Spec.PrincipalRef != nil {
		principalArn, err = resolveIAMRoleARN(ctx, r.Client, ae.Namespace, *ae.Spec.PrincipalRef)
		if err != nil {
			if errors.As(err, new(*dependencyNotReady)) {
				return nil
			}
			return err
		}
	}
	if principalArn == "" {
		return nil
	}
	if err := ekshelper.DeleteAccessEntry(ctx, r.EKSClient, clusterName, principalArn); err != nil && !ekshelper.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *EKSAccessEntryReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.EKSAccessEntry, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: obj.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *EKSAccessEntryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.EKSAccessEntry{}).
		Complete(r)
}
