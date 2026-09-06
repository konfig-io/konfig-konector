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
	awsconfigservice "github.com/aws/aws-sdk-go-v2/service/configservice"
	configtypes "github.com/aws/aws-sdk-go-v2/service/configservice/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	confighelper "github.com/konfig-io/konfig-konector/internal/aws/configservice"
)

// ConfigRuleAWSAPI is the subset of the AWS Config API used by this controller.
type ConfigRuleAWSAPI interface {
	DescribeConfigRules(ctx context.Context, params *awsconfigservice.DescribeConfigRulesInput, optFns ...func(*awsconfigservice.Options)) (*awsconfigservice.DescribeConfigRulesOutput, error)
	PutConfigRule(ctx context.Context, params *awsconfigservice.PutConfigRuleInput, optFns ...func(*awsconfigservice.Options)) (*awsconfigservice.PutConfigRuleOutput, error)
	DeleteConfigRule(ctx context.Context, params *awsconfigservice.DeleteConfigRuleInput, optFns ...func(*awsconfigservice.Options)) (*awsconfigservice.DeleteConfigRuleOutput, error)
}

// ConfigRuleReconciler reconciles ConfigRule objects.
type ConfigRuleReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	ConfigClient ConfigRuleAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=configrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=configrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=configrules/finalizers,verbs=update

func (r *ConfigRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	rule := &awsv1alpha1.ConfigRule{}
	if err := r.Get(ctx, req.NamespacedName, rule); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, rule); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !rule.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(rule, awsv1alpha1.FinalizerName) {
			if shouldAbandon(rule) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(rule, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, rule)
			}
			if err := r.deleteRule(ctx, rule); err != nil {
				logger.Error(err, "failed to delete config rule")
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
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
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

func (r *ConfigRuleReconciler) reconcileRule(ctx context.Context, rule *awsv1alpha1.ConfigRule) error {
	cfgRule := configtypes.ConfigRule{
		ConfigRuleName: aws.String(rule.Spec.RuleName),
		Description:    govOptionalStr(rule.Spec.Description),
		Source: &configtypes.Source{
			Owner:            configtypes.Owner(rule.Spec.SourceOwner),
			SourceIdentifier: aws.String(rule.Spec.SourceIdentifier),
		},
		InputParameters: govOptionalStr(rule.Spec.InputParameters),
	}
	if rule.Spec.MaximumExecutionFrequency != "" {
		cfgRule.MaximumExecutionFrequency = configtypes.MaximumExecutionFrequency(rule.Spec.MaximumExecutionFrequency)
	}
	if sc := rule.Spec.Scope; sc != nil {
		cfgRule.Scope = &configtypes.Scope{
			ComplianceResourceTypes: sc.ComplianceResourceTypes,
			TagKey:                  govOptionalStr(sc.TagKey),
			TagValue:                govOptionalStr(sc.TagValue),
		}
	}

	// PutConfigRule is an idempotent upsert.
	if _, err := r.ConfigClient.PutConfigRule(ctx, &awsconfigservice.PutConfigRuleInput{
		ConfigRule: &cfgRule,
	}); err != nil {
		return fmt.Errorf("put config rule: %w", err)
	}

	// Read back the rule to store its ARN/ID.
	descOut, err := r.ConfigClient.DescribeConfigRules(ctx, &awsconfigservice.DescribeConfigRulesInput{
		ConfigRuleNames: []string{rule.Spec.RuleName},
	})
	if err != nil {
		return fmt.Errorf("describe config rule: %w", err)
	}
	if len(descOut.ConfigRules) > 0 {
		rule.Status.RuleARN = aws.ToString(descOut.ConfigRules[0].ConfigRuleArn)
		rule.Status.RuleID = aws.ToString(descOut.ConfigRules[0].ConfigRuleId)
	}

	rule.Status.ObservedGeneration = rule.Generation
	now := metav1.Now()
	rule.Status.LastSyncTime = &now
	return r.setCondition(ctx, rule, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "config rule reconciled")
}

func (r *ConfigRuleReconciler) deleteRule(ctx context.Context, rule *awsv1alpha1.ConfigRule) error {
	// The rule name is a deterministic spec-based identifier.
	_, err := r.ConfigClient.DeleteConfigRule(ctx, &awsconfigservice.DeleteConfigRuleInput{
		ConfigRuleName: aws.String(rule.Spec.RuleName),
	})
	if confighelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ConfigRuleReconciler) setCondition(ctx context.Context, rule *awsv1alpha1.ConfigRule, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *ConfigRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ConfigRule{}).
		Complete(r)
}
