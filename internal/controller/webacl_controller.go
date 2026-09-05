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
	awswafv2 "github.com/aws/aws-sdk-go-v2/service/wafv2"
	waftypes "github.com/aws/aws-sdk-go-v2/service/wafv2/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
	wafhelper "github.com/konfig-io/konfig-konector/internal/aws/wafv2"
)

// WebACLReconciler reconciles WebACL objects.
type WebACLReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	WAFv2Client *multi.WAFv2
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=webacls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=webacls/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=webacls/finalizers,verbs=update

func (r *WebACLReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	wa := &awsv1alpha1.WebACL{}
	if err := r.Get(ctx, req.NamespacedName, wa); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, wa); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !wa.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(wa, awsv1alpha1.FinalizerName) {
			if shouldAbandon(wa) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(wa, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, wa)
			}
			if err := r.deleteWebACL(ctx, wa); err != nil {
				logger.Error(err, "failed to delete WebACL")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(wa, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, wa)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(wa, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(wa, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, wa); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileWebACL(ctx, wa); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionWA(ctx, wa, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *WebACLReconciler) reconcileWebACL(ctx context.Context, wa *awsv1alpha1.WebACL) error {
	scope := waftypes.Scope(wa.Spec.Scope)

	if wa.Status.ID != "" {
		out, err := r.WAFv2Client.GetWebACL(ctx, &awswafv2.GetWebACLInput{
			Name:  aws.String(wa.Spec.Name),
			Scope: scope,
			Id:    aws.String(wa.Status.ID),
		})
		if err != nil && !wafhelper.IsNotFound(err) {
			return fmt.Errorf("get web acl: %w", err)
		}
		if err == nil {
			wa.Status.LockToken = aws.ToString(out.LockToken)
			wa.Status.ObservedGeneration = wa.Generation
			now := metav1.Now()
			wa.Status.LastSyncTime = &now
			return r.setConditionWA(ctx, wa, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "WebACL reconciled")
		}
		wa.Status.ID = ""
		wa.Status.ARN = ""
	}

	defaultAction := &waftypes.DefaultAction{}
	if wa.Spec.DefaultAction.Allow {
		defaultAction.Allow = &waftypes.AllowAction{}
	} else {
		defaultAction.Block = &waftypes.BlockAction{}
	}

	input := &awswafv2.CreateWebACLInput{
		Name:          aws.String(wa.Spec.Name),
		Scope:         scope,
		DefaultAction: defaultAction,
		VisibilityConfig: &waftypes.VisibilityConfig{
			CloudWatchMetricsEnabled: true,
			MetricName:               aws.String(wa.Spec.Name),
			SampledRequestsEnabled:   true,
		},
	}
	if wa.Spec.Description != "" {
		input.Description = aws.String(wa.Spec.Description)
	}
	if len(wa.Spec.Tags) > 0 {
		tags := make([]waftypes.Tag, 0, len(wa.Spec.Tags))
		for k, v := range wa.Spec.Tags {
			k, v := k, v
			tags = append(tags, waftypes.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.WAFv2Client.CreateWebACL(ctx, input)
	if err != nil {
		return fmt.Errorf("create web acl: %w", err)
	}

	wa.Status.ID = aws.ToString(out.Summary.Id)
	wa.Status.ARN = aws.ToString(out.Summary.ARN)
	wa.Status.LockToken = aws.ToString(out.Summary.LockToken)
	wa.Status.ObservedGeneration = wa.Generation
	now := metav1.Now()
	wa.Status.LastSyncTime = &now
	return r.setConditionWA(ctx, wa, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "WebACL created")
}

func (r *WebACLReconciler) deleteWebACL(ctx context.Context, wa *awsv1alpha1.WebACL) error {
	if wa.Status.ID == "" {
		return nil
	}
	_, err := r.WAFv2Client.DeleteWebACL(ctx, &awswafv2.DeleteWebACLInput{
		Name:      aws.String(wa.Spec.Name),
		Scope:     waftypes.Scope(wa.Spec.Scope),
		Id:        aws.String(wa.Status.ID),
		LockToken: aws.String(wa.Status.LockToken),
	})
	if wafhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *WebACLReconciler) setConditionWA(ctx context.Context, wa *awsv1alpha1.WebACL, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&wa.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: wa.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, wa); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *WebACLReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.WebACL{}).
		Complete(r)
}
