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

// VPNConnectionRouteAWSAPI is the subset of the EC2 API used by this controller.
type VPNConnectionRouteAWSAPI interface {
	CreateVpnConnectionRoute(ctx context.Context, params *awsec2.CreateVpnConnectionRouteInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateVpnConnectionRouteOutput, error)
	DeleteVpnConnectionRoute(ctx context.Context, params *awsec2.DeleteVpnConnectionRouteInput, optFns ...func(*awsec2.Options)) (*awsec2.DeleteVpnConnectionRouteOutput, error)
}

// VPNConnectionRouteReconciler reconciles VPNConnectionRoute objects.
type VPNConnectionRouteReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client VPNConnectionRouteAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpnconnectionroutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpnconnectionroutes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=vpnconnectionroutes/finalizers,verbs=update

func (r *VPNConnectionRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.VPNConnectionRoute{}
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
			if err := r.deleteRoute(ctx, obj); err != nil {
				logger.Error(err, "failed to delete VPNConnectionRoute")
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

	if err := r.reconcileRoute(ctx, obj); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *VPNConnectionRouteReconciler) reconcileRoute(ctx context.Context, obj *awsv1alpha1.VPNConnectionRoute) error {
	vpnID, err := r.resolveVPNConnectionID(ctx, obj)
	if err != nil {
		return err
	}

	// CreateVpnConnectionRoute is idempotent for an existing (connection, CIDR)
	// pair, so it is safe to call every reconcile.
	if _, err := r.EC2Client.CreateVpnConnectionRoute(ctx, &awsec2.CreateVpnConnectionRouteInput{
		VpnConnectionId:      aws.String(vpnID),
		DestinationCidrBlock: aws.String(obj.Spec.DestinationCIDRBlock),
	}); err != nil {
		return fmt.Errorf("create VPN connection route: %w", err)
	}

	obj.Status.VPNConnectionID = vpnID
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist VPN connection ID after create: %w", err)
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "VPNConnectionRoute reconciled")
}

func (r *VPNConnectionRouteReconciler) resolveVPNConnectionID(ctx context.Context, obj *awsv1alpha1.VPNConnectionRoute) (string, error) {
	ref := obj.Spec.VPNConnectionRef
	if ref.ID != "" {
		return ref.ID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("vpnConnectionRef requires name or id")
	}
	conn := &awsv1alpha1.VPNConnection{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: obj.Namespace}, conn); err != nil {
		return "", err
	}
	if conn.Status.VPNConnectionID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("VPNConnection %s/%s has no ID yet", obj.Namespace, ref.Name)}
	}
	return conn.Status.VPNConnectionID, nil
}

func (r *VPNConnectionRouteReconciler) deleteRoute(ctx context.Context, obj *awsv1alpha1.VPNConnectionRoute) error {
	vpnID := obj.Status.VPNConnectionID
	if vpnID == "" {
		// Fallback: the route target is deterministic from spec refs.
		id, err := r.resolveVPNConnectionID(ctx, obj)
		if err != nil {
			// The referenced VPN connection may already be gone.
			return nil
		}
		vpnID = id
	}
	_, err := r.EC2Client.DeleteVpnConnectionRoute(ctx, &awsec2.DeleteVpnConnectionRouteInput{
		VpnConnectionId:      aws.String(vpnID),
		DestinationCidrBlock: aws.String(obj.Spec.DestinationCIDRBlock),
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *VPNConnectionRouteReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.VPNConnectionRoute, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *VPNConnectionRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.VPNConnectionRoute{}).
		Complete(r)
}
