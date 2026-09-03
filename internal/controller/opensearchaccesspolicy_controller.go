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
	awsoss "github.com/aws/aws-sdk-go-v2/service/opensearchserverless"
	osstypes "github.com/aws/aws-sdk-go-v2/service/opensearchserverless/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	osshelper "github.com/konfig-io/konfig-konector/internal/aws/opensearchserverless"
)

// OpenSearchAccessPolicyReconciler reconciles OpenSearchAccessPolicy objects.
type OpenSearchAccessPolicyReconciler struct {
	client.Client
	Scheme                     *runtime.Scheme
	OpenSearchServerlessClient *awsoss.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=opensearchaccesspolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=opensearchaccesspolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=opensearchaccesspolicies/finalizers,verbs=update

func (r *OpenSearchAccessPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.OpenSearchAccessPolicy{}
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
			if err := r.deletePolicy(ctx, obj); err != nil {
				logger.Error(err, "failed to delete OpenSearchAccessPolicy")
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

	if err := r.reconcilePolicy(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionOSAP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *OpenSearchAccessPolicyReconciler) reconcilePolicy(ctx context.Context, obj *awsv1alpha1.OpenSearchAccessPolicy) error {
	policyType := osstypes.AccessPolicyType(obj.Spec.Type)

	getOut, err := r.OpenSearchServerlessClient.GetAccessPolicy(ctx, &awsoss.GetAccessPolicyInput{
		Name: aws.String(obj.Spec.Name),
		Type: policyType,
	})
	if err != nil && !osshelper.IsNotFound(err) {
		return fmt.Errorf("get opensearch access policy: %w", err)
	}

	if err == nil && getOut.AccessPolicyDetail != nil {
		policyVersion := aws.ToString(getOut.AccessPolicyDetail.PolicyVersion)
		updateInput := &awsoss.UpdateAccessPolicyInput{
			Name:          aws.String(obj.Spec.Name),
			Type:          policyType,
			PolicyVersion: aws.String(policyVersion),
			Policy:        aws.String(obj.Spec.PolicyDocument),
		}
		if obj.Spec.Description != "" {
			updateInput.Description = aws.String(obj.Spec.Description)
		}
		updateOut, err := r.OpenSearchServerlessClient.UpdateAccessPolicy(ctx, updateInput)
		if err != nil {
			return fmt.Errorf("update opensearch access policy: %w", err)
		}
		if updateOut.AccessPolicyDetail != nil {
			obj.Status.PolicyVersion = aws.ToString(updateOut.AccessPolicyDetail.PolicyVersion)
		}
		obj.Status.ObservedGeneration = obj.Generation
		now := metav1.Now()
		obj.Status.LastSyncTime = &now
		return r.setConditionOSAP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "access policy reconciled")
	}

	createInput := &awsoss.CreateAccessPolicyInput{
		Name:   aws.String(obj.Spec.Name),
		Type:   policyType,
		Policy: aws.String(obj.Spec.PolicyDocument),
	}
	if obj.Spec.Description != "" {
		createInput.Description = aws.String(obj.Spec.Description)
	}
	createOut, err := r.OpenSearchServerlessClient.CreateAccessPolicy(ctx, createInput)
	if err != nil {
		return fmt.Errorf("create opensearch access policy: %w", err)
	}
	if createOut.AccessPolicyDetail != nil {
		obj.Status.PolicyVersion = aws.ToString(createOut.AccessPolicyDetail.PolicyVersion)
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionOSAP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "access policy created")
}

func (r *OpenSearchAccessPolicyReconciler) deletePolicy(ctx context.Context, obj *awsv1alpha1.OpenSearchAccessPolicy) error {
	_, err := r.OpenSearchServerlessClient.DeleteAccessPolicy(ctx, &awsoss.DeleteAccessPolicyInput{
		Name: aws.String(obj.Spec.Name),
		Type: osstypes.AccessPolicyType(obj.Spec.Type),
	})
	if osshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *OpenSearchAccessPolicyReconciler) setConditionOSAP(ctx context.Context, obj *awsv1alpha1.OpenSearchAccessPolicy, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *OpenSearchAccessPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.OpenSearchAccessPolicy{}).
		Complete(r)
}
