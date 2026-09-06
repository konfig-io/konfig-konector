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
	awsram "github.com/aws/aws-sdk-go-v2/service/ram"
	ramtypes "github.com/aws/aws-sdk-go-v2/service/ram/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// ResourceShareInvitationAWSAPI is the subset of the RAM API used by this controller.
type ResourceShareInvitationAWSAPI interface {
	GetResourceShareInvitations(ctx context.Context, params *awsram.GetResourceShareInvitationsInput, optFns ...func(*awsram.Options)) (*awsram.GetResourceShareInvitationsOutput, error)
	AcceptResourceShareInvitation(ctx context.Context, params *awsram.AcceptResourceShareInvitationInput, optFns ...func(*awsram.Options)) (*awsram.AcceptResourceShareInvitationOutput, error)
	ListResources(ctx context.Context, params *awsram.ListResourcesInput, optFns ...func(*awsram.Options)) (*awsram.ListResourcesOutput, error)
}

// ResourceShareInvitationReconciler accepts RAM share invitations in the
// receiving account and reports the shared resources.
type ResourceShareInvitationReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	RAMClient ResourceShareInvitationAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=resourceshareinvitations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=resourceshareinvitations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=resourceshareinvitations/finalizers,verbs=update

func (r *ResourceShareInvitationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	obj := &awsv1alpha1.ResourceShareInvitation{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, obj); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !obj.DeletionTimestamp.IsZero() {
		// An accepted invitation cannot be un-accepted by the receiver; the
		// owner removes the principal from the share. Nothing to do in AWS.
		if controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
			controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, obj)
		}
		return ctrl.Result{}, nil
	}
	if !controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(obj, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileInvitation(ctx, obj); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		if errors.Is(err, errPendingAcceptance) {
			return requeuePending, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ResourceShareInvitationReconciler) resolveShareARN(ctx context.Context, obj *awsv1alpha1.ResourceShareInvitation) (string, error) {
	ref := obj.Spec.ResourceShareRef
	if ref.ARN != "" {
		return ref.ARN, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("resourceShareRef requires name or arn")
	}
	ns := ref.Namespace
	if ns == "" {
		ns = obj.Namespace
	}
	rs := &awsv1alpha1.ResourceShare{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: ns}, rs); err != nil {
		return "", err
	}
	if rs.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("ResourceShare %s/%s has no ARN yet", ns, ref.Name)}
	}
	return rs.Status.ARN, nil
}

func (r *ResourceShareInvitationReconciler) reconcileInvitation(ctx context.Context, obj *awsv1alpha1.ResourceShareInvitation) error {
	shareARN, err := r.resolveShareARN(ctx, obj)
	if err != nil {
		return err
	}
	obj.Status.ResourceShareARN = shareARN

	inv, err := r.findInvitation(ctx, shareARN)
	if err != nil {
		return err
	}
	if inv != nil {
		obj.Status.InvitationARN = aws.ToString(inv.ResourceShareInvitationArn)
		obj.Status.SenderAccountID = aws.ToString(inv.SenderAccountId)
		obj.Status.Status = string(inv.Status)
		if inv.Status == ramtypes.ResourceShareInvitationStatusPending {
			out, err := r.RAMClient.AcceptResourceShareInvitation(ctx, &awsram.AcceptResourceShareInvitationInput{
				ResourceShareInvitationArn: inv.ResourceShareInvitationArn,
			})
			if err != nil {
				return fmt.Errorf("accept resource share invitation: %w", err)
			}
			if out.ResourceShareInvitation != nil {
				obj.Status.Status = string(out.ResourceShareInvitation.Status)
			}
		}
	}

	// Resources visible through the share, regardless of how acceptance
	// happened (explicit invitation, or organization sharing with no invite).
	resOut, err := r.RAMClient.ListResources(ctx, &awsram.ListResourcesInput{
		ResourceOwner:     ramtypes.ResourceOwnerOtherAccounts,
		ResourceShareArns: []string{shareARN},
	})
	if err != nil {
		return fmt.Errorf("list shared resources: %w", err)
	}
	obj.Status.ResourceARNs = obj.Status.ResourceARNs[:0]
	for _, res := range resOut.Resources {
		obj.Status.ResourceARNs = append(obj.Status.ResourceARNs, aws.ToString(res.Arn))
	}
	now := metav1.Now()
	obj.Status.LastSyncTime = &now

	accepted := obj.Status.Status == string(ramtypes.ResourceShareInvitationStatusAccepted) || (inv == nil && len(resOut.Resources) > 0)
	if !accepted {
		if obj.Status.Status == "" {
			obj.Status.Status = "NOT_FOUND"
		}
		if err := r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonPendingAcceptance,
			"no accepted invitation for "+shareARN+" yet; waiting for the owner to share with this account"); err != nil {
			return err
		}
		return errPendingAcceptance
	}
	if obj.Status.Status == "" {
		obj.Status.Status = string(ramtypes.ResourceShareInvitationStatusAccepted)
	}
	obj.Status.ObservedGeneration = obj.Generation
	return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "resource share accepted")
}

func (r *ResourceShareInvitationReconciler) findInvitation(ctx context.Context, shareARN string) (*ramtypes.ResourceShareInvitation, error) {
	var token *string
	var best *ramtypes.ResourceShareInvitation
	for {
		out, err := r.RAMClient.GetResourceShareInvitations(ctx, &awsram.GetResourceShareInvitationsInput{
			ResourceShareArns: []string{shareARN},
			NextToken:         token,
		})
		if err != nil {
			return nil, fmt.Errorf("get resource share invitations: %w", err)
		}
		for i := range out.ResourceShareInvitations {
			inv := &out.ResourceShareInvitations[i]
			// Prefer PENDING (actionable), then ACCEPTED, then anything else.
			if best == nil || inv.Status == ramtypes.ResourceShareInvitationStatusPending ||
				(inv.Status == ramtypes.ResourceShareInvitationStatusAccepted && best.Status != ramtypes.ResourceShareInvitationStatusPending) {
				best = inv
			}
		}
		if out.NextToken == nil {
			return best, nil
		}
		token = out.NextToken
	}
}

func (r *ResourceShareInvitationReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.ResourceShareInvitation, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
		Type: condType, Status: status, ObservedGeneration: obj.Generation, Reason: reason, Message: message,
	})
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *ResourceShareInvitationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ResourceShareInvitation{}).
		Complete(r)
}
