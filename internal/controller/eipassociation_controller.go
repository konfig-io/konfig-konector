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
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ec2helper "github.com/konfig-io/konfig-konector/internal/aws/ec2"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// EIPAssociationReconciler reconciles EIPAssociation objects.
type EIPAssociationReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client *multi.EC2
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=eipassociations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eipassociations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eipassociations/finalizers,verbs=update

func (r *EIPAssociationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	assoc := &awsv1alpha1.EIPAssociation{}
	if err := r.Get(ctx, req.NamespacedName, assoc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, assoc); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !assoc.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(assoc, awsv1alpha1.FinalizerName) {
			if shouldAbandon(assoc) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(assoc, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, assoc)
			}
			if err := r.deleteEIPAssociation(ctx, assoc); err != nil {
				logger.Error(err, "failed to delete EIPAssociation")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(assoc, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, assoc)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(assoc, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(assoc, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, assoc); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileEIPAssociation(ctx, assoc); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionEIPAssoc(ctx, assoc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *EIPAssociationReconciler) reconcileEIPAssociation(ctx context.Context, assoc *awsv1alpha1.EIPAssociation) error {
	// Resolve allocation ID.
	allocationID, err := r.resolveAllocationID(ctx, assoc)
	if err != nil {
		return err
	}

	// If already associated, verify it still exists.
	if assoc.Status.AssociationID != "" {
		out, err := r.EC2Client.DescribeAddresses(ctx, &awsec2.DescribeAddressesInput{
			AllocationIds: []string{allocationID},
		})
		if err == nil && len(out.Addresses) > 0 && aws.ToString(out.Addresses[0].AssociationId) == assoc.Status.AssociationID {
			assoc.Status.ObservedGeneration = assoc.Generation
			now := metav1.Now()
			assoc.Status.LastSyncTime = &now
			return r.setConditionEIPAssoc(ctx, assoc, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "EIPAssociation reconciled")
		}
		// Association gone — fall through to re-associate.
		assoc.Status.AssociationID = ""
	}

	input := &awsec2.AssociateAddressInput{
		AllocationId:       aws.String(allocationID),
		AllowReassociation: aws.Bool(assoc.Spec.AllowReassociation),
	}
	if assoc.Spec.InstanceID != "" {
		input.InstanceId = aws.String(assoc.Spec.InstanceID)
	}
	if assoc.Spec.NetworkInterfaceID != "" {
		input.NetworkInterfaceId = aws.String(assoc.Spec.NetworkInterfaceID)
	}
	if assoc.Spec.PrivateIPAddress != "" {
		input.PrivateIpAddress = aws.String(assoc.Spec.PrivateIPAddress)
	}

	out, err := r.EC2Client.AssociateAddress(ctx, input)
	if err != nil {
		return fmt.Errorf("associate address: %w", err)
	}

	assoc.Status.AssociationID = aws.ToString(out.AssociationId)
	if err := persistStatus(ctx, r.Client, assoc); err != nil {
		return fmt.Errorf("persist EIPAssociation ID after create: %w", err)
	}
	assoc.Status.ObservedGeneration = assoc.Generation
	now := metav1.Now()
	assoc.Status.LastSyncTime = &now
	return r.setConditionEIPAssoc(ctx, assoc, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "EIPAssociation created")
}

func (r *EIPAssociationReconciler) resolveAllocationID(ctx context.Context, assoc *awsv1alpha1.EIPAssociation) (string, error) {
	ref := assoc.Spec.ElasticIPRef
	if ref.AllocationID != "" {
		return ref.AllocationID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("elasticIpRef must specify either name or allocationId")
	}
	eip := &awsv1alpha1.ElasticIP{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: assoc.Namespace}, eip); err != nil {
		return "", err
	}
	if eip.Status.AllocationID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("ElasticIP %s/%s has no allocationId yet", assoc.Namespace, ref.Name)}
	}
	return eip.Status.AllocationID, nil
}

func (r *EIPAssociationReconciler) deleteEIPAssociation(ctx context.Context, assoc *awsv1alpha1.EIPAssociation) error {
	associationID := assoc.Status.AssociationID
	if associationID == "" {
		// Fallback: the status write may have been lost after associate.
		// The allocation ID is deterministic from spec, so describe the
		// address and disassociate whatever association it currently has.
		allocationID, err := r.resolveAllocationID(ctx, assoc)
		if err != nil {
			// The referenced ElasticIP CR may already be gone — nothing to find.
			return nil
		}
		out, err := r.EC2Client.DescribeAddresses(ctx, &awsec2.DescribeAddressesInput{
			AllocationIds: []string{allocationID},
		})
		if err != nil {
			if ec2helper.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("lookup address for association: %w", err)
		}
		if len(out.Addresses) == 0 || aws.ToString(out.Addresses[0].AssociationId) == "" {
			return nil
		}
		associationID = aws.ToString(out.Addresses[0].AssociationId)
	}
	_, err := r.EC2Client.DisassociateAddress(ctx, &awsec2.DisassociateAddressInput{
		AssociationId: aws.String(associationID),
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *EIPAssociationReconciler) setConditionEIPAssoc(ctx context.Context, assoc *awsv1alpha1.EIPAssociation, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&assoc.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: assoc.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, assoc); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *EIPAssociationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.EIPAssociation{}).
		Complete(r)
}
