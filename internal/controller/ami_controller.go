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

// AMIReconciler reconciles AMI objects.
type AMIReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client *awsec2.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=amis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=amis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=amis/finalizers,verbs=update

func (r *AMIReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ami := &awsv1alpha1.AMI{}
	if err := r.Get(ctx, req.NamespacedName, ami); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !ami.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ami, awsv1alpha1.FinalizerName) {
			if shouldAbandon(ami) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(ami, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, ami)
			}
			if err := r.deleteAMI(ctx, ami); err != nil {
				logger.Error(err, "failed to deregister AMI")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(ami, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, ami)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(ami, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(ami, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, ami); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileAMI(ctx, ami); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionAMI(ctx, ami, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *AMIReconciler) reconcileAMI(ctx context.Context, ami *awsv1alpha1.AMI) error {
	if ami.Status.ImageID != "" {
		out, err := r.EC2Client.DescribeImages(ctx, &awsec2.DescribeImagesInput{
			ImageIds: []string{ami.Status.ImageID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return fmt.Errorf("describe AMI: %w", err)
		}
		if err == nil && len(out.Images) > 0 {
			ami.Status.State = string(out.Images[0].State)
			if len(ami.Spec.Tags) > 0 {
				_, _ = r.EC2Client.CreateTags(ctx, &awsec2.CreateTagsInput{
					Resources: []string{ami.Status.ImageID},
					Tags:      ec2helper.TagsFromMap(ami.Spec.Tags),
				})
			}
			ami.Status.ObservedGeneration = ami.Generation
			now := metav1.Now()
			ami.Status.LastSyncTime = &now
			return r.setConditionAMI(ctx, ami, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "AMI reconciled")
		}
		ami.Status.ImageID = ""
	}

	input := &awsec2.RegisterImageInput{
		Name: aws.String(ami.Spec.Name),
	}
	if ami.Spec.Description != "" {
		input.Description = aws.String(ami.Spec.Description)
	}
	if ami.Spec.Architecture != "" {
		input.Architecture = ec2types.ArchitectureValues(ami.Spec.Architecture)
	}
	if ami.Spec.RootDeviceName != "" {
		input.RootDeviceName = aws.String(ami.Spec.RootDeviceName)
	}
	if ami.Spec.VirtualizationType != "" {
		input.VirtualizationType = aws.String(ami.Spec.VirtualizationType)
	}
	if ami.Spec.KernelID != "" {
		input.KernelId = aws.String(ami.Spec.KernelID)
	}
	if ami.Spec.RamdiskID != "" {
		input.RamdiskId = aws.String(ami.Spec.RamdiskID)
	}
	if ami.Spec.SRIOVNetSupport != "" {
		input.SriovNetSupport = aws.String(ami.Spec.SRIOVNetSupport)
	}
	if ami.Spec.ENASupport {
		input.EnaSupport = aws.Bool(true)
	}
	if len(ami.Spec.BlockDeviceMappings) > 0 {
		mappings := make([]ec2types.BlockDeviceMapping, 0, len(ami.Spec.BlockDeviceMappings))
		for _, bdm := range ami.Spec.BlockDeviceMappings {
			m := ec2types.BlockDeviceMapping{
				DeviceName: aws.String(bdm.DeviceName),
				Ebs:        &ec2types.EbsBlockDevice{},
			}
			if bdm.SnapshotID != "" {
				m.Ebs.SnapshotId = aws.String(bdm.SnapshotID)
			}
			if bdm.VolumeSize > 0 {
				m.Ebs.VolumeSize = aws.Int32(bdm.VolumeSize)
			}
			if bdm.VolumeType != "" {
				m.Ebs.VolumeType = ec2types.VolumeType(bdm.VolumeType)
			}
			if bdm.DeleteOnTermination {
				m.Ebs.DeleteOnTermination = aws.Bool(true)
			}
			if bdm.Encrypted {
				m.Ebs.Encrypted = aws.Bool(true)
			}
			mappings = append(mappings, m)
		}
		input.BlockDeviceMappings = mappings
	}
	if len(ami.Spec.Tags) > 0 {
		input.TagSpecifications = []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeImage,
				Tags:         ec2helper.TagsFromMap(ami.Spec.Tags),
			},
		}
	}

	out, err := r.EC2Client.RegisterImage(ctx, input)
	if err != nil {
		return fmt.Errorf("register AMI: %w", err)
	}

	ami.Status.ImageID = aws.ToString(out.ImageId)
	ami.Status.State = "pending"
	if err := persistStatus(ctx, r.Client, ami); err != nil {
		return fmt.Errorf("persist AMI ID after create: %w", err)
	}
	ami.Status.ObservedGeneration = ami.Generation
	now := metav1.Now()
	ami.Status.LastSyncTime = &now
	return r.setConditionAMI(ctx, ami, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "AMI registered")
}

func (r *AMIReconciler) deleteAMI(ctx context.Context, ami *awsv1alpha1.AMI) error {
	imageID := ami.Status.ImageID
	if imageID == "" {
		// Fallback: the status write may have been lost after RegisterImage.
		// AMI names are unique per account, so look up our own image by name.
		out, err := r.EC2Client.DescribeImages(ctx, &awsec2.DescribeImagesInput{
			Owners: []string{"self"},
			Filters: []ec2types.Filter{
				{Name: aws.String("name"), Values: []string{ami.Spec.Name}},
			},
		})
		if err != nil {
			return fmt.Errorf("lookup AMI by name: %w", err)
		}
		if len(out.Images) != 1 {
			return nil
		}
		imageID = aws.ToString(out.Images[0].ImageId)
	}
	_, err := r.EC2Client.DeregisterImage(ctx, &awsec2.DeregisterImageInput{
		ImageId: aws.String(imageID),
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *AMIReconciler) setConditionAMI(ctx context.Context, ami *awsv1alpha1.AMI, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&ami.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: ami.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, ami); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *AMIReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.AMI{}).
		Complete(r)
}
