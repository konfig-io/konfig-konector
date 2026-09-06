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
	awscfn "github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	cfnhelper "github.com/konfig-io/konfig-konector/internal/aws/cloudformation"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// CloudFormationStackSetReconciler reconciles CloudFormationStackSet objects.
type CloudFormationStackSetReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	CloudFormationClient *multi.CloudFormation
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudformationstacksets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudformationstacksets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudformationstacksets/finalizers,verbs=update

func (r *CloudFormationStackSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.CloudFormationStackSet{}
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
			if err := r.deleteStackSet(ctx, obj); err != nil {
				logger.Error(err, "failed to delete CloudFormationStackSet")
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

	if err := r.reconcileStackSet(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionCFNSS(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *CloudFormationStackSetReconciler) reconcileStackSet(ctx context.Context, obj *awsv1alpha1.CloudFormationStackSet) error {
	params := buildCFNParameters(obj.Spec.Parameters)
	caps := buildCFNCapabilities(obj.Spec.Capabilities)
	tags := buildCFNTags(obj.Spec.Tags)

	descOut, err := r.CloudFormationClient.DescribeStackSet(ctx, &awscfn.DescribeStackSetInput{
		StackSetName: aws.String(obj.Spec.StackSetName),
	})
	if err != nil && !cfnhelper.IsNotFound(err) {
		return fmt.Errorf("describe cloudformation stackset: %w", err)
	}

	if err == nil && descOut.StackSet != nil {
		obj.Status.StackSetID = aws.ToString(descOut.StackSet.StackSetId)
		obj.Status.StackSetStatus = string(descOut.StackSet.Status)

		updateInput := &awscfn.UpdateStackSetInput{
			StackSetName: aws.String(obj.Spec.StackSetName),
			Parameters:   params,
			Capabilities: caps,
			Tags:         tags,
		}
		if obj.Spec.TemplateBody != "" {
			updateInput.TemplateBody = aws.String(obj.Spec.TemplateBody)
		} else if obj.Spec.TemplateURL != "" {
			updateInput.TemplateURL = aws.String(obj.Spec.TemplateURL)
		}
		if obj.Spec.Description != "" {
			updateInput.Description = aws.String(obj.Spec.Description)
		}
		if _, err := r.CloudFormationClient.UpdateStackSet(ctx, updateInput); err != nil {
			return fmt.Errorf("update cloudformation stackset: %w", err)
		}

		obj.Status.ObservedGeneration = obj.Generation
		now := metav1.Now()
		obj.Status.LastSyncTime = &now
		return r.setConditionCFNSS(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "CloudFormationStackSet reconciled")
	}

	createInput := &awscfn.CreateStackSetInput{
		StackSetName: aws.String(obj.Spec.StackSetName),
		Parameters:   params,
		Capabilities: caps,
		Tags:         tags,
	}
	if obj.Spec.TemplateBody != "" {
		createInput.TemplateBody = aws.String(obj.Spec.TemplateBody)
	} else if obj.Spec.TemplateURL != "" {
		createInput.TemplateURL = aws.String(obj.Spec.TemplateURL)
	}
	if obj.Spec.Description != "" {
		createInput.Description = aws.String(obj.Spec.Description)
	}
	out, err := r.CloudFormationClient.CreateStackSet(ctx, createInput)
	if err != nil {
		return fmt.Errorf("create cloudformation stackset: %w", err)
	}

	obj.Status.StackSetID = aws.ToString(out.StackSetId)
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionCFNSS(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "CloudFormationStackSet created")
}

func (r *CloudFormationStackSetReconciler) deleteStackSet(ctx context.Context, obj *awsv1alpha1.CloudFormationStackSet) error {
	_, err := r.CloudFormationClient.DeleteStackSet(ctx, &awscfn.DeleteStackSetInput{
		StackSetName: aws.String(obj.Spec.StackSetName),
	})
	if cfnhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *CloudFormationStackSetReconciler) setConditionCFNSS(ctx context.Context, obj *awsv1alpha1.CloudFormationStackSet, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *CloudFormationStackSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CloudFormationStackSet{}).
		Complete(r)
}
