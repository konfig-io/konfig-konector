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
	awsr53r "github.com/aws/aws-sdk-go-v2/service/route53resolver"
	r53rtypes "github.com/aws/aws-sdk-go-v2/service/route53resolver/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	r53rhelper "github.com/konfig-io/konfig-konector/internal/aws/route53resolver"
)

// ResolverEndpointReconciler reconciles ResolverEndpoint objects.
type ResolverEndpointReconciler struct {
	client.Client
	Scheme                *runtime.Scheme
	Route53ResolverClient *awsr53r.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=resolverendpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=resolverendpoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=resolverendpoints/finalizers,verbs=update

func (r *ResolverEndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.ResolverEndpoint{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
			if shouldAbandon(obj) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, obj)
			}
			if err := r.deleteEndpoint(ctx, obj); err != nil {
				logger.Error(err, "failed to delete ResolverEndpoint")
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
	}

	if err := r.reconcileEndpoint(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionRE(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ResolverEndpointReconciler) reconcileEndpoint(ctx context.Context, obj *awsv1alpha1.ResolverEndpoint) error {
	if obj.Status.EndpointID != "" {
		getOut, err := r.Route53ResolverClient.GetResolverEndpoint(ctx, &awsr53r.GetResolverEndpointInput{
			ResolverEndpointId: aws.String(obj.Status.EndpointID),
		})
		if err != nil && !r53rhelper.IsNotFound(err) {
			return fmt.Errorf("get resolver endpoint: %w", err)
		}
		if err == nil && getOut.ResolverEndpoint != nil {
			ep := getOut.ResolverEndpoint
			obj.Status.Status = string(ep.Status)
			obj.Status.HostVPCID = aws.ToString(ep.HostVPCId)

			if ep.Status == r53rtypes.ResolverEndpointStatusOperational {
				obj.Status.ObservedGeneration = obj.Generation
				now := metav1.Now()
				obj.Status.LastSyncTime = &now
				return r.setConditionRE(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ResolverEndpoint operational")
			}
			return r.setConditionRE(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonCreated, fmt.Sprintf("endpoint status: %s", ep.Status))
		}
		obj.Status.EndpointID = ""
	}

	ipAddresses := make([]r53rtypes.IpAddressRequest, 0, len(obj.Spec.IPAddresses))
	for _, ip := range obj.Spec.IPAddresses {
		req := r53rtypes.IpAddressRequest{
			SubnetId: aws.String(ip.SubnetID),
		}
		if ip.IP != "" {
			req.Ip = aws.String(ip.IP)
		}
		ipAddresses = append(ipAddresses, req)
	}

	input := &awsr53r.CreateResolverEndpointInput{
		Name:             aws.String(obj.Spec.Name),
		Direction:        r53rtypes.ResolverEndpointDirection(obj.Spec.Direction),
		SecurityGroupIds: obj.Spec.SecurityGroupIDs,
		IpAddresses:      ipAddresses,
		CreatorRequestId: aws.String(string(obj.UID)),
	}
	if len(obj.Spec.Tags) > 0 {
		tags := make([]r53rtypes.Tag, 0, len(obj.Spec.Tags))
		for k, v := range obj.Spec.Tags {
			k, v := k, v
			tags = append(tags, r53rtypes.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.Route53ResolverClient.CreateResolverEndpoint(ctx, input)
	if err != nil {
		return fmt.Errorf("create resolver endpoint: %w", err)
	}

	if out.ResolverEndpoint != nil {
		obj.Status.EndpointID = aws.ToString(out.ResolverEndpoint.Id)
		obj.Status.Status = string(out.ResolverEndpoint.Status)
		obj.Status.HostVPCID = aws.ToString(out.ResolverEndpoint.HostVPCId)
		// The AWS resource now exists; losing the ID would orphan it.
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return fmt.Errorf("persist status after create: %w", err)
		}
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionRE(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonCreated, "ResolverEndpoint creating")
}

func (r *ResolverEndpointReconciler) deleteEndpoint(ctx context.Context, obj *awsv1alpha1.ResolverEndpoint) error {
	if obj.Status.EndpointID == "" {
		// Status may have been lost after a successful create; the create
		// path sets CreatorRequestId to this CR's UID, so filter on it to
		// find only the endpoint this CR created before giving up.
		paginator := awsr53r.NewListResolverEndpointsPaginator(r.Route53ResolverClient, &awsr53r.ListResolverEndpointsInput{
			Filters: []r53rtypes.Filter{{
				Name:   aws.String("CreatorRequestId"),
				Values: []string{string(obj.UID)},
			}},
		})
		for paginator.HasMorePages() && obj.Status.EndpointID == "" {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				return fmt.Errorf("list resolver endpoints: %w", err)
			}
			for _, ep := range page.ResolverEndpoints {
				if aws.ToString(ep.CreatorRequestId) == string(obj.UID) {
					obj.Status.EndpointID = aws.ToString(ep.Id)
					break
				}
			}
		}
		if obj.Status.EndpointID == "" {
			return nil
		}
	}
	_, err := r.Route53ResolverClient.DeleteResolverEndpoint(ctx, &awsr53r.DeleteResolverEndpointInput{
		ResolverEndpointId: aws.String(obj.Status.EndpointID),
	})
	if r53rhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ResolverEndpointReconciler) setConditionRE(ctx context.Context, obj *awsv1alpha1.ResolverEndpoint, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *ResolverEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ResolverEndpoint{}).
		Complete(r)
}
