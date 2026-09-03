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
	awseks "github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ekshelper "github.com/konfig-io/konfig-konector/internal/aws/eks"
)

// EKSIdentityProviderConfigReconciler reconciles EKSIdentityProviderConfig objects.
type EKSIdentityProviderConfigReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EKSClient *awseks.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=eksidentityproviderconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eksidentityproviderconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eksidentityproviderconfigs/finalizers,verbs=update

func (r *EKSIdentityProviderConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.EKSIdentityProviderConfig{}
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
			if err := r.deleteIdentityProviderConfig(ctx, obj); err != nil {
				logger.Error(err, "failed to delete EKSIdentityProviderConfig")
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

	if err := r.reconcileIdentityProviderConfig(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionEIPC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *EKSIdentityProviderConfigReconciler) reconcileIdentityProviderConfig(ctx context.Context, obj *awsv1alpha1.EKSIdentityProviderConfig) error {
	if obj.Status.ARN != "" {
		out, err := r.EKSClient.DescribeIdentityProviderConfig(ctx, &awseks.DescribeIdentityProviderConfigInput{
			ClusterName: aws.String(obj.Spec.ClusterName),
			IdentityProviderConfig: &ekstypes.IdentityProviderConfig{
				Name: aws.String(obj.Spec.IdentityProviderConfigName),
				Type: aws.String("oidc"),
			},
		})
		if err != nil && !ekshelper.IsNotFound(err) {
			return fmt.Errorf("describe eks identity provider config: %w", err)
		}
		if err == nil && out.IdentityProviderConfig != nil && out.IdentityProviderConfig.Oidc != nil {
			oidc := out.IdentityProviderConfig.Oidc
			obj.Status.ARN = aws.ToString(oidc.IdentityProviderConfigArn)
			obj.Status.Status = string(oidc.Status)
			if oidc.Status == ekstypes.ConfigStatusCreating {
				return nil
			}
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionEIPC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "EKSIdentityProviderConfig reconciled")
		}
		obj.Status.ARN = ""
	}

	oidcReq := &ekstypes.OidcIdentityProviderConfigRequest{
		ClientId:                   aws.String(obj.Spec.OIDC.ClientID),
		IssuerUrl:                  aws.String(obj.Spec.OIDC.IssuerURL),
		IdentityProviderConfigName: aws.String(obj.Spec.IdentityProviderConfigName),
	}
	if obj.Spec.OIDC.UsernameClaim != "" {
		oidcReq.UsernameClaim = aws.String(obj.Spec.OIDC.UsernameClaim)
	}
	if obj.Spec.OIDC.UsernamePrefix != "" {
		oidcReq.UsernamePrefix = aws.String(obj.Spec.OIDC.UsernamePrefix)
	}
	if obj.Spec.OIDC.GroupsClaim != "" {
		oidcReq.GroupsClaim = aws.String(obj.Spec.OIDC.GroupsClaim)
	}
	if obj.Spec.OIDC.GroupsPrefix != "" {
		oidcReq.GroupsPrefix = aws.String(obj.Spec.OIDC.GroupsPrefix)
	}
	if len(obj.Spec.OIDC.RequiredClaims) > 0 {
		oidcReq.RequiredClaims = obj.Spec.OIDC.RequiredClaims
	}

	tags := make(map[string]string, len(obj.Spec.Tags))
	for k, v := range obj.Spec.Tags {
		tags[k] = v
	}

	_, err := r.EKSClient.AssociateIdentityProviderConfig(ctx, &awseks.AssociateIdentityProviderConfigInput{
		ClusterName: aws.String(obj.Spec.ClusterName),
		Oidc:        oidcReq,
		Tags:        tags,
	})
	if err != nil {
		return fmt.Errorf("associate eks identity provider config: %w", err)
	}

	obj.Status.ARN = "pending"
	obj.Status.Status = string(ekstypes.ConfigStatusCreating)
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionEIPC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "EKSIdentityProviderConfig created")
}

func (r *EKSIdentityProviderConfigReconciler) deleteIdentityProviderConfig(ctx context.Context, obj *awsv1alpha1.EKSIdentityProviderConfig) error {
	_, err := r.EKSClient.DisassociateIdentityProviderConfig(ctx, &awseks.DisassociateIdentityProviderConfigInput{
		ClusterName: aws.String(obj.Spec.ClusterName),
		IdentityProviderConfig: &ekstypes.IdentityProviderConfig{
			Name: aws.String(obj.Spec.IdentityProviderConfigName),
			Type: aws.String("oidc"),
		},
	})
	if ekshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *EKSIdentityProviderConfigReconciler) setConditionEIPC(ctx context.Context, obj *awsv1alpha1.EKSIdentityProviderConfig, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *EKSIdentityProviderConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.EKSIdentityProviderConfig{}).
		Complete(r)
}
