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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ramhelper "github.com/konfig-io/konfig-konector/internal/aws/ram"
)

// ramTags converts a CR tag map to RAM SDK tags.
func ramTags(tags map[string]string) []ramtypes.Tag {
	if len(tags) == 0 {
		return nil
	}
	out := make([]ramtypes.Tag, 0, len(tags))
	for k, v := range tags {
		out = append(out, ramtypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	return out
}

// ResourceShareAWSAPI is the subset of the RAM API used by this controller.
type ResourceShareAWSAPI interface {
	CreateResourceShare(ctx context.Context, params *awsram.CreateResourceShareInput, optFns ...func(*awsram.Options)) (*awsram.CreateResourceShareOutput, error)
	UpdateResourceShare(ctx context.Context, params *awsram.UpdateResourceShareInput, optFns ...func(*awsram.Options)) (*awsram.UpdateResourceShareOutput, error)
	DeleteResourceShare(ctx context.Context, params *awsram.DeleteResourceShareInput, optFns ...func(*awsram.Options)) (*awsram.DeleteResourceShareOutput, error)
	GetResourceShares(ctx context.Context, params *awsram.GetResourceSharesInput, optFns ...func(*awsram.Options)) (*awsram.GetResourceSharesOutput, error)
	GetResourceShareAssociations(ctx context.Context, params *awsram.GetResourceShareAssociationsInput, optFns ...func(*awsram.Options)) (*awsram.GetResourceShareAssociationsOutput, error)
	AssociateResourceShare(ctx context.Context, params *awsram.AssociateResourceShareInput, optFns ...func(*awsram.Options)) (*awsram.AssociateResourceShareOutput, error)
	DisassociateResourceShare(ctx context.Context, params *awsram.DisassociateResourceShareInput, optFns ...func(*awsram.Options)) (*awsram.DisassociateResourceShareOutput, error)
	TagResource(ctx context.Context, params *awsram.TagResourceInput, optFns ...func(*awsram.Options)) (*awsram.TagResourceOutput, error)
}

// ResourceShareReconciler reconciles ResourceShare objects.
type ResourceShareReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	RAMClient ResourceShareAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=resourceshares,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=resourceshares/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=resourceshares/finalizers,verbs=update

func (r *ResourceShareReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	rs := &awsv1alpha1.ResourceShare{}
	if err := r.Get(ctx, req.NamespacedName, rs); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, rs); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !rs.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(rs, awsv1alpha1.FinalizerName) {
			if shouldAbandon(rs) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(rs, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, rs)
			}
			if err := r.deleteShare(ctx, rs); err != nil {
				logger.Error(err, "failed to delete resource share")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(rs, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, rs)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(rs, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(rs, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, rs); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileShare(ctx, rs); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, rs, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ResourceShareReconciler) reconcileShare(ctx context.Context, rs *awsv1alpha1.ResourceShare) error {
	exists := false
	if rs.Status.ARN != "" {
		out, err := r.RAMClient.GetResourceShares(ctx, &awsram.GetResourceSharesInput{
			ResourceOwner:     ramtypes.ResourceOwnerSelf,
			ResourceShareArns: []string{rs.Status.ARN},
		})
		if err != nil && !ramhelper.IsNotFound(err) {
			return fmt.Errorf("get resource shares: %w", err)
		}
		if err == nil {
			for _, s := range out.ResourceShares {
				if s.Status == ramtypes.ResourceShareStatusActive || s.Status == ramtypes.ResourceShareStatusPending {
					exists = true
				}
			}
		}
	}

	if !exists {
		out, err := r.RAMClient.CreateResourceShare(ctx, &awsram.CreateResourceShareInput{
			Name:                    aws.String(rs.Spec.Name),
			ResourceArns:            rs.Spec.ResourceArns,
			Principals:              rs.Spec.Principals,
			AllowExternalPrincipals: aws.Bool(rs.Spec.AllowExternalPrincipals),
			PermissionArns:          rs.Spec.PermissionArns,
			Tags:                    ramTags(rs.Spec.Tags),
		})
		if err != nil {
			return fmt.Errorf("create resource share: %w", err)
		}
		// Persist the ARN immediately: the AWS resource now exists, and
		// losing the identifier would orphan it on delete.
		rs.Status.ARN = aws.ToString(out.ResourceShare.ResourceShareArn)
		if err := persistStatus(ctx, r.Client, rs); err != nil {
			return fmt.Errorf("persist resource share ARN after create: %w", err)
		}
	} else if rs.Status.ObservedGeneration != rs.Generation {
		if _, err := r.RAMClient.UpdateResourceShare(ctx, &awsram.UpdateResourceShareInput{
			ResourceShareArn:        aws.String(rs.Status.ARN),
			Name:                    aws.String(rs.Spec.Name),
			AllowExternalPrincipals: aws.Bool(rs.Spec.AllowExternalPrincipals),
		}); err != nil {
			return fmt.Errorf("update resource share: %w", err)
		}
		if err := r.syncAssociations(ctx, rs); err != nil {
			return err
		}
		if len(rs.Spec.Tags) > 0 {
			if _, err := r.RAMClient.TagResource(ctx, &awsram.TagResourceInput{
				ResourceShareArn: aws.String(rs.Status.ARN),
				Tags:             ramTags(rs.Spec.Tags),
			}); err != nil {
				return fmt.Errorf("tag resource share: %w", err)
			}
		}
	}

	rs.Status.ObservedGeneration = rs.Generation
	now := metav1.Now()
	rs.Status.LastSyncTime = &now
	return r.setCondition(ctx, rs, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "resource share reconciled")
}

// listAssociations returns the associated entities of the given type that are
// not in a disassociating/failed state.
func (r *ResourceShareReconciler) listAssociations(ctx context.Context, shareARN string, assocType ramtypes.ResourceShareAssociationType) (map[string]bool, error) {
	current := map[string]bool{}
	var next *string
	for {
		out, err := r.RAMClient.GetResourceShareAssociations(ctx, &awsram.GetResourceShareAssociationsInput{
			AssociationType:   assocType,
			ResourceShareArns: []string{shareARN},
			NextToken:         next,
		})
		if err != nil {
			return nil, fmt.Errorf("get resource share associations (%s): %w", assocType, err)
		}
		for _, a := range out.ResourceShareAssociations {
			switch a.Status {
			case ramtypes.ResourceShareAssociationStatusAssociated, ramtypes.ResourceShareAssociationStatusAssociating:
				current[aws.ToString(a.AssociatedEntity)] = true
			}
		}
		if out.NextToken == nil {
			return current, nil
		}
		next = out.NextToken
	}
}

// syncAssociations associates missing and disassociates extra resources and
// principals so live state matches the spec.
func (r *ResourceShareReconciler) syncAssociations(ctx context.Context, rs *awsv1alpha1.ResourceShare) error {
	currentResources, err := r.listAssociations(ctx, rs.Status.ARN, ramtypes.ResourceShareAssociationTypeResource)
	if err != nil {
		return err
	}
	currentPrincipals, err := r.listAssociations(ctx, rs.Status.ARN, ramtypes.ResourceShareAssociationTypePrincipal)
	if err != nil {
		return err
	}

	var addResources, addPrincipals, removeResources, removePrincipals []string
	desiredResources := map[string]bool{}
	for _, arn := range rs.Spec.ResourceArns {
		desiredResources[arn] = true
		if !currentResources[arn] {
			addResources = append(addResources, arn)
		}
	}
	desiredPrincipals := map[string]bool{}
	for _, p := range rs.Spec.Principals {
		desiredPrincipals[p] = true
		if !currentPrincipals[p] {
			addPrincipals = append(addPrincipals, p)
		}
	}
	for arn := range currentResources {
		if !desiredResources[arn] {
			removeResources = append(removeResources, arn)
		}
	}
	for p := range currentPrincipals {
		if !desiredPrincipals[p] {
			removePrincipals = append(removePrincipals, p)
		}
	}

	if len(addResources) > 0 || len(addPrincipals) > 0 {
		if _, err := r.RAMClient.AssociateResourceShare(ctx, &awsram.AssociateResourceShareInput{
			ResourceShareArn: aws.String(rs.Status.ARN),
			ResourceArns:     addResources,
			Principals:       addPrincipals,
		}); err != nil {
			return fmt.Errorf("associate resource share: %w", err)
		}
	}
	if len(removeResources) > 0 || len(removePrincipals) > 0 {
		if _, err := r.RAMClient.DisassociateResourceShare(ctx, &awsram.DisassociateResourceShareInput{
			ResourceShareArn: aws.String(rs.Status.ARN),
			ResourceArns:     removeResources,
			Principals:       removePrincipals,
		}); err != nil {
			return fmt.Errorf("disassociate resource share: %w", err)
		}
	}
	return nil
}

func (r *ResourceShareReconciler) deleteShare(ctx context.Context, rs *awsv1alpha1.ResourceShare) error {
	if rs.Status.ARN == "" {
		// Never created (or the identifier was lost). Share ARNs cannot be
		// derived from the spec, so do not guess.
		return nil
	}
	_, err := r.RAMClient.DeleteResourceShare(ctx, &awsram.DeleteResourceShareInput{
		ResourceShareArn: aws.String(rs.Status.ARN),
	})
	if ramhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ResourceShareReconciler) setCondition(ctx context.Context, rs *awsv1alpha1.ResourceShare, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&rs.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: rs.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, rs); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *ResourceShareReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ResourceShare{}).
		Complete(r)
}
