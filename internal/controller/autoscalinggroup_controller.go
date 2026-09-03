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
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsautoscaling "github.com/aws/aws-sdk-go-v2/service/autoscaling"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	asghelper "github.com/konfig-io/konfig-konector/internal/aws/autoscaling"
)

// AutoScalingGroupReconciler reconciles AutoScalingGroup objects.
type AutoScalingGroupReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	ASGClient *awsautoscaling.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=autoscalinggroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=autoscalinggroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=autoscalinggroups/finalizers,verbs=update

func (r *AutoScalingGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	asg := &awsv1alpha1.AutoScalingGroup{}
	if err := r.Get(ctx, req.NamespacedName, asg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !asg.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(asg, awsv1alpha1.FinalizerName) {
			if shouldAbandon(asg) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(asg, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, asg)
			}
			if err := r.deleteASG(ctx, asg); err != nil {
				logger.Error(err, "failed to delete auto scaling group")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(asg, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, asg)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(asg, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(asg, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, asg); err != nil {
			return ctrl.Result{}, err
		}
	}

	result, err := r.reconcileASG(ctx, asg)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, asg, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *AutoScalingGroupReconciler) reconcileASG(ctx context.Context, asg *awsv1alpha1.AutoScalingGroup) (ctrl.Result, error) {
	// Resolve launch template ref.
	ltID, ltVersion, err := r.resolveLaunchTemplateRef(ctx, asg.Namespace, asg.Spec.LaunchTemplateRef)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Resolve subnet IDs.
	subnetIDs, err := r.resolveSubnetIDsForASG(ctx, asg.Namespace, asg.Spec.VPCZoneIdentifier)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Check if ASG exists.
	out, err := r.ASGClient.DescribeAutoScalingGroups(ctx, &awsautoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: []string{asg.Spec.AutoScalingGroupName},
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	ltSpec := &asgtypes.LaunchTemplateSpecification{
		LaunchTemplateId: aws.String(ltID),
		Version:          aws.String(ltVersion),
	}

	if len(out.AutoScalingGroups) > 0 {
		// Update existing ASG.
		existing := out.AutoScalingGroups[0]
		needsRefresh := false

		// Check if launch template version changed.
		if existing.LaunchTemplate != nil {
			if aws.ToString(existing.LaunchTemplate.Version) != ltVersion {
				needsRefresh = true
			}
		}

		updateInput := &awsautoscaling.UpdateAutoScalingGroupInput{
			AutoScalingGroupName: aws.String(asg.Spec.AutoScalingGroupName),
			MinSize:              aws.Int32(asg.Spec.MinSize),
			MaxSize:              aws.Int32(asg.Spec.MaxSize),
			LaunchTemplate:       ltSpec,
		}
		if asg.Spec.DesiredCapacity != nil {
			updateInput.DesiredCapacity = asg.Spec.DesiredCapacity
		}
		if len(subnetIDs) > 0 {
			updateInput.VPCZoneIdentifier = aws.String(strings.Join(subnetIDs, ","))
		}
		if _, err := r.ASGClient.UpdateAutoScalingGroup(ctx, updateInput); err != nil {
			return ctrl.Result{}, fmt.Errorf("update auto scaling group: %w", err)
		}

		// If launch template version changed, trigger instance refresh.
		if needsRefresh && asg.Status.ObservedGeneration != asg.Generation {
			if _, err := r.ASGClient.StartInstanceRefresh(ctx, &awsautoscaling.StartInstanceRefreshInput{
				AutoScalingGroupName: aws.String(asg.Spec.AutoScalingGroupName),
				Strategy:             asgtypes.RefreshStrategyRolling,
			}); err != nil {
				// Non-fatal: refresh already in progress returns an error we can ignore.
				if !strings.Contains(err.Error(), "InstanceRefreshInProgress") {
					return ctrl.Result{}, fmt.Errorf("start instance refresh: %w", err)
				}
			}
		}
	} else {
		// Create new ASG.
		tags := r.buildASGTags(asg.Spec.AutoScalingGroupName, asg.Spec.Tags)
		createInput := &awsautoscaling.CreateAutoScalingGroupInput{
			AutoScalingGroupName: aws.String(asg.Spec.AutoScalingGroupName),
			MinSize:              aws.Int32(asg.Spec.MinSize),
			MaxSize:              aws.Int32(asg.Spec.MaxSize),
			LaunchTemplate:       ltSpec,
			Tags:                 tags,
		}
		if asg.Spec.DesiredCapacity != nil {
			createInput.DesiredCapacity = asg.Spec.DesiredCapacity
		}
		if len(subnetIDs) > 0 {
			createInput.VPCZoneIdentifier = aws.String(strings.Join(subnetIDs, ","))
		}
		if len(asg.Spec.TargetGroupARNs) > 0 {
			createInput.TargetGroupARNs = asg.Spec.TargetGroupARNs
		}
		if _, err := r.ASGClient.CreateAutoScalingGroup(ctx, createInput); err != nil {
			return ctrl.Result{}, fmt.Errorf("create auto scaling group: %w", err)
		}
	}

	asg.Status.ObservedGeneration = asg.Generation
	now := metav1.Now()
	asg.Status.LastSyncTime = &now
	return requeueResult(), r.setCondition(ctx, asg, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Auto scaling group reconciled")
}

func (r *AutoScalingGroupReconciler) resolveLaunchTemplateRef(ctx context.Context, namespace string, ref awsv1alpha1.LaunchTemplateRef) (id, version string, err error) {
	version = ref.Version
	if version == "" {
		version = "$Latest"
	}

	if ref.ID != "" {
		return ref.ID, version, nil
	}

	if ref.Name != "" {
		ltCR := &awsv1alpha1.LaunchTemplate{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, ltCR); err != nil {
			return "", "", err
		}
		if ltCR.Status.LaunchTemplateID == "" {
			return "", "", &dependencyNotReady{msg: fmt.Sprintf("LaunchTemplate %s/%s has no ID yet", namespace, ref.Name)}
		}
		// If version not specified, use the latest version number from status.
		if ref.Version == "" && ltCR.Status.LatestVersionNumber > 0 {
			version = fmt.Sprintf("%d", ltCR.Status.LatestVersionNumber)
		}
		return ltCR.Status.LaunchTemplateID, version, nil
	}

	return "", "", fmt.Errorf("launchTemplateRef must specify either name or id")
}

func (r *AutoScalingGroupReconciler) resolveSubnetIDsForASG(ctx context.Context, namespace string, refs []awsv1alpha1.SubnetRef) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.ID != "" {
			ids = append(ids, ref.ID)
			continue
		}
		sn := &awsv1alpha1.Subnet{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, sn); err != nil {
			return nil, err
		}
		if sn.Status.SubnetID == "" {
			return nil, &dependencyNotReady{msg: fmt.Sprintf("Subnet %s/%s has no ID yet", namespace, ref.Name)}
		}
		ids = append(ids, sn.Status.SubnetID)
	}
	return ids, nil
}

func (r *AutoScalingGroupReconciler) buildASGTags(asgName string, tags map[string]string) []asgtypes.Tag {
	result := make([]asgtypes.Tag, 0, len(tags))
	for k, v := range tags {
		k, v := k, v
		result = append(result, asgtypes.Tag{
			Key:               &k,
			Value:             &v,
			ResourceId:        aws.String(asgName),
			ResourceType:      aws.String("auto-scaling-group"),
			PropagateAtLaunch: aws.Bool(true),
		})
	}
	return result
}

func (r *AutoScalingGroupReconciler) deleteASG(ctx context.Context, asg *awsv1alpha1.AutoScalingGroup) error {
	_, err := r.ASGClient.DeleteAutoScalingGroup(ctx, &awsautoscaling.DeleteAutoScalingGroupInput{
		AutoScalingGroupName: aws.String(asg.Spec.AutoScalingGroupName),
		ForceDelete:          aws.Bool(true),
	})
	if asghelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *AutoScalingGroupReconciler) setCondition(ctx context.Context, asg *awsv1alpha1.AutoScalingGroup, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&asg.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: asg.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, asg); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *AutoScalingGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.AutoScalingGroup{}).
		Complete(r)
}
