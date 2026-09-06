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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ec2helper "github.com/konfig-io/konfig-konector/internal/aws/ec2"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

var requeueEC2Polling = ctrl.Result{RequeueAfter: 15 * time.Second}

// EC2InstanceReconciler reconciles EC2Instance objects.
type EC2InstanceReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client *multi.EC2
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=ec2instances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ec2instances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ec2instances/finalizers,verbs=update

func (r *EC2InstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	inst := &awsv1alpha1.EC2Instance{}
	if err := r.Get(ctx, req.NamespacedName, inst); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, inst); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !inst.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(inst, awsv1alpha1.FinalizerName) {
			if shouldAbandon(inst) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(inst, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, inst)
			}
			if err := r.terminateInstance(ctx, inst); err != nil {
				logger.Error(err, "failed to terminate EC2 instance")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(inst, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, inst)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(inst, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(inst, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, inst); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	result, err := r.reconcileInstance(ctx, inst)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, inst, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *EC2InstanceReconciler) reconcileInstance(ctx context.Context, inst *awsv1alpha1.EC2Instance) (ctrl.Result, error) {
	// If we have an instance ID, check its current state.
	if inst.Status.InstanceID != "" {
		out, err := r.EC2Client.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{
			InstanceIds: []string{inst.Status.InstanceID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("describe instance: %w", err)
		}
		if err == nil && len(out.Reservations) > 0 && len(out.Reservations[0].Instances) > 0 {
			ec2Inst := out.Reservations[0].Instances[0]
			inst.Status.State = string(ec2Inst.State.Name)
			inst.Status.PrivateIP = aws.ToString(ec2Inst.PrivateIpAddress)
			inst.Status.PublicIP = aws.ToString(ec2Inst.PublicIpAddress)

			switch ec2Inst.State.Name {
			case ec2types.InstanceStateNameTerminated, ec2types.InstanceStateNameShuttingDown:
				// Instance is gone — clear ID so it gets recreated.
				inst.Status.InstanceID = ""
			case ec2types.InstanceStateNamePending:
				_ = r.setCondition(ctx, inst, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "EC2 instance is pending")
				return requeueEC2Polling, nil
			case ec2types.InstanceStateNameRunning:
				// Sync tags — instance itself is immutable after creation.
				if err := ec2helper.SyncResourceTags(ctx, r.EC2Client, inst.Status.InstanceID, "instance", inst.Spec.Tags); err != nil {
					return ctrl.Result{}, err
				}
				now := metav1.Now()
				inst.Status.LastSyncTime = &now
				if inst.Status.ObservedGeneration != inst.Generation {
					// The spec changed after creation, but EC2 instances are
					// immutable by design. Report honestly instead of Synced;
					// ObservedGeneration is deliberately NOT bumped so this
					// condition stays sticky until the spec is reverted or the
					// instance is recreated. Not an error — requeue normally.
					return requeueResult(), r.setCondition(ctx, inst, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonUpdateNotSupported,
						"EC2 instances are immutable; spec changes after creation are not applied")
				}
				return requeueResult(), r.setCondition(ctx, inst, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "EC2 instance running")
			default:
				// stopping, stopped — report state but don't retry immediately.
				_ = r.setCondition(ctx, inst, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "NotRunning",
					fmt.Sprintf("EC2 instance is %s", inst.Status.State))
				return requeueResult(), nil
			}
		}
	}

	// Resolve subnet ID.
	subnetID, err := r.resolveSubnetID(ctx, inst)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Resolve security group IDs.
	sgIDs, err := r.resolveSGIDsForInstance(ctx, inst)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Build RunInstances input.
	input := &awsec2.RunInstancesInput{
		ImageId:      aws.String(inst.Spec.ImageID),
		InstanceType: ec2types.InstanceType(inst.Spec.InstanceType),
		MinCount:     aws.Int32(1),
		MaxCount:     aws.Int32(1),
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeInstance,
				Tags:         ec2helper.TagsFromMap(inst.Spec.Tags),
			},
		},
	}
	if inst.Spec.KeyName != "" {
		input.KeyName = aws.String(inst.Spec.KeyName)
	}
	if inst.Spec.UserData != "" {
		input.UserData = aws.String(inst.Spec.UserData)
	}
	if inst.Spec.IAMInstanceProfile != "" {
		input.IamInstanceProfile = &ec2types.IamInstanceProfileSpecification{
			Name: aws.String(inst.Spec.IAMInstanceProfile),
		}
	}
	if subnetID != "" || len(sgIDs) > 0 {
		ni := ec2types.InstanceNetworkInterfaceSpecification{
			DeviceIndex:              aws.Int32(0),
			AssociatePublicIpAddress: aws.Bool(inst.Spec.AssociatePublicIPAddress),
		}
		if subnetID != "" {
			ni.SubnetId = aws.String(subnetID)
		}
		if len(sgIDs) > 0 {
			ni.Groups = sgIDs
		}
		input.NetworkInterfaces = []ec2types.InstanceNetworkInterfaceSpecification{ni}
	}

	out, err := r.EC2Client.RunInstances(ctx, input)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("run instance: %w", err)
	}

	inst.Status.InstanceID = aws.ToString(out.Instances[0].InstanceId)
	inst.Status.State = string(out.Instances[0].State.Name)
	// Record the generation the instance was created from; later Generation
	// bumps are reported as UpdateNotSupported since instances are immutable.
	inst.Status.ObservedGeneration = inst.Generation
	// RunInstances is not idempotent and the existence check is gated on
	// Status.InstanceID, so persisting the ID here is the only protection
	// against duplicating the instance on retry.
	if err := persistStatus(ctx, r.Client, inst); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist EC2 instance ID after create: %w", err)
	}
	_ = r.setCondition(ctx, inst, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "EC2 instance is pending")
	return requeueEC2Polling, nil
}

func (r *EC2InstanceReconciler) terminateInstance(ctx context.Context, inst *awsv1alpha1.EC2Instance) error {
	if inst.Status.InstanceID == "" {
		// No fallback lookup: instances have no deterministic unique identifier
		// (tags may be empty or shared). The persist immediately after
		// RunInstances keeps this window effectively closed.
		return nil
	}
	_, err := r.EC2Client.TerminateInstances(ctx, &awsec2.TerminateInstancesInput{
		InstanceIds: []string{inst.Status.InstanceID},
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *EC2InstanceReconciler) resolveSubnetID(ctx context.Context, inst *awsv1alpha1.EC2Instance) (string, error) {
	if inst.Spec.SubnetRef == "" {
		return "", nil
	}
	subnetCR := &awsv1alpha1.Subnet{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: inst.Spec.SubnetRef, Namespace: inst.Namespace}, subnetCR); err != nil {
		return "", err
	}
	if subnetCR.Status.SubnetID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("Subnet %s/%s has no ID yet", inst.Namespace, inst.Spec.SubnetRef)}
	}
	return subnetCR.Status.SubnetID, nil
}

func (r *EC2InstanceReconciler) resolveSGIDsForInstance(ctx context.Context, inst *awsv1alpha1.EC2Instance) ([]string, error) {
	ids := make([]string, 0, len(inst.Spec.SecurityGroupRefs))
	for _, ref := range inst.Spec.SecurityGroupRefs {
		if ref.ID != "" {
			ids = append(ids, ref.ID)
			continue
		}
		sgCR := &awsv1alpha1.SecurityGroup{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: inst.Namespace}, sgCR); err != nil {
			return nil, err
		}
		if sgCR.Status.GroupID == "" {
			return nil, &dependencyNotReady{msg: fmt.Sprintf("SecurityGroup %s/%s has no ID yet", inst.Namespace, ref.Name)}
		}
		ids = append(ids, sgCR.Status.GroupID)
	}
	return ids, nil
}

func (r *EC2InstanceReconciler) setCondition(ctx context.Context, inst *awsv1alpha1.EC2Instance, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&inst.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: inst.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, inst); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *EC2InstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.EC2Instance{}).
		Complete(r)
}
