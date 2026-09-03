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
	awselbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	elbv2helper "github.com/konfig-io/konfig-konector/internal/aws/elbv2"
)

// TargetGroupReconciler reconciles TargetGroup objects.
type TargetGroupReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	ELBv2Client *awselbv2.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=targetgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=targetgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=targetgroups/finalizers,verbs=update

func (r *TargetGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	tg := &awsv1alpha1.TargetGroup{}
	if err := r.Get(ctx, req.NamespacedName, tg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !tg.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(tg, awsv1alpha1.FinalizerName) {
			if shouldAbandon(tg) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(tg, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, tg)
			}
			if err := r.deleteTargetGroup(ctx, tg); err != nil {
				logger.Error(err, "failed to delete TargetGroup")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(tg, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, tg)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(tg, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(tg, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, tg); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileTargetGroup(ctx, tg); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionTG(ctx, tg, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *TargetGroupReconciler) reconcileTargetGroup(ctx context.Context, tg *awsv1alpha1.TargetGroup) error {
	if tg.Status.ARN != "" {
		out, err := r.ELBv2Client.DescribeTargetGroups(ctx, &awselbv2.DescribeTargetGroupsInput{
			TargetGroupArns: []string{tg.Status.ARN},
		})
		if err != nil && !elbv2helper.IsNotFound(err) {
			return fmt.Errorf("describe target group: %w", err)
		}
		if err == nil && len(out.TargetGroups) > 0 {
			tg.Status.ObservedGeneration = tg.Generation
			now := metav1.Now()
			tg.Status.LastSyncTime = &now
			return r.setConditionTG(ctx, tg, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "TargetGroup reconciled")
		}
		tg.Status.ARN = ""
	}

	input := &awselbv2.CreateTargetGroupInput{
		Name:     aws.String(tg.Spec.Name),
		Protocol: elbv2types.ProtocolEnum(tg.Spec.Protocol),
		Port:     aws.Int32(tg.Spec.Port),
	}
	if tg.Spec.TargetType != "" {
		input.TargetType = elbv2types.TargetTypeEnum(tg.Spec.TargetType)
	}
	if tg.Spec.VPCRef != nil {
		vpcCR := &awsv1alpha1.VPC{}
		if tg.Spec.VPCRef.ID != "" {
			input.VpcId = aws.String(tg.Spec.VPCRef.ID)
		} else if tg.Spec.VPCRef.Name != "" {
			if err := r.Get(ctx, k8stypes.NamespacedName{Name: tg.Spec.VPCRef.Name, Namespace: tg.Namespace}, vpcCR); err != nil {
				return err
			}
			if vpcCR.Status.VPCID == "" {
				return &dependencyNotReady{msg: fmt.Sprintf("VPC %s/%s has no vpcId yet", tg.Namespace, tg.Spec.VPCRef.Name)}
			}
			input.VpcId = aws.String(vpcCR.Status.VPCID)
		}
	}
	if hc := tg.Spec.HealthCheck; hc != nil {
		if hc.Protocol != "" {
			input.HealthCheckProtocol = elbv2types.ProtocolEnum(hc.Protocol)
		}
		if hc.Port != "" {
			input.HealthCheckPort = aws.String(hc.Port)
		}
		if hc.Path != "" {
			input.HealthCheckPath = aws.String(hc.Path)
		}
		if hc.HealthyThreshold > 0 {
			input.HealthyThresholdCount = aws.Int32(hc.HealthyThreshold)
		}
		if hc.UnhealthyThreshold > 0 {
			input.UnhealthyThresholdCount = aws.Int32(hc.UnhealthyThreshold)
		}
		if hc.IntervalSeconds > 0 {
			input.HealthCheckIntervalSeconds = aws.Int32(hc.IntervalSeconds)
		}
	}
	if len(tg.Spec.Tags) > 0 {
		tags := make([]elbv2types.Tag, 0, len(tg.Spec.Tags))
		for k, v := range tg.Spec.Tags {
			k, v := k, v
			tags = append(tags, elbv2types.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.ELBv2Client.CreateTargetGroup(ctx, input)
	if err != nil {
		return fmt.Errorf("create target group: %w", err)
	}
	if len(out.TargetGroups) == 0 {
		return fmt.Errorf("create target group: empty response")
	}

	tg.Status.ARN = aws.ToString(out.TargetGroups[0].TargetGroupArn)
	// The AWS resource now exists; losing the ARN would orphan it.
	if err := persistStatus(ctx, r.Client, tg); err != nil {
		return fmt.Errorf("persist status after create: %w", err)
	}
	tg.Status.ObservedGeneration = tg.Generation
	now := metav1.Now()
	tg.Status.LastSyncTime = &now
	return r.setConditionTG(ctx, tg, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "TargetGroup created")
}

func (r *TargetGroupReconciler) deleteTargetGroup(ctx context.Context, tg *awsv1alpha1.TargetGroup) error {
	if tg.Status.ARN == "" {
		// Status may have been lost after a successful create; the create
		// path always uses spec.name, so look it up by name before giving up.
		out, err := r.ELBv2Client.DescribeTargetGroups(ctx, &awselbv2.DescribeTargetGroupsInput{
			Names: []string{tg.Spec.Name},
		})
		if elbv2helper.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("describe target group by name: %w", err)
		}
		if len(out.TargetGroups) == 0 {
			return nil
		}
		tg.Status.ARN = aws.ToString(out.TargetGroups[0].TargetGroupArn)
	}
	_, err := r.ELBv2Client.DeleteTargetGroup(ctx, &awselbv2.DeleteTargetGroupInput{
		TargetGroupArn: aws.String(tg.Status.ARN),
	})
	if elbv2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *TargetGroupReconciler) setConditionTG(ctx context.Context, tg *awsv1alpha1.TargetGroup, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&tg.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: tg.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, tg); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *TargetGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.TargetGroup{}).
		Complete(r)
}
