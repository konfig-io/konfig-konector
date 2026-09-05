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
	awsglue "github.com/aws/aws-sdk-go-v2/service/glue"
	gluetypes "github.com/aws/aws-sdk-go-v2/service/glue/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	gluehelper "github.com/konfig-io/konfig-konector/internal/aws/glue"
)

// GlueTriggerAWSAPI is the subset of the Glue SDK client used by this
// controller. *awsglue.Client satisfies it.
type GlueTriggerAWSAPI interface {
	GetTrigger(ctx context.Context, params *awsglue.GetTriggerInput, optFns ...func(*awsglue.Options)) (*awsglue.GetTriggerOutput, error)
	CreateTrigger(ctx context.Context, params *awsglue.CreateTriggerInput, optFns ...func(*awsglue.Options)) (*awsglue.CreateTriggerOutput, error)
	UpdateTrigger(ctx context.Context, params *awsglue.UpdateTriggerInput, optFns ...func(*awsglue.Options)) (*awsglue.UpdateTriggerOutput, error)
	DeleteTrigger(ctx context.Context, params *awsglue.DeleteTriggerInput, optFns ...func(*awsglue.Options)) (*awsglue.DeleteTriggerOutput, error)
}

// GlueTriggerReconciler reconciles GlueTrigger objects.
type GlueTriggerReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	GlueClient GlueTriggerAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=gluetriggers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=gluetriggers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=gluetriggers/finalizers,verbs=update

func (r *GlueTriggerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	tr := &awsv1alpha1.GlueTrigger{}
	if err := r.Get(ctx, req.NamespacedName, tr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, tr); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !tr.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(tr, awsv1alpha1.FinalizerName) {
			if shouldAbandon(tr) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(tr, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, tr)
			}
			if err := r.deleteTrigger(ctx, tr); err != nil {
				logger.Error(err, "failed to delete Glue trigger")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(tr, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, tr)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(tr, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(tr, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, tr); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileTrigger(ctx, tr); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, tr, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func triggerActions(spec []awsv1alpha1.GlueTriggerAction) []gluetypes.Action {
	actions := make([]gluetypes.Action, 0, len(spec))
	for _, a := range spec {
		action := gluetypes.Action{
			JobName: aws.String(a.JobName),
		}
		if len(a.Arguments) > 0 {
			action.Arguments = a.Arguments
		}
		actions = append(actions, action)
	}
	return actions
}

func triggerPredicate(spec *awsv1alpha1.GlueTriggerPredicate) *gluetypes.Predicate {
	if spec == nil {
		return nil
	}
	p := &gluetypes.Predicate{
		Logical: gluetypes.Logical(spec.Logical),
	}
	for _, c := range spec.Conditions {
		cond := gluetypes.Condition{
			LogicalOperator: gluetypes.LogicalOperator(c.LogicalOperator),
			State:           gluetypes.JobRunState(c.State),
		}
		if c.JobName != "" {
			cond.JobName = aws.String(c.JobName)
		}
		if c.CrawlerName != "" {
			cond.CrawlerName = aws.String(c.CrawlerName)
		}
		p.Conditions = append(p.Conditions, cond)
	}
	return p
}

func (r *GlueTriggerReconciler) reconcileTrigger(ctx context.Context, tr *awsv1alpha1.GlueTrigger) error {
	getOut, err := r.GlueClient.GetTrigger(ctx, &awsglue.GetTriggerInput{
		Name: aws.String(tr.Spec.Name),
	})
	if gluehelper.IsNotFound(err) {
		input := &awsglue.CreateTriggerInput{
			Name:            aws.String(tr.Spec.Name),
			Type:            gluetypes.TriggerType(tr.Spec.Type),
			Actions:         triggerActions(tr.Spec.Actions),
			Predicate:       triggerPredicate(tr.Spec.Predicate),
			StartOnCreation: tr.Spec.StartOnCreation,
			Tags:            tr.Spec.Tags,
		}
		if tr.Spec.Schedule != "" {
			input.Schedule = aws.String(tr.Spec.Schedule)
		}
		if tr.Spec.Description != "" {
			input.Description = aws.String(tr.Spec.Description)
		}
		if _, err := r.GlueClient.CreateTrigger(ctx, input); err != nil {
			return fmt.Errorf("create Glue trigger: %w", err)
		}
		// Persist the identifier immediately: the AWS resource now exists.
		tr.Status.TriggerName = tr.Spec.Name
		if err := persistStatus(ctx, r.Client, tr); err != nil {
			return fmt.Errorf("persist trigger name after create: %w", err)
		}
	} else if err != nil {
		return err
	} else {
		if getOut.Trigger != nil {
			tr.Status.State = string(getOut.Trigger.State)
		}
		if tr.Status.ObservedGeneration != tr.Generation {
			update := &gluetypes.TriggerUpdate{
				Actions:   triggerActions(tr.Spec.Actions),
				Predicate: triggerPredicate(tr.Spec.Predicate),
			}
			if tr.Spec.Schedule != "" {
				update.Schedule = aws.String(tr.Spec.Schedule)
			}
			if tr.Spec.Description != "" {
				update.Description = aws.String(tr.Spec.Description)
			}
			if _, err := r.GlueClient.UpdateTrigger(ctx, &awsglue.UpdateTriggerInput{
				Name:          aws.String(tr.Spec.Name),
				TriggerUpdate: update,
			}); err != nil {
				return fmt.Errorf("update Glue trigger: %w", err)
			}
		}
	}

	tr.Status.TriggerName = tr.Spec.Name
	tr.Status.ObservedGeneration = tr.Generation
	now := metav1.Now()
	tr.Status.LastSyncTime = &now
	return r.setCondition(ctx, tr, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Glue trigger reconciled")
}

func (r *GlueTriggerReconciler) deleteTrigger(ctx context.Context, tr *awsv1alpha1.GlueTrigger) error {
	name := tr.Status.TriggerName
	if name == "" {
		name = tr.Spec.Name
	}
	_, err := r.GlueClient.DeleteTrigger(ctx, &awsglue.DeleteTriggerInput{
		Name: aws.String(name),
	})
	if gluehelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *GlueTriggerReconciler) setCondition(ctx context.Context, tr *awsv1alpha1.GlueTrigger, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&tr.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: tr.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, tr); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *GlueTriggerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.GlueTrigger{}).
		Complete(r)
}
