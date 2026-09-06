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
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ec2helper "github.com/konfig-io/konfig-konector/internal/aws/ec2"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// PlacementGroupReconciler reconciles PlacementGroup objects.
type PlacementGroupReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client *multi.EC2
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=placementgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=placementgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=placementgroups/finalizers,verbs=update

func (r *PlacementGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	pg := &awsv1alpha1.PlacementGroup{}
	if err := r.Get(ctx, req.NamespacedName, pg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, pg); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !pg.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(pg, awsv1alpha1.FinalizerName) {
			if shouldAbandon(pg) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(pg, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, pg)
			}
			if err := r.deletePlacementGroup(ctx, pg); err != nil {
				logger.Error(err, "failed to delete PlacementGroup")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(pg, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, pg)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(pg, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(pg, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, pg); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcilePlacementGroup(ctx, pg); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionPG(ctx, pg, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *PlacementGroupReconciler) reconcilePlacementGroup(ctx context.Context, pg *awsv1alpha1.PlacementGroup) error {
	// Check if already exists by name.
	out, err := r.EC2Client.DescribePlacementGroups(ctx, &awsec2.DescribePlacementGroupsInput{
		GroupNames: []string{pg.Spec.GroupName},
	})
	if err != nil && !ec2helper.IsNotFound(err) {
		return fmt.Errorf("describe placement group: %w", err)
	}

	if err == nil && len(out.PlacementGroups) > 0 {
		existing := out.PlacementGroups[0]
		pg.Status.GroupID = aws.ToString(existing.GroupId)
		pg.Status.State = string(existing.State)
		// Sync tags.
		if len(pg.Spec.Tags) > 0 {
			if _, tagErr := r.EC2Client.CreateTags(ctx, &awsec2.CreateTagsInput{
				Resources: []string{pg.Status.GroupID},
				Tags:      ec2helper.TagsFromMap(pg.Spec.Tags),
			}); tagErr != nil {
				return fmt.Errorf("tag placement group: %w", tagErr)
			}
		}
	} else {
		// Create.
		input := &awsec2.CreatePlacementGroupInput{
			GroupName: aws.String(pg.Spec.GroupName),
			Strategy:  ec2types.PlacementStrategy(pg.Spec.Strategy),
			TagSpecifications: []ec2types.TagSpecification{
				{
					ResourceType: ec2types.ResourceTypePlacementGroup,
					Tags:         ec2helper.TagsFromMap(pg.Spec.Tags),
				},
			},
		}
		if pg.Spec.PartitionCount > 0 {
			input.PartitionCount = aws.Int32(pg.Spec.PartitionCount)
		}
		if pg.Spec.SpreadLevel != "" {
			input.SpreadLevel = ec2types.SpreadLevel(pg.Spec.SpreadLevel)
		}
		createOut, createErr := r.EC2Client.CreatePlacementGroup(ctx, input)
		if createErr != nil {
			return fmt.Errorf("create placement group: %w", createErr)
		}
		if createOut.PlacementGroup != nil {
			pg.Status.GroupID = aws.ToString(createOut.PlacementGroup.GroupId)
			pg.Status.State = string(createOut.PlacementGroup.State)
		}
	}

	pg.Status.ObservedGeneration = pg.Generation
	now := metav1.Now()
	pg.Status.LastSyncTime = &now
	return r.setConditionPG(ctx, pg, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "PlacementGroup reconciled")
}

func (r *PlacementGroupReconciler) deletePlacementGroup(ctx context.Context, pg *awsv1alpha1.PlacementGroup) error {
	if pg.Spec.GroupName == "" {
		return nil
	}
	_, err := r.EC2Client.DeletePlacementGroup(ctx, &awsec2.DeletePlacementGroupInput{
		GroupName: aws.String(pg.Spec.GroupName),
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *PlacementGroupReconciler) setConditionPG(ctx context.Context, pg *awsv1alpha1.PlacementGroup, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&pg.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: pg.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, pg); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *PlacementGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.PlacementGroup{}).
		Complete(r)
}
