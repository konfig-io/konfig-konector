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

// ElasticIPReconciler reconciles ElasticIP objects.
type ElasticIPReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client *awsec2.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=elasticips,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=elasticips/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=elasticips/finalizers,verbs=update

func (r *ElasticIPReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	eip := &awsv1alpha1.ElasticIP{}
	if err := r.Get(ctx, req.NamespacedName, eip); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !eip.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(eip, awsv1alpha1.FinalizerName) {
			if shouldAbandon(eip) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(eip, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, eip)
			}
			if err := r.deleteElasticIP(ctx, eip); err != nil {
				logger.Error(err, "failed to delete ElasticIP")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(eip, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, eip)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(eip, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(eip, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, eip); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileElasticIP(ctx, eip); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionEIP(ctx, eip, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ElasticIPReconciler) reconcileElasticIP(ctx context.Context, eip *awsv1alpha1.ElasticIP) error {
	// If we already have an allocation ID, verify it still exists.
	if eip.Status.AllocationID != "" {
		out, err := r.EC2Client.DescribeAddresses(ctx, &awsec2.DescribeAddressesInput{
			AllocationIds: []string{eip.Status.AllocationID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return fmt.Errorf("describe elastic IP: %w", err)
		}
		if err == nil && len(out.Addresses) > 0 {
			// Already exists — sync tags.
			if err := r.syncEIPTags(ctx, eip, eip.Status.AllocationID); err != nil {
				return err
			}
			eip.Status.PublicIP = aws.ToString(out.Addresses[0].PublicIp)
			eip.Status.ObservedGeneration = eip.Generation
			now := metav1.Now()
			eip.Status.LastSyncTime = &now
			return r.setConditionEIP(ctx, eip, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ElasticIP reconciled")
		}
		// Allocation no longer exists — fall through to allocate.
		eip.Status.AllocationID = ""
	}

	// Allocate a new EIP.
	domain := eip.Spec.Domain
	if domain == "" {
		domain = "vpc"
	}
	allocOut, err := r.EC2Client.AllocateAddress(ctx, &awsec2.AllocateAddressInput{
		Domain: ec2types.DomainType(domain),
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeElasticIp,
				Tags:         ec2helper.TagsFromMap(eip.Spec.Tags),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("allocate elastic IP: %w", err)
	}

	eip.Status.AllocationID = aws.ToString(allocOut.AllocationId)
	eip.Status.PublicIP = aws.ToString(allocOut.PublicIp)
	if err := persistStatus(ctx, r.Client, eip); err != nil {
		return fmt.Errorf("persist ElasticIP allocation ID after create: %w", err)
	}
	eip.Status.ObservedGeneration = eip.Generation
	now := metav1.Now()
	eip.Status.LastSyncTime = &now
	return r.setConditionEIP(ctx, eip, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "ElasticIP allocated")
}

func (r *ElasticIPReconciler) syncEIPTags(ctx context.Context, eip *awsv1alpha1.ElasticIP, allocationID string) error {
	if len(eip.Spec.Tags) == 0 {
		return nil
	}
	_, err := r.EC2Client.CreateTags(ctx, &awsec2.CreateTagsInput{
		Resources: []string{allocationID},
		Tags:      ec2helper.TagsFromMap(eip.Spec.Tags),
	})
	return err
}

func (r *ElasticIPReconciler) deleteElasticIP(ctx context.Context, eip *awsv1alpha1.ElasticIP) error {
	allocationID := eip.Status.AllocationID
	if allocationID == "" {
		// Fallback: the status write may have been lost after allocate.
		// Look up by the tags the create path applied; without tags the
		// address is indistinguishable from others, so give up.
		if len(eip.Spec.Tags) == 0 {
			return nil
		}
		filters := make([]ec2types.Filter, 0, len(eip.Spec.Tags))
		for k, v := range eip.Spec.Tags {
			filters = append(filters, ec2types.Filter{Name: aws.String("tag:" + k), Values: []string{v}})
		}
		out, err := r.EC2Client.DescribeAddresses(ctx, &awsec2.DescribeAddressesInput{Filters: filters})
		if err != nil {
			return fmt.Errorf("lookup elastic IP by tags: %w", err)
		}
		if len(out.Addresses) != 1 {
			return nil
		}
		allocationID = aws.ToString(out.Addresses[0].AllocationId)
	}
	_, err := r.EC2Client.ReleaseAddress(ctx, &awsec2.ReleaseAddressInput{
		AllocationId: aws.String(allocationID),
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ElasticIPReconciler) setConditionEIP(ctx context.Context, eip *awsv1alpha1.ElasticIP, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&eip.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: eip.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, eip); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *ElasticIPReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ElasticIP{}).
		Complete(r)
}
