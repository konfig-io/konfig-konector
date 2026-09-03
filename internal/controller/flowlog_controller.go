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
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ec2helper "github.com/konfig-io/konfig-konector/internal/aws/ec2"
)

// FlowLogReconciler reconciles FlowLog objects.
type FlowLogReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client *awsec2.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=flowlogs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=flowlogs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=flowlogs/finalizers,verbs=update

func (r *FlowLogReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	fl := &awsv1alpha1.FlowLog{}
	if err := r.Get(ctx, req.NamespacedName, fl); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !fl.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(fl, awsv1alpha1.FinalizerName) {
			if shouldAbandon(fl) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(fl, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, fl)
			}
			if err := r.deleteFlowLog(ctx, fl); err != nil {
				logger.Error(err, "failed to delete FlowLog")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(fl, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, fl)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(fl, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(fl, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, fl); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileFlowLog(ctx, fl); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionFL(ctx, fl, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *FlowLogReconciler) reconcileFlowLog(ctx context.Context, fl *awsv1alpha1.FlowLog) error {
	if fl.Status.FlowLogID != "" {
		out, err := r.EC2Client.DescribeFlowLogs(ctx, &awsec2.DescribeFlowLogsInput{
			FlowLogIds: []string{fl.Status.FlowLogID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return fmt.Errorf("describe flow log: %w", err)
		}
		if err == nil && len(out.FlowLogs) > 0 {
			fl.Status.ObservedGeneration = fl.Generation
			now := metav1.Now()
			fl.Status.LastSyncTime = &now
			return r.setConditionFL(ctx, fl, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "FlowLog reconciled")
		}
		fl.Status.FlowLogID = ""
	}

	input := &awsec2.CreateFlowLogsInput{
		ResourceIds:  []string{fl.Spec.ResourceID},
		ResourceType: ec2types.FlowLogsResourceType(fl.Spec.ResourceType),
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeVpcFlowLog,
				Tags:         ec2helper.TagsFromMap(fl.Spec.Tags),
			},
		},
	}
	if fl.Spec.TrafficType != "" {
		input.TrafficType = ec2types.TrafficType(fl.Spec.TrafficType)
	} else {
		input.TrafficType = ec2types.TrafficTypeAll
	}
	if fl.Spec.LogDestinationType != "" {
		input.LogDestinationType = ec2types.LogDestinationType(fl.Spec.LogDestinationType)
	}
	if fl.Spec.LogDestination != "" {
		input.LogDestination = aws.String(fl.Spec.LogDestination)
	}
	if fl.Spec.LogGroupName != "" {
		input.LogGroupName = aws.String(fl.Spec.LogGroupName)
	}
	if fl.Spec.DeliverLogsPermissionARN != "" {
		input.DeliverLogsPermissionArn = aws.String(fl.Spec.DeliverLogsPermissionARN)
	}
	if fl.Spec.LogFormat != "" {
		input.LogFormat = aws.String(fl.Spec.LogFormat)
	}
	if fl.Spec.MaxAggregationInterval > 0 {
		input.MaxAggregationInterval = aws.Int32(fl.Spec.MaxAggregationInterval)
	}

	out, err := r.EC2Client.CreateFlowLogs(ctx, input)
	if err != nil {
		return fmt.Errorf("create flow log: %w", err)
	}
	if len(out.Unsuccessful) > 0 {
		return fmt.Errorf("create flow log unsuccessful: %s", aws.ToString(out.Unsuccessful[0].Error.Message))
	}
	if len(out.FlowLogIds) == 0 {
		return fmt.Errorf("create flow log: no flow log ID returned")
	}

	fl.Status.FlowLogID = out.FlowLogIds[0]
	if err := persistStatus(ctx, r.Client, fl); err != nil {
		return fmt.Errorf("persist FlowLog ID after create: %w", err)
	}
	fl.Status.ObservedGeneration = fl.Generation
	now := metav1.Now()
	fl.Status.LastSyncTime = &now
	return r.setConditionFL(ctx, fl, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "FlowLog created")
}

func (r *FlowLogReconciler) deleteFlowLog(ctx context.Context, fl *awsv1alpha1.FlowLog) error {
	flowLogID := fl.Status.FlowLogID
	if flowLogID == "" {
		// Fallback: the status write may have been lost after create.
		// Look up by the monitored resource ID plus the tags the create
		// path applied.
		filters := []ec2types.Filter{
			{Name: aws.String("resource-id"), Values: []string{fl.Spec.ResourceID}},
		}
		for k, v := range fl.Spec.Tags {
			filters = append(filters, ec2types.Filter{Name: aws.String("tag:" + k), Values: []string{v}})
		}
		out, err := r.EC2Client.DescribeFlowLogs(ctx, &awsec2.DescribeFlowLogsInput{Filter: filters})
		if err != nil {
			return fmt.Errorf("lookup flow log by resource/tags: %w", err)
		}
		if len(out.FlowLogs) != 1 {
			return nil
		}
		flowLogID = aws.ToString(out.FlowLogs[0].FlowLogId)
	}
	_, err := r.EC2Client.DeleteFlowLogs(ctx, &awsec2.DeleteFlowLogsInput{
		FlowLogIds: []string{flowLogID},
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *FlowLogReconciler) setConditionFL(ctx context.Context, fl *awsv1alpha1.FlowLog, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&fl.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: fl.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, fl); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *FlowLogReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.FlowLog{}).
		Complete(r)
}
