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
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// SpotFleetReconciler reconciles SpotFleet objects.
type SpotFleetReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client *multi.EC2
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=spotfleets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=spotfleets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=spotfleets/finalizers,verbs=update

func (r *SpotFleetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	sf := &awsv1alpha1.SpotFleet{}
	if err := r.Get(ctx, req.NamespacedName, sf); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, sf); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !sf.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(sf, awsv1alpha1.FinalizerName) {
			if shouldAbandon(sf) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(sf, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, sf)
			}
			if err := r.deleteSpotFleet(ctx, sf); err != nil {
				logger.Error(err, "failed to delete SpotFleet")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(sf, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, sf)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(sf, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(sf, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, sf); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileSpotFleet(ctx, sf); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionSF(ctx, sf, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *SpotFleetReconciler) reconcileSpotFleet(ctx context.Context, sf *awsv1alpha1.SpotFleet) error {
	if sf.Status.SpotFleetRequestID != "" {
		out, err := r.EC2Client.DescribeSpotFleetRequests(ctx, &awsec2.DescribeSpotFleetRequestsInput{
			SpotFleetRequestIds: []string{sf.Status.SpotFleetRequestID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return fmt.Errorf("describe spot fleet: %w", err)
		}
		if err == nil && len(out.SpotFleetRequestConfigs) > 0 {
			sf.Status.RequestState = string(out.SpotFleetRequestConfigs[0].SpotFleetRequestState)
			sf.Status.ObservedGeneration = sf.Generation
			now := metav1.Now()
			sf.Status.LastSyncTime = &now
			return r.setConditionSF(ctx, sf, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "SpotFleet reconciled")
		}
		sf.Status.SpotFleetRequestID = ""
	}

	launchSpecs := make([]ec2types.SpotFleetLaunchSpecification, 0, len(sf.Spec.LaunchSpecs))
	for _, ls := range sf.Spec.LaunchSpecs {
		spec := ec2types.SpotFleetLaunchSpecification{
			ImageId:      aws.String(ls.ImageID),
			InstanceType: ec2types.InstanceType(ls.InstanceType),
		}
		if ls.SubnetID != "" {
			spec.SubnetId = aws.String(ls.SubnetID)
		}
		if ls.KeyName != "" {
			spec.KeyName = aws.String(ls.KeyName)
		}
		if len(ls.SecurityGroupIDs) > 0 {
			groups := make([]ec2types.GroupIdentifier, 0, len(ls.SecurityGroupIDs))
			for _, sg := range ls.SecurityGroupIDs {
				sg := sg
				groups = append(groups, ec2types.GroupIdentifier{GroupId: &sg})
			}
			spec.SecurityGroups = groups
		}
		if ls.SpotPrice != "" {
			spec.SpotPrice = aws.String(ls.SpotPrice)
		}
		if ls.WeightedCapacity > 0 {
			spec.WeightedCapacity = aws.Float64(ls.WeightedCapacity)
		}
		launchSpecs = append(launchSpecs, spec)
	}

	config := ec2types.SpotFleetRequestConfigData{
		IamFleetRole:         aws.String(sf.Spec.IAMFleetRole),
		TargetCapacity:       aws.Int32(sf.Spec.TargetCapacity),
		LaunchSpecifications: launchSpecs,
	}
	if sf.Spec.AllocationStrategy != "" {
		config.AllocationStrategy = ec2types.AllocationStrategy(sf.Spec.AllocationStrategy)
	}
	if sf.Spec.SpotPrice != "" {
		config.SpotPrice = aws.String(sf.Spec.SpotPrice)
	}
	if sf.Spec.TerminateInstancesWithExpiration {
		config.TerminateInstancesWithExpiration = aws.Bool(true)
	}
	if sf.Spec.ValidUntil != nil {
		t := sf.Spec.ValidUntil.Time
		config.ValidUntil = &t
	}
	if len(sf.Spec.Tags) > 0 {
		config.TagSpecifications = []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeSpotFleetRequest,
				Tags:         ec2helper.TagsFromMap(sf.Spec.Tags),
			},
		}
	}

	out, err := r.EC2Client.RequestSpotFleet(ctx, &awsec2.RequestSpotFleetInput{
		SpotFleetRequestConfig: &config,
	})
	if err != nil {
		return fmt.Errorf("request spot fleet: %w", err)
	}

	sf.Status.SpotFleetRequestID = aws.ToString(out.SpotFleetRequestId)
	sf.Status.RequestState = "submitted"
	// RequestSpotFleet is not idempotent and the existence check is gated on
	// Status.SpotFleetRequestID, so persisting the ID here is the primary
	// protection against duplicating the fleet request on retry.
	if err := persistStatus(ctx, r.Client, sf); err != nil {
		return fmt.Errorf("persist spot fleet request ID after create: %w", err)
	}
	sf.Status.ObservedGeneration = sf.Generation
	now := metav1.Now()
	sf.Status.LastSyncTime = &now
	return r.setConditionSF(ctx, sf, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "SpotFleet request submitted")
}

func (r *SpotFleetReconciler) deleteSpotFleet(ctx context.Context, sf *awsv1alpha1.SpotFleet) error {
	requestID := sf.Status.SpotFleetRequestID
	if requestID == "" {
		// Fallback: the status write may have been lost after create.
		// DescribeSpotFleetRequests has no filters, but the create path tags
		// the request, so find it via DescribeTags. Without tags the request
		// is indistinguishable from others, so give up (the persist right
		// after RequestSpotFleet keeps that window tiny).
		if len(sf.Spec.Tags) == 0 {
			return nil
		}
		filters := []ec2types.Filter{
			{Name: aws.String("resource-type"), Values: []string{"spot-fleet-request"}},
		}
		for k, v := range sf.Spec.Tags {
			filters = append(filters, ec2types.Filter{Name: aws.String("tag:" + k), Values: []string{v}})
		}
		out, err := r.EC2Client.DescribeTags(ctx, &awsec2.DescribeTagsInput{Filters: filters})
		if err != nil {
			return fmt.Errorf("lookup spot fleet request by tags: %w", err)
		}
		ids := make(map[string]bool)
		for _, t := range out.Tags {
			ids[aws.ToString(t.ResourceId)] = true
		}
		if len(ids) != 1 {
			return nil
		}
		for id := range ids {
			requestID = id
		}
	}
	_, err := r.EC2Client.CancelSpotFleetRequests(ctx, &awsec2.CancelSpotFleetRequestsInput{
		SpotFleetRequestIds: []string{requestID},
		TerminateInstances:  aws.Bool(true),
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SpotFleetReconciler) setConditionSF(ctx context.Context, sf *awsv1alpha1.SpotFleet, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&sf.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: sf.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, sf); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *SpotFleetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.SpotFleet{}).
		Complete(r)
}
