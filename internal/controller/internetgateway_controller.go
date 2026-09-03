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
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
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

// InternetGatewayReconciler reconciles InternetGateway objects.
type InternetGatewayReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client *awsec2.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=internetgateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=internetgateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=internetgateways/finalizers,verbs=update

func (r *InternetGatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	igw := &awsv1alpha1.InternetGateway{}
	if err := r.Get(ctx, req.NamespacedName, igw); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !igw.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(igw, awsv1alpha1.FinalizerName) {
			if shouldAbandon(igw) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(igw, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, igw)
			}
			if igw.Status.InternetGatewayID != "" {
				// Detach from VPC first, then delete.
				vpcID, _ := r.resolveVPCID(ctx, igw.Namespace, &igw.Spec.VPCRef)
				if vpcID != "" {
					if _, err := r.EC2Client.DetachInternetGateway(ctx, &awsec2.DetachInternetGatewayInput{
						InternetGatewayId: aws.String(igw.Status.InternetGatewayID),
						VpcId:             aws.String(vpcID),
					}); err != nil && !ec2helper.IsNotFound(err) {
						logger.Error(err, "failed to detach internet gateway", "igwId", igw.Status.InternetGatewayID)
						return ctrl.Result{}, err
					}
				}
				if _, err := r.EC2Client.DeleteInternetGateway(ctx, &awsec2.DeleteInternetGatewayInput{
					InternetGatewayId: aws.String(igw.Status.InternetGatewayID),
				}); err != nil && !ec2helper.IsNotFound(err) {
					logger.Error(err, "failed to delete internet gateway", "igwId", igw.Status.InternetGatewayID)
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(igw, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, igw)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(igw, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(igw, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, igw); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileIGW(ctx, igw); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, igw, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *InternetGatewayReconciler) reconcileIGW(ctx context.Context, igw *awsv1alpha1.InternetGateway) error {
	vpcID, err := r.resolveVPCID(ctx, igw.Namespace, &igw.Spec.VPCRef)
	if err != nil {
		return err
	}

	igwID := igw.Status.InternetGatewayID
	if igwID != "" {
		out, err := r.EC2Client.DescribeInternetGateways(ctx, &awsec2.DescribeInternetGatewaysInput{
			InternetGatewayIds: []string{igwID},
		})
		if err != nil && !ec2helper.IsNotFound(err) {
			return err
		}
		if err != nil || len(out.InternetGateways) == 0 {
			igwID = ""
		}
	}

	if igwID == "" {
		out, err := r.EC2Client.CreateInternetGateway(ctx, &awsec2.CreateInternetGatewayInput{
			TagSpecifications: []types.TagSpecification{
				{ResourceType: types.ResourceTypeInternetGateway, Tags: ec2helper.TagsFromMap(igw.Spec.Tags)},
			},
		})
		if err != nil {
			return fmt.Errorf("create internet gateway: %w", err)
		}
		igwID = aws.ToString(out.InternetGateway.InternetGatewayId)
	}

	igw.Status.InternetGatewayID = igwID

	// Ensure IGW is attached to the VPC.
	out, err := r.EC2Client.DescribeInternetGateways(ctx, &awsec2.DescribeInternetGatewaysInput{
		InternetGatewayIds: []string{igwID},
	})
	if err != nil {
		return err
	}
	attached := false
	if len(out.InternetGateways) > 0 {
		for _, att := range out.InternetGateways[0].Attachments {
			if aws.ToString(att.VpcId) == vpcID {
				attached = true
				break
			}
		}
	}
	if !attached {
		if _, err := r.EC2Client.AttachInternetGateway(ctx, &awsec2.AttachInternetGatewayInput{
			InternetGatewayId: aws.String(igwID),
			VpcId:             aws.String(vpcID),
		}); err != nil {
			return fmt.Errorf("attach internet gateway: %w", err)
		}
	}

	if err := ec2helper.SyncResourceTags(ctx, r.EC2Client, igwID, "internet-gateway", igw.Spec.Tags); err != nil {
		return fmt.Errorf("sync tags: %w", err)
	}

	igw.Status.ObservedGeneration = igw.Generation
	now := metav1.Now()
	igw.Status.LastSyncTime = &now
	return r.setCondition(ctx, igw, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "internet gateway reconciled")
}

func (r *InternetGatewayReconciler) resolveVPCID(ctx context.Context, namespace string, ref *awsv1alpha1.VPCResourceRef) (string, error) {
	if ref.ID != "" {
		return ref.ID, nil
	}
	if ref.Name != "" {
		vpcCR := &awsv1alpha1.VPC{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, vpcCR); err != nil {
			return "", err
		}
		if vpcCR.Status.VPCID == "" {
			return "", &dependencyNotReady{msg: fmt.Sprintf("VPC %s/%s has no ID yet", namespace, ref.Name)}
		}
		return vpcCR.Status.VPCID, nil
	}
	return "", fmt.Errorf("vpcRef requires either name or id")
}

func (r *InternetGatewayReconciler) setCondition(ctx context.Context, igw *awsv1alpha1.InternetGateway, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&igw.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: igw.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, igw); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *InternetGatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.InternetGateway{}).
		Complete(r)
}
