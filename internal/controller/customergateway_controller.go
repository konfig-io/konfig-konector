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

// CustomerGatewayAWSAPI is the subset of the EC2 API used by this controller.
type CustomerGatewayAWSAPI interface {
	DescribeCustomerGateways(ctx context.Context, params *awsec2.DescribeCustomerGatewaysInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeCustomerGatewaysOutput, error)
	CreateCustomerGateway(ctx context.Context, params *awsec2.CreateCustomerGatewayInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateCustomerGatewayOutput, error)
	DeleteCustomerGateway(ctx context.Context, params *awsec2.DeleteCustomerGatewayInput, optFns ...func(*awsec2.Options)) (*awsec2.DeleteCustomerGatewayOutput, error)
	CreateTags(ctx context.Context, params *awsec2.CreateTagsInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateTagsOutput, error)
}

// CustomerGatewayReconciler reconciles CustomerGateway objects.
type CustomerGatewayReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client CustomerGatewayAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=customergateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=customergateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=customergateways/finalizers,verbs=update

func (r *CustomerGatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.CustomerGateway{}
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
			if err := r.deleteCustomerGateway(ctx, obj); err != nil {
				logger.Error(err, "failed to delete CustomerGateway")
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

	if err := r.reconcileCustomerGateway(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *CustomerGatewayReconciler) reconcileCustomerGateway(ctx context.Context, obj *awsv1alpha1.CustomerGateway) error {
	if obj.Status.CustomerGatewayID != "" {
		out, err := r.EC2Client.DescribeCustomerGateways(ctx, &awsec2.DescribeCustomerGatewaysInput{
			CustomerGatewayIds: []string{obj.Status.CustomerGatewayID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return fmt.Errorf("describe customer gateway: %w", err)
		}
		if err == nil && len(out.CustomerGateways) > 0 && aws.ToString(out.CustomerGateways[0].State) != "deleted" {
			obj.Status.State = aws.ToString(out.CustomerGateways[0].State)
			if len(obj.Spec.Tags) > 0 {
				if _, tagErr := r.EC2Client.CreateTags(ctx, &awsec2.CreateTagsInput{
					Resources: []string{obj.Status.CustomerGatewayID},
					Tags:      ec2helper.TagsFromMap(obj.Spec.Tags),
				}); tagErr != nil {
					return fmt.Errorf("tag customer gateway: %w", tagErr)
				}
			}
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "CustomerGateway reconciled")
		}
		// Deleted out of band — recreate.
		obj.Status.CustomerGatewayID = ""
	}

	gwType := obj.Spec.Type
	if gwType == "" {
		gwType = "ipsec.1"
	}
	input := &awsec2.CreateCustomerGatewayInput{
		Type:      ec2types.GatewayType(gwType),
		BgpAsn:    aws.Int32(obj.Spec.BGPASN),
		IpAddress: aws.String(obj.Spec.IPAddress),
		TagSpecifications: []ec2types.TagSpecification{
			{ResourceType: ec2types.ResourceTypeCustomerGateway, Tags: ec2helper.TagsFromMap(obj.Spec.Tags)},
		},
	}
	if obj.Spec.DeviceName != "" {
		input.DeviceName = aws.String(obj.Spec.DeviceName)
	}
	out, err := r.EC2Client.CreateCustomerGateway(ctx, input)
	if err != nil {
		return fmt.Errorf("create customer gateway: %w", err)
	}
	obj.Status.CustomerGatewayID = aws.ToString(out.CustomerGateway.CustomerGatewayId)
	obj.Status.State = aws.ToString(out.CustomerGateway.State)
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist customer gateway ID after create: %w", err)
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "CustomerGateway created")
}

func (r *CustomerGatewayReconciler) deleteCustomerGateway(ctx context.Context, obj *awsv1alpha1.CustomerGateway) error {
	id := obj.Status.CustomerGatewayID
	if id == "" {
		// Fallback: the IP address + BGP ASN pair is deterministic from spec.
		out, err := r.EC2Client.DescribeCustomerGateways(ctx, &awsec2.DescribeCustomerGatewaysInput{
			Filters: []ec2types.Filter{
				{Name: aws.String("ip-address"), Values: []string{obj.Spec.IPAddress}},
				{Name: aws.String("state"), Values: []string{"available", "pending"}},
			},
		})
		if err != nil || len(out.CustomerGateways) != 1 {
			return nil
		}
		id = aws.ToString(out.CustomerGateways[0].CustomerGatewayId)
	}
	_, err := r.EC2Client.DeleteCustomerGateway(ctx, &awsec2.DeleteCustomerGatewayInput{
		CustomerGatewayId: aws.String(id),
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *CustomerGatewayReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.CustomerGateway, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *CustomerGatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CustomerGateway{}).
		Complete(r)
}
