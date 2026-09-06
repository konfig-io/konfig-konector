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
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
	osshelper "github.com/konfig-io/konfig-konector/internal/aws/opensearchserverless"
)

// OpenSearchServerlessCollectionReconciler reconciles OpenSearchServerlessCollection objects.
type OpenSearchServerlessCollectionReconciler struct {
	client.Client
	Scheme                     *runtime.Scheme
	OpenSearchServerlessClient *multi.OpenSearchServerless
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=opensearchserverlesscollections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=opensearchserverlesscollections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=opensearchserverlesscollections/finalizers,verbs=update

func (r *OpenSearchServerlessCollectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.OpenSearchServerlessCollection{}
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
			if err := r.deleteCollection(ctx, obj); err != nil {
				logger.Error(err, "failed to delete OpenSearchServerlessCollection")
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

	if err := r.reconcileCollection(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionOSSC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *OpenSearchServerlessCollectionReconciler) reconcileCollection(ctx context.Context, obj *awsv1alpha1.OpenSearchServerlessCollection) error {
	if obj.Status.CollectionID != "" {
		batchOut, err := r.OpenSearchServerlessClient.BatchGetCollection(ctx, &awsoss.BatchGetCollectionInput{
			Ids: []string{obj.Status.CollectionID},
		})
		if err != nil && !osshelper.IsNotFound(err) {
			return fmt.Errorf("get opensearch serverless collection: %w", err)
		}
		if err == nil && len(batchOut.CollectionDetails) > 0 {
			detail := batchOut.CollectionDetails[0]
			obj.Status.CollectionARN = aws.ToString(detail.Arn)
			obj.Status.Status = string(detail.Status)
			if detail.CollectionEndpoint != nil {
				obj.Status.CollectionEndpoint = *detail.CollectionEndpoint
			}

			if detail.Status == osstypes.CollectionStatusActive {
				obj.Status.ObservedGeneration = obj.Generation
				now := metav1.Now()
				obj.Status.LastSyncTime = &now
				return r.setConditionOSSC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "collection active")
			}
			return r.setConditionOSSC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonCreated, fmt.Sprintf("collection status: %s", detail.Status))
		}
		obj.Status.CollectionID = ""
		obj.Status.CollectionARN = ""
	}

	input := &awsoss.CreateCollectionInput{
		Name: aws.String(obj.Spec.Name),
	}
	if obj.Spec.Type != "" {
		input.Type = osstypes.CollectionType(obj.Spec.Type)
	}
	if obj.Spec.Description != "" {
		input.Description = aws.String(obj.Spec.Description)
	}
	if len(obj.Spec.Tags) > 0 {
		tags := make([]osstypes.Tag, 0, len(obj.Spec.Tags))
		for k, v := range obj.Spec.Tags {
			k, v := k, v
			tags = append(tags, osstypes.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.OpenSearchServerlessClient.CreateCollection(ctx, input)
	if err != nil {
		return fmt.Errorf("create opensearch serverless collection: %w", err)
	}

	if out.CreateCollectionDetail != nil {
		obj.Status.CollectionID = aws.ToString(out.CreateCollectionDetail.Id)
		obj.Status.CollectionARN = aws.ToString(out.CreateCollectionDetail.Arn)
		obj.Status.Status = string(out.CreateCollectionDetail.Status)
		// The AWS resource now exists; losing the ID would orphan it and a
		// retried create-by-name would conflict.
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return fmt.Errorf("persist status after create: %w", err)
		}
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionOSSC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonCreated, "collection creating")
}

func (r *OpenSearchServerlessCollectionReconciler) deleteCollection(ctx context.Context, obj *awsv1alpha1.OpenSearchServerlessCollection) error {
	if obj.Status.CollectionID == "" {
		// Status may have been lost after a successful create; the create
		// path always uses spec.name (unique per account), so look it up
		// before giving up.
		batchOut, err := r.OpenSearchServerlessClient.BatchGetCollection(ctx, &awsoss.BatchGetCollectionInput{
			Names: []string{obj.Spec.Name},
		})
		if err != nil && !osshelper.IsNotFound(err) {
			return fmt.Errorf("get opensearch serverless collection by name: %w", err)
		}
		if err != nil || len(batchOut.CollectionDetails) == 0 {
			return nil
		}
		obj.Status.CollectionID = aws.ToString(batchOut.CollectionDetails[0].Id)
	}
	_, err := r.OpenSearchServerlessClient.DeleteCollection(ctx, &awsoss.DeleteCollectionInput{
		Id: aws.String(obj.Status.CollectionID),
	})
	if osshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *OpenSearchServerlessCollectionReconciler) setConditionOSSC(ctx context.Context, obj *awsv1alpha1.OpenSearchServerlessCollection, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *OpenSearchServerlessCollectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.OpenSearchServerlessCollection{}).
		Complete(r)
}
