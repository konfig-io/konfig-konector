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
	wafhelper "github.com/konfig-io/konfig-konector/internal/aws/wafv2"
)

// WAFRegexPatternSetReconciler reconciles WAFRegexPatternSet objects.
type WAFRegexPatternSetReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	WAFv2Client *awswafv2.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=wafregexpatternsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=wafregexpatternsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=wafregexpatternsets/finalizers,verbs=update

func (r *WAFRegexPatternSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.WAFRegexPatternSet{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
			if shouldAbandon(obj) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, obj)
			}
			if err := r.deleteRegexPatternSet(ctx, obj); err != nil {
				logger.Error(err, "failed to delete WAFRegexPatternSet")
				return ctrl.Result{}, err
			}
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
	}

	if err := r.reconcileRegexPatternSet(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionRPS(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *WAFRegexPatternSetReconciler) reconcileRegexPatternSet(ctx context.Context, obj *awsv1alpha1.WAFRegexPatternSet) error {
	scope := waftypes.Scope(obj.Spec.Scope)
	regexList := buildRegexList(obj.Spec.RegularExpressionList)

	if obj.Status.ID != "" {
		out, err := r.WAFv2Client.GetRegexPatternSet(ctx, &awswafv2.GetRegexPatternSetInput{
			Name:  aws.String(obj.Spec.Name),
			Scope: scope,
			Id:    aws.String(obj.Status.ID),
		})
		if err != nil && !wafhelper.IsNotFound(err) {
			return fmt.Errorf("get regex pattern set: %w", err)
		}
		if err == nil {
			lockToken := aws.ToString(out.LockToken)
			updateInput := &awswafv2.UpdateRegexPatternSetInput{
				Name:                  aws.String(obj.Spec.Name),
				Scope:                 scope,
				Id:                    aws.String(obj.Status.ID),
				LockToken:             aws.String(lockToken),
				RegularExpressionList: regexList,
			}
			if obj.Spec.Description != "" {
				updateInput.Description = aws.String(obj.Spec.Description)
			}
			updateOut, err := r.WAFv2Client.UpdateRegexPatternSet(ctx, updateInput)
			if err != nil {
				return fmt.Errorf("update regex pattern set: %w", err)
			}
			obj.Status.LockToken = aws.ToString(updateOut.NextLockToken)
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionRPS(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "WAFRegexPatternSet reconciled")
		}
		obj.Status.ID = ""
		obj.Status.ARN = ""
	}

	input := &awswafv2.CreateRegexPatternSetInput{
		Name:                  aws.String(obj.Spec.Name),
		Scope:                 scope,
		RegularExpressionList: regexList,
	}
	if obj.Spec.Description != "" {
		input.Description = aws.String(obj.Spec.Description)
	}
	if len(obj.Spec.Tags) > 0 {
		tags := make([]waftypes.Tag, 0, len(obj.Spec.Tags))
		for k, v := range obj.Spec.Tags {
			k, v := k, v
			tags = append(tags, waftypes.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.WAFv2Client.CreateRegexPatternSet(ctx, input)
	if err != nil {
		return fmt.Errorf("create regex pattern set: %w", err)
	}

	obj.Status.ID = aws.ToString(out.Summary.Id)
	obj.Status.ARN = aws.ToString(out.Summary.ARN)
	obj.Status.LockToken = aws.ToString(out.Summary.LockToken)
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionRPS(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "WAFRegexPatternSet created")
}

func (r *WAFRegexPatternSetReconciler) deleteRegexPatternSet(ctx context.Context, obj *awsv1alpha1.WAFRegexPatternSet) error {
	if obj.Status.ID == "" {
		return nil
	}
	// Refresh lock token before delete.
	getOut, err := r.WAFv2Client.GetRegexPatternSet(ctx, &awswafv2.GetRegexPatternSetInput{
		Name:  aws.String(obj.Spec.Name),
		Scope: waftypes.Scope(obj.Spec.Scope),
		Id:    aws.String(obj.Status.ID),
	})
	if wafhelper.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = r.WAFv2Client.DeleteRegexPatternSet(ctx, &awswafv2.DeleteRegexPatternSetInput{
		Name:      aws.String(obj.Spec.Name),
		Scope:     waftypes.Scope(obj.Spec.Scope),
		Id:        aws.String(obj.Status.ID),
		LockToken: getOut.LockToken,
	})
	if wafhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func buildRegexList(patterns []string) []waftypes.Regex {
	out := make([]waftypes.Regex, 0, len(patterns))
	for _, p := range patterns {
		p := p
		out = append(out, waftypes.Regex{RegexString: &p})
	}
	return out
}

func (r *WAFRegexPatternSetReconciler) setConditionRPS(ctx context.Context, obj *awsv1alpha1.WAFRegexPatternSet, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *WAFRegexPatternSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.WAFRegexPatternSet{}).
		Complete(r)
}
