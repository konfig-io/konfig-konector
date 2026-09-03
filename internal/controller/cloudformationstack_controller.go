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
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfn "github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	cfnhelper "github.com/konfig-io/konfig-konector/internal/aws/cloudformation"
)

// CloudFormationStackReconciler reconciles CloudFormationStack objects.
type CloudFormationStackReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	CloudFormationClient *awscfn.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudformationstacks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudformationstacks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudformationstacks/finalizers,verbs=update

func (r *CloudFormationStackReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.CloudFormationStack{}
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
			if err := r.deleteStack(ctx, obj); err != nil {
				logger.Error(err, "failed to delete CloudFormationStack")
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

	if err := r.reconcileStack(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionCFN(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *CloudFormationStackReconciler) reconcileStack(ctx context.Context, obj *awsv1alpha1.CloudFormationStack) error {
	params := buildCFNParameters(obj.Spec.Parameters)
	caps := buildCFNCapabilities(obj.Spec.Capabilities)
	tags := buildCFNTags(obj.Spec.Tags)

	descOut, err := r.CloudFormationClient.DescribeStacks(ctx, &awscfn.DescribeStacksInput{
		StackName: aws.String(obj.Spec.StackName),
	})
	if err != nil && !cfnhelper.IsNotFound(err) {
		return fmt.Errorf("describe cloudformation stack: %w", err)
	}

	if err == nil && len(descOut.Stacks) > 0 {
		stack := descOut.Stacks[0]
		obj.Status.StackID = aws.ToString(stack.StackId)
		obj.Status.StackStatus = string(stack.StackStatus)

		if strings.HasSuffix(string(stack.StackStatus), "_IN_PROGRESS") {
			return r.setConditionCFN(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonCreated, fmt.Sprintf("stack status: %s", stack.StackStatus))
		}

		updateInput := &awscfn.UpdateStackInput{
			StackName:    aws.String(obj.Spec.StackName),
			Parameters:   params,
			Capabilities: caps,
			Tags:         tags,
		}
		if obj.Spec.TemplateBody != "" {
			updateInput.TemplateBody = aws.String(obj.Spec.TemplateBody)
		} else if obj.Spec.TemplateURL != "" {
			updateInput.TemplateURL = aws.String(obj.Spec.TemplateURL)
		}
		if _, err := r.CloudFormationClient.UpdateStack(ctx, updateInput); err != nil {
			// No-op if no updates (ValidationError: No updates are to be performed)
			if !strings.Contains(err.Error(), "No updates are to be performed") {
				return fmt.Errorf("update cloudformation stack: %w", err)
			}
		}

		obj.Status.ObservedGeneration = obj.Generation
		now := metav1.Now()
		obj.Status.LastSyncTime = &now
		return r.setConditionCFN(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "CloudFormationStack reconciled")
	}

	createInput := &awscfn.CreateStackInput{
		StackName:    aws.String(obj.Spec.StackName),
		Parameters:   params,
		Capabilities: caps,
		Tags:         tags,
	}
	if obj.Spec.TemplateBody != "" {
		createInput.TemplateBody = aws.String(obj.Spec.TemplateBody)
	} else if obj.Spec.TemplateURL != "" {
		createInput.TemplateURL = aws.String(obj.Spec.TemplateURL)
	}
	out, err := r.CloudFormationClient.CreateStack(ctx, createInput)
	if err != nil {
		return fmt.Errorf("create cloudformation stack: %w", err)
	}

	obj.Status.StackID = aws.ToString(out.StackId)
	obj.Status.StackStatus = "CREATE_IN_PROGRESS"
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionCFN(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonCreated, "CloudFormationStack creating")
}

func (r *CloudFormationStackReconciler) deleteStack(ctx context.Context, obj *awsv1alpha1.CloudFormationStack) error {
	_, err := r.CloudFormationClient.DeleteStack(ctx, &awscfn.DeleteStackInput{
		StackName: aws.String(obj.Spec.StackName),
	})
	if cfnhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func buildCFNParameters(params []awsv1alpha1.CloudFormationParameter) []cfntypes.Parameter {
	out := make([]cfntypes.Parameter, 0, len(params))
	for _, p := range params {
		p := p
		out = append(out, cfntypes.Parameter{
			ParameterKey:   &p.ParameterKey,
			ParameterValue: &p.ParameterValue,
		})
	}
	return out
}

func buildCFNCapabilities(caps []string) []cfntypes.Capability {
	out := make([]cfntypes.Capability, 0, len(caps))
	for _, c := range caps {
		out = append(out, cfntypes.Capability(c))
	}
	return out
}

func buildCFNTags(tags map[string]string) []cfntypes.Tag {
	out := make([]cfntypes.Tag, 0, len(tags))
	for k, v := range tags {
		k, v := k, v
		out = append(out, cfntypes.Tag{Key: &k, Value: &v})
	}
	return out
}

func (r *CloudFormationStackReconciler) setConditionCFN(ctx context.Context, obj *awsv1alpha1.CloudFormationStack, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *CloudFormationStackReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CloudFormationStack{}).
		Complete(r)
}
