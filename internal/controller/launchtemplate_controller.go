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
)

// LaunchTemplateReconciler reconciles LaunchTemplate objects.
type LaunchTemplateReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client *awsec2.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=launchtemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=launchtemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=launchtemplates/finalizers,verbs=update

func (r *LaunchTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	lt := &awsv1alpha1.LaunchTemplate{}
	if err := r.Get(ctx, req.NamespacedName, lt); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !lt.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(lt, awsv1alpha1.FinalizerName) {
			if shouldAbandon(lt) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(lt, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, lt)
			}
			if err := r.deleteLaunchTemplate(ctx, lt); err != nil {
				logger.Error(err, "failed to delete launch template")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(lt, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, lt)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(lt, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(lt, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, lt); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileLaunchTemplate(ctx, lt); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, lt, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *LaunchTemplateReconciler) reconcileLaunchTemplate(ctx context.Context, lt *awsv1alpha1.LaunchTemplate) error {
	// Check if already exists.
	out, describeErr := r.EC2Client.DescribeLaunchTemplates(ctx, &awsec2.DescribeLaunchTemplatesInput{
		LaunchTemplateNames: []string{lt.Spec.LaunchTemplateName},
	})
	if describeErr != nil && !ec2helper.IsNotFound(describeErr) {
		return describeErr
	}
	exists := describeErr == nil && out != nil && len(out.LaunchTemplates) > 0

	ltData, err := r.buildLaunchTemplateData(ctx, lt)
	if err != nil {
		return err
	}

	if exists {
		existing := out.LaunchTemplates[0]
		lt.Status.LaunchTemplateID = aws.ToString(existing.LaunchTemplateId)

		// On spec change, create a new template version.
		if lt.Status.ObservedGeneration != lt.Generation {
			verOut, err := r.EC2Client.CreateLaunchTemplateVersion(ctx, &awsec2.CreateLaunchTemplateVersionInput{
				LaunchTemplateId:   existing.LaunchTemplateId,
				LaunchTemplateData: ltData,
			})
			if err != nil {
				return fmt.Errorf("create launch template version: %w", err)
			}
			lt.Status.LatestVersionNumber = aws.ToInt64(verOut.LaunchTemplateVersion.VersionNumber)
		}

		// Sync tags.
		if err := ec2helper.SyncResourceTags(ctx, r.EC2Client, lt.Status.LaunchTemplateID, "launch-template", lt.Spec.Tags); err != nil {
			return err
		}
	} else {
		// Create new launch template.
		tags := lt.Spec.Tags
		createOut, err := r.EC2Client.CreateLaunchTemplate(ctx, &awsec2.CreateLaunchTemplateInput{
			LaunchTemplateName: aws.String(lt.Spec.LaunchTemplateName),
			LaunchTemplateData: ltData,
			TagSpecifications: []ec2types.TagSpecification{
				{
					ResourceType: ec2types.ResourceTypeLaunchTemplate,
					Tags:         ec2helper.TagsFromMap(tags),
				},
			},
		})
		if err != nil {
			return fmt.Errorf("create launch template: %w", err)
		}
		lt.Status.LaunchTemplateID = aws.ToString(createOut.LaunchTemplate.LaunchTemplateId)
		lt.Status.LatestVersionNumber = aws.ToInt64(createOut.LaunchTemplate.LatestVersionNumber)
		if err := persistStatus(ctx, r.Client, lt); err != nil {
			return fmt.Errorf("persist launch template ID after create: %w", err)
		}
	}

	lt.Status.ObservedGeneration = lt.Generation
	now := metav1.Now()
	lt.Status.LastSyncTime = &now
	return r.setCondition(ctx, lt, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Launch template reconciled")
}

func (r *LaunchTemplateReconciler) buildLaunchTemplateData(ctx context.Context, lt *awsv1alpha1.LaunchTemplate) (*ec2types.RequestLaunchTemplateData, error) {
	data := &ec2types.RequestLaunchTemplateData{
		ImageId: aws.String(lt.Spec.ImageID),
	}
	if lt.Spec.InstanceType != "" {
		data.InstanceType = ec2types.InstanceType(lt.Spec.InstanceType)
	}
	if lt.Spec.KeyName != "" {
		data.KeyName = aws.String(lt.Spec.KeyName)
	}
	if lt.Spec.UserData != "" {
		data.UserData = aws.String(lt.Spec.UserData)
	}
	if lt.Spec.IAMInstanceProfile != "" {
		data.IamInstanceProfile = &ec2types.LaunchTemplateIamInstanceProfileSpecificationRequest{
			Name: aws.String(lt.Spec.IAMInstanceProfile),
		}
	}

	// Resolve security group IDs.
	if len(lt.Spec.SecurityGroupRefs) > 0 {
		sgIDs, err := r.resolveSGIDsForLT(ctx, lt.Namespace, lt.Spec.SecurityGroupRefs)
		if err != nil {
			return nil, err
		}
		data.SecurityGroupIds = sgIDs
	}

	// Block device mappings.
	for _, bdm := range lt.Spec.BlockDeviceMappings {
		bdm := bdm
		ebs := &ec2types.LaunchTemplateEbsBlockDeviceRequest{}
		if bdm.VolumeSize > 0 {
			ebs.VolumeSize = aws.Int32(bdm.VolumeSize)
		}
		if bdm.VolumeType != "" {
			ebs.VolumeType = ec2types.VolumeType(bdm.VolumeType)
		}
		if bdm.Encrypted {
			ebs.Encrypted = aws.Bool(true)
		}
		data.BlockDeviceMappings = append(data.BlockDeviceMappings, ec2types.LaunchTemplateBlockDeviceMappingRequest{
			DeviceName: aws.String(bdm.DeviceName),
			Ebs:        ebs,
		})
	}

	return data, nil
}

func (r *LaunchTemplateReconciler) resolveSGIDsForLT(ctx context.Context, namespace string, refs []awsv1alpha1.SecurityGroupRef) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.ID != "" {
			ids = append(ids, ref.ID)
			continue
		}
		sgCR := &awsv1alpha1.SecurityGroup{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, sgCR); err != nil {
			return nil, err
		}
		if sgCR.Status.GroupID == "" {
			return nil, &dependencyNotReady{msg: fmt.Sprintf("SecurityGroup %s/%s has no ID yet", namespace, ref.Name)}
		}
		ids = append(ids, sgCR.Status.GroupID)
	}
	return ids, nil
}

func (r *LaunchTemplateReconciler) deleteLaunchTemplate(ctx context.Context, lt *awsv1alpha1.LaunchTemplate) error {
	input := &awsec2.DeleteLaunchTemplateInput{}
	if lt.Status.LaunchTemplateID != "" {
		input.LaunchTemplateId = aws.String(lt.Status.LaunchTemplateID)
	} else {
		// Fallback: the status write may have been lost after create.
		// Launch template names are unique per account/region, so delete by
		// the deterministic spec name.
		input.LaunchTemplateName = aws.String(lt.Spec.LaunchTemplateName)
	}
	_, err := r.EC2Client.DeleteLaunchTemplate(ctx, input)
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *LaunchTemplateReconciler) setCondition(ctx context.Context, lt *awsv1alpha1.LaunchTemplate, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&lt.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: lt.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, lt); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *LaunchTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.LaunchTemplate{}).
		Complete(r)
}
