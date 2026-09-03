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

// EBSVolumeReconciler reconciles EBSVolume objects.
type EBSVolumeReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client *awsec2.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=ebsvolumes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ebsvolumes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ebsvolumes/finalizers,verbs=update

func (r *EBSVolumeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	vol := &awsv1alpha1.EBSVolume{}
	if err := r.Get(ctx, req.NamespacedName, vol); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !vol.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(vol, awsv1alpha1.FinalizerName) {
			if shouldAbandon(vol) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(vol, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, vol)
			}
			if err := r.deleteEBSVolume(ctx, vol); err != nil {
				logger.Error(err, "failed to delete EBSVolume")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(vol, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, vol)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(vol, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(vol, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, vol); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileEBSVolume(ctx, vol); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionEBS(ctx, vol, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *EBSVolumeReconciler) reconcileEBSVolume(ctx context.Context, vol *awsv1alpha1.EBSVolume) error {
	if vol.Status.VolumeID != "" {
		out, err := r.EC2Client.DescribeVolumes(ctx, &awsec2.DescribeVolumesInput{
			VolumeIds: []string{vol.Status.VolumeID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return fmt.Errorf("describe volume: %w", err)
		}
		if err == nil && len(out.Volumes) > 0 {
			vol.Status.State = string(out.Volumes[0].State)
			// Sync tags.
			if len(vol.Spec.Tags) > 0 {
				_, _ = r.EC2Client.CreateTags(ctx, &awsec2.CreateTagsInput{
					Resources: []string{vol.Status.VolumeID},
					Tags:      ec2helper.TagsFromMap(vol.Spec.Tags),
				})
			}
			// ModifyVolume for mutable fields (size, iops, throughput, type).
			modInput := &awsec2.ModifyVolumeInput{
				VolumeId: aws.String(vol.Status.VolumeID),
			}
			modified := false
			if vol.Spec.Size > 0 && vol.Spec.Size > aws.ToInt32(out.Volumes[0].Size) {
				modInput.Size = aws.Int32(vol.Spec.Size)
				modified = true
			}
			if vol.Spec.VolumeType != "" && string(out.Volumes[0].VolumeType) != vol.Spec.VolumeType {
				modInput.VolumeType = ec2types.VolumeType(vol.Spec.VolumeType)
				modified = true
			}
			if vol.Spec.IOPS > 0 {
				modInput.Iops = aws.Int32(vol.Spec.IOPS)
				modified = true
			}
			if vol.Spec.Throughput > 0 {
				modInput.Throughput = aws.Int32(vol.Spec.Throughput)
				modified = true
			}
			if modified {
				if _, err := r.EC2Client.ModifyVolume(ctx, modInput); err != nil {
					return fmt.Errorf("modify volume: %w", err)
				}
			}
			vol.Status.ObservedGeneration = vol.Generation
			now := metav1.Now()
			vol.Status.LastSyncTime = &now
			return r.setConditionEBS(ctx, vol, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "EBSVolume reconciled")
		}
		vol.Status.VolumeID = ""
	}

	input := &awsec2.CreateVolumeInput{
		AvailabilityZone: aws.String(vol.Spec.AvailabilityZone),
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeVolume,
				Tags:         ec2helper.TagsFromMap(vol.Spec.Tags),
			},
		},
	}
	if vol.Spec.VolumeType != "" {
		input.VolumeType = ec2types.VolumeType(vol.Spec.VolumeType)
	}
	if vol.Spec.Size > 0 {
		input.Size = aws.Int32(vol.Spec.Size)
	}
	if vol.Spec.IOPS > 0 {
		input.Iops = aws.Int32(vol.Spec.IOPS)
	}
	if vol.Spec.Throughput > 0 {
		input.Throughput = aws.Int32(vol.Spec.Throughput)
	}
	if vol.Spec.Encrypted {
		input.Encrypted = aws.Bool(true)
	}
	if vol.Spec.KMSKeyID != "" {
		input.KmsKeyId = aws.String(vol.Spec.KMSKeyID)
	}
	if vol.Spec.SnapshotID != "" {
		input.SnapshotId = aws.String(vol.Spec.SnapshotID)
	}
	if vol.Spec.MultiAttachEnabled {
		input.MultiAttachEnabled = aws.Bool(true)
	}

	out, err := r.EC2Client.CreateVolume(ctx, input)
	if err != nil {
		return fmt.Errorf("create volume: %w", err)
	}

	vol.Status.VolumeID = aws.ToString(out.VolumeId)
	vol.Status.State = string(out.State)
	if err := persistStatus(ctx, r.Client, vol); err != nil {
		return fmt.Errorf("persist EBSVolume ID after create: %w", err)
	}
	vol.Status.ObservedGeneration = vol.Generation
	now := metav1.Now()
	vol.Status.LastSyncTime = &now
	return r.setConditionEBS(ctx, vol, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "EBSVolume created")
}

func (r *EBSVolumeReconciler) deleteEBSVolume(ctx context.Context, vol *awsv1alpha1.EBSVolume) error {
	volumeID := vol.Status.VolumeID
	if volumeID == "" {
		// Fallback: the status write may have been lost after CreateVolume.
		// Look up by the tags the create path applied, scoped to the AZ;
		// without tags the volume is indistinguishable, so give up.
		if len(vol.Spec.Tags) == 0 {
			return nil
		}
		filters := []ec2types.Filter{
			{Name: aws.String("availability-zone"), Values: []string{vol.Spec.AvailabilityZone}},
		}
		for k, v := range vol.Spec.Tags {
			filters = append(filters, ec2types.Filter{Name: aws.String("tag:" + k), Values: []string{v}})
		}
		out, err := r.EC2Client.DescribeVolumes(ctx, &awsec2.DescribeVolumesInput{Filters: filters})
		if err != nil {
			return fmt.Errorf("lookup volume by tags: %w", err)
		}
		if len(out.Volumes) != 1 {
			return nil
		}
		volumeID = aws.ToString(out.Volumes[0].VolumeId)
	}
	_, err := r.EC2Client.DeleteVolume(ctx, &awsec2.DeleteVolumeInput{
		VolumeId: aws.String(volumeID),
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *EBSVolumeReconciler) setConditionEBS(ctx context.Context, vol *awsv1alpha1.EBSVolume, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&vol.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: vol.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, vol); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *EBSVolumeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.EBSVolume{}).
		Complete(r)
}
