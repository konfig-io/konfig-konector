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
	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	dynamohelper "github.com/konfig-io/konfig-konector/internal/aws/dynamodb"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// DynamoDBTablePolicyReconciler reconciles DynamoDBTablePolicy objects.
type DynamoDBTablePolicyReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	DynamoDBClient *multi.DynamoDB
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=dynamodbtablepolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dynamodbtablepolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dynamodbtablepolicies/finalizers,verbs=update

func (r *DynamoDBTablePolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.DynamoDBTablePolicy{}
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
			if err := r.deletePolicy(ctx, obj); err != nil {
				logger.Error(err, "failed to delete DynamoDBTablePolicy")
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

	if err := r.reconcilePolicy(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionDTP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *DynamoDBTablePolicyReconciler) reconcilePolicy(ctx context.Context, obj *awsv1alpha1.DynamoDBTablePolicy) error {
	out, err := r.DynamoDBClient.PutResourcePolicy(ctx, &awsdynamodb.PutResourcePolicyInput{
		ResourceArn: aws.String(obj.Spec.ResourceARN),
		Policy:      aws.String(obj.Spec.PolicyDocument),
	})
	if err != nil {
		return fmt.Errorf("put dynamodb resource policy: %w", err)
	}

	if out.RevisionId != nil {
		obj.Status.RevisionID = *out.RevisionId
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionDTP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "DynamoDBTablePolicy reconciled")
}

func (r *DynamoDBTablePolicyReconciler) deletePolicy(ctx context.Context, obj *awsv1alpha1.DynamoDBTablePolicy) error {
	_, err := r.DynamoDBClient.DeleteResourcePolicy(ctx, &awsdynamodb.DeleteResourcePolicyInput{
		ResourceArn: aws.String(obj.Spec.ResourceARN),
	})
	if dynamohelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *DynamoDBTablePolicyReconciler) setConditionDTP(ctx context.Context, obj *awsv1alpha1.DynamoDBTablePolicy, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *DynamoDBTablePolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.DynamoDBTablePolicy{}).
		Complete(r)
}
