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
	awssfn "github.com/aws/aws-sdk-go-v2/service/sfn"
	sfntypes "github.com/aws/aws-sdk-go-v2/service/sfn/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
	sfnhelper "github.com/konfig-io/konfig-konector/internal/aws/sfn"
)

// StateMachineReconciler reconciles StateMachine objects.
type StateMachineReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	SFNClient *multi.SFN
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=statemachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=statemachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=statemachines/finalizers,verbs=update

func (r *StateMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.StateMachine{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, obj); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !obj.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
			if shouldAbandon(obj) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, obj)
			}
			if err := r.deleteStateMachine(ctx, obj); err != nil {
				logger.Error(err, "failed to delete StateMachine")
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
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileStateMachine(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionSM(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *StateMachineReconciler) reconcileStateMachine(ctx context.Context, obj *awsv1alpha1.StateMachine) error {
	if obj.Status.StateMachineARN != "" {
		descOut, err := r.SFNClient.DescribeStateMachine(ctx, &awssfn.DescribeStateMachineInput{
			StateMachineArn: aws.String(obj.Status.StateMachineARN),
		})
		if err != nil && !sfnhelper.IsNotFound(err) {
			return fmt.Errorf("describe state machine: %w", err)
		}
		if err == nil && descOut.StateMachineArn != nil {
			obj.Status.Status = string(descOut.Status)

			if _, err := r.SFNClient.UpdateStateMachine(ctx, &awssfn.UpdateStateMachineInput{
				StateMachineArn: aws.String(obj.Status.StateMachineARN),
				Definition:      aws.String(obj.Spec.Definition),
				RoleArn:         aws.String(obj.Spec.RoleARN),
			}); err != nil {
				return fmt.Errorf("update state machine: %w", err)
			}
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionSM(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "StateMachine reconciled")
		}
		obj.Status.StateMachineARN = ""
	}

	smType := sfntypes.StateMachineTypeStandard
	if obj.Spec.Type == "EXPRESS" {
		smType = sfntypes.StateMachineTypeExpress
	}

	input := &awssfn.CreateStateMachineInput{
		Name:       aws.String(obj.Spec.Name),
		Definition: aws.String(obj.Spec.Definition),
		RoleArn:    aws.String(obj.Spec.RoleARN),
		Type:       smType,
	}
	if len(obj.Spec.Tags) > 0 {
		tags := make([]sfntypes.Tag, 0, len(obj.Spec.Tags))
		for k, v := range obj.Spec.Tags {
			k, v := k, v
			tags = append(tags, sfntypes.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.SFNClient.CreateStateMachine(ctx, input)
	if err != nil {
		return fmt.Errorf("create state machine: %w", err)
	}

	obj.Status.StateMachineARN = aws.ToString(out.StateMachineArn)
	// Persist the ARN immediately: the AWS resource now exists, and losing the
	// identifier would orphan it or create a duplicate on retry.
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist state machine ARN after create: %w", err)
	}
	obj.Status.Status = "ACTIVE"
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionSM(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "StateMachine created")
}

func (r *StateMachineReconciler) deleteStateMachine(ctx context.Context, obj *awsv1alpha1.StateMachine) error {
	arn := obj.Status.StateMachineARN
	if arn == "" {
		// Status may have been lost before it was persisted; fall back to
		// looking the state machine up by its spec name.
		found, err := r.findStateMachineARNByName(ctx, obj.Spec.Name)
		if err != nil {
			return err
		}
		if found == "" {
			return nil
		}
		arn = found
	}
	_, err := r.SFNClient.DeleteStateMachine(ctx, &awssfn.DeleteStateMachineInput{
		StateMachineArn: aws.String(arn),
	})
	if sfnhelper.IsNotFound(err) {
		return nil
	}
	return err
}

// findStateMachineARNByName lists state machines and returns the ARN of the
// one whose name matches exactly, or "" if none matches.
func (r *StateMachineReconciler) findStateMachineARNByName(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", nil
	}
	var next *string
	for {
		out, err := r.SFNClient.ListStateMachines(ctx, &awssfn.ListStateMachinesInput{NextToken: next})
		if err != nil {
			return "", err
		}
		for _, sm := range out.StateMachines {
			if aws.ToString(sm.Name) == name {
				return aws.ToString(sm.StateMachineArn), nil
			}
		}
		if out.NextToken == nil {
			return "", nil
		}
		next = out.NextToken
	}
}

func (r *StateMachineReconciler) setConditionSM(ctx context.Context, obj *awsv1alpha1.StateMachine, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *StateMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.StateMachine{}).
		Complete(r)
}
