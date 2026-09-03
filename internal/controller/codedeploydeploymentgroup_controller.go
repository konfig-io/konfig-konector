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
	awscd "github.com/aws/aws-sdk-go-v2/service/codedeploy"
	cdtypes "github.com/aws/aws-sdk-go-v2/service/codedeploy/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	cdhelper "github.com/konfig-io/konfig-konector/internal/aws/codedeploy"
)

// CodeDeployDeploymentGroupReconciler reconciles CodeDeployDeploymentGroup objects.
type CodeDeployDeploymentGroupReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	CodeDeployClient *awscd.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=codedeploydeploymentgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=codedeploydeploymentgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=codedeploydeploymentgroups/finalizers,verbs=update

func (r *CodeDeployDeploymentGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.CodeDeployDeploymentGroup{}
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
			if err := r.deleteDeploymentGroup(ctx, obj); err != nil {
				logger.Error(err, "failed to delete CodeDeployDeploymentGroup")
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

	if err := r.reconcileDeploymentGroup(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionCDDG(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *CodeDeployDeploymentGroupReconciler) reconcileDeploymentGroup(ctx context.Context, obj *awsv1alpha1.CodeDeployDeploymentGroup) error {
	getOut, err := r.CodeDeployClient.GetDeploymentGroup(ctx, &awscd.GetDeploymentGroupInput{
		ApplicationName:     aws.String(obj.Spec.ApplicationName),
		DeploymentGroupName: aws.String(obj.Spec.DeploymentGroupName),
	})
	if err != nil && !cdhelper.IsNotFound(err) {
		return fmt.Errorf("get codedeploy deployment group: %w", err)
	}

	if err == nil && getOut.DeploymentGroupInfo != nil {
		obj.Status.DeploymentGroupID = aws.ToString(getOut.DeploymentGroupInfo.DeploymentGroupId)

		updateInput := &awscd.UpdateDeploymentGroupInput{
			ApplicationName:            aws.String(obj.Spec.ApplicationName),
			CurrentDeploymentGroupName: aws.String(obj.Spec.DeploymentGroupName),
			ServiceRoleArn:             aws.String(obj.Spec.ServiceRoleARN),
			AutoScalingGroups:          obj.Spec.AutoScalingGroups,
		}
		if obj.Spec.DeploymentConfigName != "" {
			updateInput.DeploymentConfigName = aws.String(obj.Spec.DeploymentConfigName)
		}
		if _, err := r.CodeDeployClient.UpdateDeploymentGroup(ctx, updateInput); err != nil {
			return fmt.Errorf("update codedeploy deployment group: %w", err)
		}

		obj.Status.ObservedGeneration = obj.Generation
		now := metav1.Now()
		obj.Status.LastSyncTime = &now
		return r.setConditionCDDG(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "CodeDeployDeploymentGroup reconciled")
	}

	tags := make([]cdtypes.Tag, 0, len(obj.Spec.Tags))
	for k, v := range obj.Spec.Tags {
		k, v := k, v
		tags = append(tags, cdtypes.Tag{Key: &k, Value: &v})
	}

	createInput := &awscd.CreateDeploymentGroupInput{
		ApplicationName:     aws.String(obj.Spec.ApplicationName),
		DeploymentGroupName: aws.String(obj.Spec.DeploymentGroupName),
		ServiceRoleArn:      aws.String(obj.Spec.ServiceRoleARN),
		AutoScalingGroups:   obj.Spec.AutoScalingGroups,
		Tags:                tags,
	}
	if obj.Spec.DeploymentConfigName != "" {
		createInput.DeploymentConfigName = aws.String(obj.Spec.DeploymentConfigName)
	}

	out, err := r.CodeDeployClient.CreateDeploymentGroup(ctx, createInput)
	if err != nil {
		return fmt.Errorf("create codedeploy deployment group: %w", err)
	}

	obj.Status.DeploymentGroupID = aws.ToString(out.DeploymentGroupId)
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionCDDG(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "CodeDeployDeploymentGroup created")
}

func (r *CodeDeployDeploymentGroupReconciler) deleteDeploymentGroup(ctx context.Context, obj *awsv1alpha1.CodeDeployDeploymentGroup) error {
	_, err := r.CodeDeployClient.DeleteDeploymentGroup(ctx, &awscd.DeleteDeploymentGroupInput{
		ApplicationName:     aws.String(obj.Spec.ApplicationName),
		DeploymentGroupName: aws.String(obj.Spec.DeploymentGroupName),
	})
	if cdhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *CodeDeployDeploymentGroupReconciler) setConditionCDDG(ctx context.Context, obj *awsv1alpha1.CodeDeployDeploymentGroup, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *CodeDeployDeploymentGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CodeDeployDeploymentGroup{}).
		Complete(r)
}
