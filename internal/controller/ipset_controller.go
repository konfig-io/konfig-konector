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

// IPSetReconciler reconciles IPSet objects.
type IPSetReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	WAFv2Client *multi.WAFv2
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=ipsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ipsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ipsets/finalizers,verbs=update

func (r *IPSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ip := &awsv1alpha1.IPSet{}
	if err := r.Get(ctx, req.NamespacedName, ip); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, ip); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !ip.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ip, awsv1alpha1.FinalizerName) {
			if shouldAbandon(ip) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(ip, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, ip)
			}
			if err := r.deleteIPSet(ctx, ip); err != nil {
				logger.Error(err, "failed to delete IPSet")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(ip, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, ip)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(ip, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(ip, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, ip); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileIPSet(ctx, ip); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionIPSet(ctx, ip, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *IPSetReconciler) reconcileIPSet(ctx context.Context, ip *awsv1alpha1.IPSet) error {
	scope := waftypes.Scope(ip.Spec.Scope)

	if ip.Status.ID != "" {
		out, err := r.WAFv2Client.GetIPSet(ctx, &awswafv2.GetIPSetInput{
			Name:  aws.String(ip.Spec.Name),
			Scope: scope,
			Id:    aws.String(ip.Status.ID),
		})
		if err != nil && !wafhelper.IsNotFound(err) {
			return fmt.Errorf("get ip set: %w", err)
		}
		if err == nil {
			ip.Status.LockToken = aws.ToString(out.LockToken)
			ip.Status.ObservedGeneration = ip.Generation
			now := metav1.Now()
			ip.Status.LastSyncTime = &now
			return r.setConditionIPSet(ctx, ip, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "IPSet reconciled")
		}
		ip.Status.ID = ""
		ip.Status.ARN = ""
	}

	input := &awswafv2.CreateIPSetInput{
		Name:             aws.String(ip.Spec.Name),
		Scope:            scope,
		IPAddressVersion: waftypes.IPAddressVersion(ip.Spec.IPAddressVersion),
		Addresses:        ip.Spec.Addresses,
	}
	if ip.Spec.Description != "" {
		input.Description = aws.String(ip.Spec.Description)
	}
	if len(ip.Spec.Tags) > 0 {
		tags := make([]waftypes.Tag, 0, len(ip.Spec.Tags))
		for k, v := range ip.Spec.Tags {
			k, v := k, v
			tags = append(tags, waftypes.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.WAFv2Client.CreateIPSet(ctx, input)
	if err != nil {
		return fmt.Errorf("create ip set: %w", err)
	}

	ip.Status.ID = aws.ToString(out.Summary.Id)
	ip.Status.ARN = aws.ToString(out.Summary.ARN)
	ip.Status.LockToken = aws.ToString(out.Summary.LockToken)
	ip.Status.ObservedGeneration = ip.Generation
	now := metav1.Now()
	ip.Status.LastSyncTime = &now
	return r.setConditionIPSet(ctx, ip, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "IPSet created")
}

func (r *IPSetReconciler) deleteIPSet(ctx context.Context, ip *awsv1alpha1.IPSet) error {
	if ip.Status.ID == "" {
		return nil
	}
	_, err := r.WAFv2Client.DeleteIPSet(ctx, &awswafv2.DeleteIPSetInput{
		Name:      aws.String(ip.Spec.Name),
		Scope:     waftypes.Scope(ip.Spec.Scope),
		Id:        aws.String(ip.Status.ID),
		LockToken: aws.String(ip.Status.LockToken),
	})
	if wafhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *IPSetReconciler) setConditionIPSet(ctx context.Context, ip *awsv1alpha1.IPSet, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&ip.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: ip.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, ip); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *IPSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.IPSet{}).
		Complete(r)
}
