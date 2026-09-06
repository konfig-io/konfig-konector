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
	awsxray "github.com/aws/aws-sdk-go-v2/service/xray"
	xraytypes "github.com/aws/aws-sdk-go-v2/service/xray/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	xrayhelper "github.com/konfig-io/konfig-konector/internal/aws/xray"
)

// XRayGroupAWSAPI is the subset of the X-Ray API used by this controller.
type XRayGroupAWSAPI interface {
	GetGroup(ctx context.Context, params *awsxray.GetGroupInput, optFns ...func(*awsxray.Options)) (*awsxray.GetGroupOutput, error)
	CreateGroup(ctx context.Context, params *awsxray.CreateGroupInput, optFns ...func(*awsxray.Options)) (*awsxray.CreateGroupOutput, error)
	UpdateGroup(ctx context.Context, params *awsxray.UpdateGroupInput, optFns ...func(*awsxray.Options)) (*awsxray.UpdateGroupOutput, error)
	DeleteGroup(ctx context.Context, params *awsxray.DeleteGroupInput, optFns ...func(*awsxray.Options)) (*awsxray.DeleteGroupOutput, error)
}

// XRayGroupReconciler reconciles XRayGroup objects.
type XRayGroupReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	XRayClient XRayGroupAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=xraygroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=xraygroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=xraygroups/finalizers,verbs=update

func (r *XRayGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	g := &awsv1alpha1.XRayGroup{}
	if err := r.Get(ctx, req.NamespacedName, g); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, g); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !g.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(g, awsv1alpha1.FinalizerName) {
			if shouldAbandon(g) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(g, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, g)
			}
			if err := r.deleteGroup(ctx, g); err != nil {
				logger.Error(err, "failed to delete X-Ray group")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(g, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, g)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(g, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(g, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, g); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileGroup(ctx, g); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, g, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *XRayGroupReconciler) insightsConfig(g *awsv1alpha1.XRayGroup) *xraytypes.InsightsConfiguration {
	if g.Spec.InsightsConfiguration == nil {
		return nil
	}
	return &xraytypes.InsightsConfiguration{
		InsightsEnabled:      aws.Bool(g.Spec.InsightsConfiguration.Enabled),
		NotificationsEnabled: aws.Bool(g.Spec.InsightsConfiguration.NotificationsEnabled),
	}
}

func (r *XRayGroupReconciler) reconcileGroup(ctx context.Context, g *awsv1alpha1.XRayGroup) error {
	got, err := r.XRayClient.GetGroup(ctx, &awsxray.GetGroupInput{
		GroupName: aws.String(g.Spec.GroupName),
	})
	if xrayhelper.IsNotFound(err) {
		in := &awsxray.CreateGroupInput{
			GroupName:             aws.String(g.Spec.GroupName),
			InsightsConfiguration: r.insightsConfig(g),
		}
		if g.Spec.FilterExpression != "" {
			in.FilterExpression = aws.String(g.Spec.FilterExpression)
		}
		for k, v := range g.Spec.Tags {
			in.Tags = append(in.Tags, xraytypes.Tag{Key: aws.String(k), Value: aws.String(v)})
		}
		created, err := r.XRayClient.CreateGroup(ctx, in)
		if err != nil {
			return fmt.Errorf("create X-Ray group: %w", err)
		}
		if created.Group != nil {
			g.Status.ARN = aws.ToString(created.Group.GroupARN)
		}
		// Persist the ARN immediately: the AWS resource now exists, and losing
		// the identifier would orphan it on delete.
		if err := persistStatus(ctx, r.Client, g); err != nil {
			return fmt.Errorf("persist group ARN after create: %w", err)
		}
	} else if err != nil {
		return err
	} else {
		if got.Group != nil {
			g.Status.ARN = aws.ToString(got.Group.GroupARN)
		}
		if g.Status.ObservedGeneration != g.Generation {
			in := &awsxray.UpdateGroupInput{
				GroupName:             aws.String(g.Spec.GroupName),
				InsightsConfiguration: r.insightsConfig(g),
			}
			if g.Spec.FilterExpression != "" {
				in.FilterExpression = aws.String(g.Spec.FilterExpression)
			}
			if _, err := r.XRayClient.UpdateGroup(ctx, in); err != nil {
				return fmt.Errorf("update X-Ray group: %w", err)
			}
		}
	}

	g.Status.ObservedGeneration = g.Generation
	now := metav1.Now()
	g.Status.LastSyncTime = &now
	return r.setCondition(ctx, g, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "X-Ray group reconciled")
}

func (r *XRayGroupReconciler) deleteGroup(ctx context.Context, g *awsv1alpha1.XRayGroup) error {
	in := &awsxray.DeleteGroupInput{}
	if g.Status.ARN != "" {
		in.GroupARN = aws.String(g.Status.ARN)
	} else {
		// The group name in spec is the deterministic AWS identifier.
		in.GroupName = aws.String(g.Spec.GroupName)
	}
	_, err := r.XRayClient.DeleteGroup(ctx, in)
	if xrayhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *XRayGroupReconciler) setCondition(ctx context.Context, g *awsv1alpha1.XRayGroup, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&g.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: g.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, g); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *XRayGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.XRayGroup{}).
		Complete(r)
}
