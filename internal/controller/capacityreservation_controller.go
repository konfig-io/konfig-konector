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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ec2helper "github.com/konfig-io/konfig-konector/internal/aws/ec2"
)

// CapacityReservationAWSAPI is the subset of the EC2 API used by this controller.
type CapacityReservationAWSAPI interface {
	DescribeCapacityReservations(ctx context.Context, params *awsec2.DescribeCapacityReservationsInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeCapacityReservationsOutput, error)
	CreateCapacityReservation(ctx context.Context, params *awsec2.CreateCapacityReservationInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateCapacityReservationOutput, error)
	ModifyCapacityReservation(ctx context.Context, params *awsec2.ModifyCapacityReservationInput, optFns ...func(*awsec2.Options)) (*awsec2.ModifyCapacityReservationOutput, error)
	CancelCapacityReservation(ctx context.Context, params *awsec2.CancelCapacityReservationInput, optFns ...func(*awsec2.Options)) (*awsec2.CancelCapacityReservationOutput, error)
	CreateTags(ctx context.Context, params *awsec2.CreateTagsInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateTagsOutput, error)
}

// CapacityReservationReconciler reconciles CapacityReservation objects.
type CapacityReservationReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client CapacityReservationAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=capacityreservations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=capacityreservations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=capacityreservations/finalizers,verbs=update

func (r *CapacityReservationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.CapacityReservation{}
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
			if err := r.cancelReservation(ctx, obj); err != nil {
				logger.Error(err, "failed to cancel CapacityReservation")
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

	if err := r.reconcileReservation(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *CapacityReservationReconciler) reconcileReservation(ctx context.Context, obj *awsv1alpha1.CapacityReservation) error {
	if obj.Status.CapacityReservationID == "" {
		input := &awsec2.CreateCapacityReservationInput{
			InstanceType:     aws.String(obj.Spec.InstanceType),
			InstancePlatform: ec2types.CapacityReservationInstancePlatform(obj.Spec.InstancePlatform),
			AvailabilityZone: aws.String(obj.Spec.AvailabilityZone),
			InstanceCount:    aws.Int32(obj.Spec.InstanceCount),
			TagSpecifications: []ec2types.TagSpecification{
				{ResourceType: ec2types.ResourceTypeCapacityReservation, Tags: ec2helper.TagsFromMap(obj.Spec.Tags)},
			},
		}
		if obj.Spec.Tenancy != "" {
			input.Tenancy = ec2types.CapacityReservationTenancy(obj.Spec.Tenancy)
		}
		if obj.Spec.EndDateType != "" {
			input.EndDateType = ec2types.EndDateType(obj.Spec.EndDateType)
		}
		if obj.Spec.EndDate != nil {
			input.EndDate = &obj.Spec.EndDate.Time
		}
		out, err := r.EC2Client.CreateCapacityReservation(ctx, input)
		if err != nil {
			return fmt.Errorf("create capacity reservation: %w", err)
		}
		obj.Status.CapacityReservationID = aws.ToString(out.CapacityReservation.CapacityReservationId)
		obj.Status.ARN = aws.ToString(out.CapacityReservation.CapacityReservationArn)
		obj.Status.State = string(out.CapacityReservation.State)
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return fmt.Errorf("persist capacity reservation ID after create: %w", err)
		}
		obj.Status.ObservedGeneration = obj.Generation
		now := metav1.Now()
		obj.Status.LastSyncTime = &now
		return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "CapacityReservation created")
	}

	out, err := r.EC2Client.DescribeCapacityReservations(ctx, &awsec2.DescribeCapacityReservationsInput{
		CapacityReservationIds: []string{obj.Status.CapacityReservationID},
	})
	if err != nil && !ec2helper.IsNotFound(err) {
		return fmt.Errorf("describe capacity reservation: %w", err)
	}
	if ec2helper.IsNotFound(err) || len(out.CapacityReservations) == 0 {
		obj.Status.CapacityReservationID = ""
		return r.reconcileReservation(ctx, obj)
	}
	cr := out.CapacityReservations[0]
	obj.Status.ARN = aws.ToString(cr.CapacityReservationArn)
	obj.Status.State = string(cr.State)
	if cr.State == ec2types.CapacityReservationStateCancelled || cr.State == ec2types.CapacityReservationStateExpired {
		// Cancelled/expired out of band — recreate on the next pass.
		obj.Status.CapacityReservationID = ""
		return r.reconcileReservation(ctx, obj)
	}

	if obj.Status.ObservedGeneration != obj.Generation {
		input := &awsec2.ModifyCapacityReservationInput{
			CapacityReservationId: aws.String(obj.Status.CapacityReservationID),
			InstanceCount:         aws.Int32(obj.Spec.InstanceCount),
		}
		if obj.Spec.EndDateType != "" {
			input.EndDateType = ec2types.EndDateType(obj.Spec.EndDateType)
		}
		if obj.Spec.EndDate != nil {
			input.EndDate = &obj.Spec.EndDate.Time
		}
		if _, err := r.EC2Client.ModifyCapacityReservation(ctx, input); err != nil {
			return fmt.Errorf("modify capacity reservation: %w", err)
		}
		if len(obj.Spec.Tags) > 0 {
			if _, err := r.EC2Client.CreateTags(ctx, &awsec2.CreateTagsInput{
				Resources: []string{obj.Status.CapacityReservationID},
				Tags:      ec2helper.TagsFromMap(obj.Spec.Tags),
			}); err != nil {
				return fmt.Errorf("tag capacity reservation: %w", err)
			}
		}
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "CapacityReservation reconciled")
}

func (r *CapacityReservationReconciler) cancelReservation(ctx context.Context, obj *awsv1alpha1.CapacityReservation) error {
	if obj.Status.CapacityReservationID == "" {
		return nil
	}
	_, err := r.EC2Client.CancelCapacityReservation(ctx, &awsec2.CancelCapacityReservationInput{
		CapacityReservationId: aws.String(obj.Status.CapacityReservationID),
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *CapacityReservationReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.CapacityReservation, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *CapacityReservationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CapacityReservation{}).
		Complete(r)
}
