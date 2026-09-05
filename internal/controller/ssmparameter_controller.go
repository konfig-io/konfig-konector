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
	awsssm "github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ssmhelper "github.com/konfig-io/konfig-konector/internal/aws/ssm"
)

// SSMParameterAWSAPI is the subset of the SSM API used by this controller.
type SSMParameterAWSAPI interface {
	GetParameter(ctx context.Context, params *awsssm.GetParameterInput, optFns ...func(*awsssm.Options)) (*awsssm.GetParameterOutput, error)
	PutParameter(ctx context.Context, params *awsssm.PutParameterInput, optFns ...func(*awsssm.Options)) (*awsssm.PutParameterOutput, error)
	DeleteParameter(ctx context.Context, params *awsssm.DeleteParameterInput, optFns ...func(*awsssm.Options)) (*awsssm.DeleteParameterOutput, error)
}

// SSMParameterReconciler reconciles SSMParameter objects.
type SSMParameterReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	SSMClient SSMParameterAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=ssmparameters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ssmparameters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ssmparameters/finalizers,verbs=update

func (r *SSMParameterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	p := &awsv1alpha1.SSMParameter{}
	if err := r.Get(ctx, req.NamespacedName, p); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, p); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !p.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(p, awsv1alpha1.FinalizerName) {
			if shouldAbandon(p) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(p, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, p)
			}
			if err := r.deleteSSMParameter(ctx, p); err != nil {
				logger.Error(err, "failed to delete SSMParameter")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(p, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, p)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(p, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(p, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, p); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileSSMParameter(ctx, p); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionSSM(ctx, p, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *SSMParameterReconciler) reconcileSSMParameter(ctx context.Context, p *awsv1alpha1.SSMParameter) error {
	value := p.Spec.Value
	if p.Spec.ValueFrom != nil {
		v, err := resolveSecretValue(ctx, r.Client, p.Namespace, *p.Spec.ValueFrom)
		if err != nil {
			return fmt.Errorf("resolve valueFrom: %w", err)
		}
		value = v
	}

	if p.Status.ARN != "" {
		out, err := r.SSMClient.GetParameter(ctx, &awsssm.GetParameterInput{
			Name:           aws.String(p.Spec.ParameterName),
			WithDecryption: aws.Bool(true),
		})
		if err != nil && !ssmhelper.IsNotFound(err) {
			return fmt.Errorf("get parameter: %w", err)
		}
		// Fall through to PutParameter when the value drifted (e.g. the source
		// Secret was rotated) so the change actually propagates.
		if err == nil && aws.ToString(out.Parameter.Value) == value {
			p.Status.Version = out.Parameter.Version
			p.Status.ObservedGeneration = p.Generation
			now := metav1.Now()
			p.Status.LastSyncTime = &now
			return r.setConditionSSM(ctx, p, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "SSMParameter reconciled")
		}
		if err != nil {
			p.Status.ARN = ""
		}
	}
	// Updating an existing parameter requires Overwrite, and PutParameter
	// rejects Tags combined with Overwrite — tags only apply on first create.
	updating := p.Status.ARN != ""
	input := &awsssm.PutParameterInput{
		Name:      aws.String(p.Spec.ParameterName),
		Type:      ssmtypes.ParameterType(p.Spec.Type),
		Value:     aws.String(value),
		Overwrite: aws.Bool(updating || p.Spec.Overwrite),
	}
	if p.Spec.Description != "" {
		input.Description = aws.String(p.Spec.Description)
	}
	if p.Spec.KMSKeyID != "" {
		input.KeyId = aws.String(p.Spec.KMSKeyID)
	}
	if p.Spec.Tier != "" {
		input.Tier = ssmtypes.ParameterTier(p.Spec.Tier)
	}
	if len(p.Spec.Tags) > 0 && !aws.ToBool(input.Overwrite) {
		tags := make([]ssmtypes.Tag, 0, len(p.Spec.Tags))
		for k, v := range p.Spec.Tags {
			k, v := k, v
			tags = append(tags, ssmtypes.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.SSMClient.PutParameter(ctx, input)
	if err != nil {
		return fmt.Errorf("put parameter: %w", err)
	}

	p.Status.Version = out.Version
	descOut, err2 := r.SSMClient.GetParameter(ctx, &awsssm.GetParameterInput{
		Name: aws.String(p.Spec.ParameterName),
	})
	if err2 == nil {
		p.Status.ARN = aws.ToString(descOut.Parameter.ARN)
	}

	p.Status.ObservedGeneration = p.Generation
	now := metav1.Now()
	p.Status.LastSyncTime = &now
	return r.setConditionSSM(ctx, p, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "SSMParameter created")
}

func (r *SSMParameterReconciler) deleteSSMParameter(ctx context.Context, p *awsv1alpha1.SSMParameter) error {
	if p.Spec.ParameterName == "" {
		return nil
	}
	_, err := r.SSMClient.DeleteParameter(ctx, &awsssm.DeleteParameterInput{
		Name: aws.String(p.Spec.ParameterName),
	})
	if ssmhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SSMParameterReconciler) setConditionSSM(ctx context.Context, p *awsv1alpha1.SSMParameter, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&p.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: p.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, p); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *SSMParameterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.SSMParameter{}).
		Complete(r)
}
