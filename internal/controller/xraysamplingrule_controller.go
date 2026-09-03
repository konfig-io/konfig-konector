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

// XRaySamplingRuleAWSAPI is the subset of the X-Ray API used by this controller.
type XRaySamplingRuleAWSAPI interface {
	GetSamplingRules(ctx context.Context, params *awsxray.GetSamplingRulesInput, optFns ...func(*awsxray.Options)) (*awsxray.GetSamplingRulesOutput, error)
	CreateSamplingRule(ctx context.Context, params *awsxray.CreateSamplingRuleInput, optFns ...func(*awsxray.Options)) (*awsxray.CreateSamplingRuleOutput, error)
	UpdateSamplingRule(ctx context.Context, params *awsxray.UpdateSamplingRuleInput, optFns ...func(*awsxray.Options)) (*awsxray.UpdateSamplingRuleOutput, error)
	DeleteSamplingRule(ctx context.Context, params *awsxray.DeleteSamplingRuleInput, optFns ...func(*awsxray.Options)) (*awsxray.DeleteSamplingRuleOutput, error)
}

// XRaySamplingRuleReconciler reconciles XRaySamplingRule objects.
type XRaySamplingRuleReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	XRayClient XRaySamplingRuleAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=xraysamplingrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=xraysamplingrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=xraysamplingrules/finalizers,verbs=update

func (r *XRaySamplingRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	rule := &awsv1alpha1.XRaySamplingRule{}
	if err := r.Get(ctx, req.NamespacedName, rule); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !rule.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(rule, awsv1alpha1.FinalizerName) {
			if shouldAbandon(rule) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(rule, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, rule)
			}
			if err := r.deleteRule(ctx, rule); err != nil {
				logger.Error(err, "failed to delete X-Ray sampling rule")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(rule, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, rule)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(rule, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(rule, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, rule); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileRule(ctx, rule); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, rule, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

// findRule looks up the sampling rule by name; nil when not present.
func (r *XRaySamplingRuleReconciler) findRule(ctx context.Context, name string) (*xraytypes.SamplingRule, error) {
	var next *string
	for {
		out, err := r.XRayClient.GetSamplingRules(ctx, &awsxray.GetSamplingRulesInput{NextToken: next})
		if err != nil {
			return nil, err
		}
		for _, rec := range out.SamplingRuleRecords {
			if rec.SamplingRule != nil && aws.ToString(rec.SamplingRule.RuleName) == name {
				return rec.SamplingRule, nil
			}
		}
		if out.NextToken == nil {
			return nil, nil
		}
		next = out.NextToken
	}
}

// matcherOrStar returns s, defaulting to "*" (match all) when empty; the
// X-Ray API requires every matcher field to be set.
func matcherOrStar(s string) *string {
	if s == "" {
		return aws.String("*")
	}
	return aws.String(s)
}

func (r *XRaySamplingRuleReconciler) reconcileRule(ctx context.Context, rule *awsv1alpha1.XRaySamplingRule) error {
	existing, err := r.findRule(ctx, rule.Spec.RuleName)
	if err != nil {
		return err
	}

	if existing == nil {
		in := &awsxray.CreateSamplingRuleInput{
			SamplingRule: &xraytypes.SamplingRule{
				RuleName:      aws.String(rule.Spec.RuleName),
				Priority:      aws.Int32(rule.Spec.Priority),
				FixedRate:     rule.Spec.FixedRate,
				ReservoirSize: rule.Spec.ReservoirSize,
				ServiceName:   matcherOrStar(rule.Spec.ServiceName),
				ServiceType:   matcherOrStar(rule.Spec.ServiceType),
				Host:          matcherOrStar(rule.Spec.Host),
				HTTPMethod:    matcherOrStar(rule.Spec.HTTPMethod),
				URLPath:       matcherOrStar(rule.Spec.URLPath),
				ResourceARN:   matcherOrStar(rule.Spec.ResourceARN),
				Version:       aws.Int32(1),
			},
		}
		for k, v := range rule.Spec.Tags {
			in.Tags = append(in.Tags, xraytypes.Tag{Key: aws.String(k), Value: aws.String(v)})
		}
		created, err := r.XRayClient.CreateSamplingRule(ctx, in)
		if err != nil {
			return fmt.Errorf("create X-Ray sampling rule: %w", err)
		}
		if created.SamplingRuleRecord != nil && created.SamplingRuleRecord.SamplingRule != nil {
			rule.Status.ARN = aws.ToString(created.SamplingRuleRecord.SamplingRule.RuleARN)
		}
		// Persist the ARN immediately: the AWS resource now exists, and losing
		// the identifier would orphan it on delete.
		if err := persistStatus(ctx, r.Client, rule); err != nil {
			return fmt.Errorf("persist sampling rule ARN after create: %w", err)
		}
	} else {
		rule.Status.ARN = aws.ToString(existing.RuleARN)
		if rule.Status.ObservedGeneration != rule.Generation {
			in := &awsxray.UpdateSamplingRuleInput{
				SamplingRuleUpdate: &xraytypes.SamplingRuleUpdate{
					RuleName:      aws.String(rule.Spec.RuleName),
					Priority:      aws.Int32(rule.Spec.Priority),
					FixedRate:     aws.Float64(rule.Spec.FixedRate),
					ReservoirSize: aws.Int32(rule.Spec.ReservoirSize),
					ServiceName:   matcherOrStar(rule.Spec.ServiceName),
					ServiceType:   matcherOrStar(rule.Spec.ServiceType),
					Host:          matcherOrStar(rule.Spec.Host),
					HTTPMethod:    matcherOrStar(rule.Spec.HTTPMethod),
					URLPath:       matcherOrStar(rule.Spec.URLPath),
					ResourceARN:   matcherOrStar(rule.Spec.ResourceARN),
				},
			}
			if _, err := r.XRayClient.UpdateSamplingRule(ctx, in); err != nil {
				return fmt.Errorf("update X-Ray sampling rule: %w", err)
			}
		}
	}

	rule.Status.ObservedGeneration = rule.Generation
	now := metav1.Now()
	rule.Status.LastSyncTime = &now
	return r.setCondition(ctx, rule, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "X-Ray sampling rule reconciled")
}

func (r *XRaySamplingRuleReconciler) deleteRule(ctx context.Context, rule *awsv1alpha1.XRaySamplingRule) error {
	in := &awsxray.DeleteSamplingRuleInput{}
	if rule.Status.ARN != "" {
		in.RuleARN = aws.String(rule.Status.ARN)
	} else {
		// The rule name in spec is the deterministic AWS identifier.
		in.RuleName = aws.String(rule.Spec.RuleName)
	}
	_, err := r.XRayClient.DeleteSamplingRule(ctx, in)
	if xrayhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *XRaySamplingRuleReconciler) setCondition(ctx context.Context, rule *awsv1alpha1.XRaySamplingRule, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&rule.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: rule.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, rule); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *XRaySamplingRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.XRaySamplingRule{}).
		Complete(r)
}
