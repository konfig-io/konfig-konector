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
	awsssm "github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ssmhelper "github.com/konfig-io/konfig-konector/internal/aws/ssm"
)

// SSMDocumentReconciler reconciles SSMDocument objects.
type SSMDocumentReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	SSMClient *awsssm.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=ssmdocuments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ssmdocuments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ssmdocuments/finalizers,verbs=update

func (r *SSMDocumentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.SSMDocument{}
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
			if err := r.deleteDocument(ctx, obj); err != nil {
				logger.Error(err, "failed to delete SSMDocument")
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

	if err := r.reconcileDocument(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionSD(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *SSMDocumentReconciler) reconcileDocument(ctx context.Context, obj *awsv1alpha1.SSMDocument) error {
	descOut, err := r.SSMClient.DescribeDocument(ctx, &awsssm.DescribeDocumentInput{
		Name: aws.String(obj.Spec.Name),
	})
	if err != nil && !ssmhelper.IsNotFound(err) {
		return fmt.Errorf("describe ssm document: %w", err)
	}

	if err == nil && descOut.Document != nil {
		obj.Status.DocumentVersion = aws.ToString(descOut.Document.DocumentVersion)
		obj.Status.Status = string(descOut.Document.Status)

		updateInput := &awsssm.UpdateDocumentInput{
			Name:            aws.String(obj.Spec.Name),
			Content:         aws.String(obj.Spec.Content),
			DocumentVersion: aws.String("$LATEST"),
		}
		if obj.Spec.DocumentFormat != "" {
			updateInput.DocumentFormat = ssmtypes.DocumentFormat(obj.Spec.DocumentFormat)
		}
		if obj.Spec.VersionName != "" {
			updateInput.VersionName = aws.String(obj.Spec.VersionName)
		}
		out, err := r.SSMClient.UpdateDocument(ctx, updateInput)
		if err != nil {
			return fmt.Errorf("update ssm document: %w", err)
		}
		if out.DocumentDescription != nil {
			obj.Status.DocumentVersion = aws.ToString(out.DocumentDescription.DocumentVersion)
			obj.Status.Status = string(out.DocumentDescription.Status)
		}
	} else {
		tags := make([]ssmtypes.Tag, 0, len(obj.Spec.Tags))
		for k, v := range obj.Spec.Tags {
			k, v := k, v
			tags = append(tags, ssmtypes.Tag{Key: &k, Value: &v})
		}

		createInput := &awsssm.CreateDocumentInput{
			Name:    aws.String(obj.Spec.Name),
			Content: aws.String(obj.Spec.Content),
		}
		if obj.Spec.DocumentType != "" {
			createInput.DocumentType = ssmtypes.DocumentType(obj.Spec.DocumentType)
		}
		if obj.Spec.DocumentFormat != "" {
			createInput.DocumentFormat = ssmtypes.DocumentFormat(obj.Spec.DocumentFormat)
		}
		if obj.Spec.VersionName != "" {
			createInput.VersionName = aws.String(obj.Spec.VersionName)
		}
		if len(tags) > 0 {
			createInput.Tags = tags
		}
		out, err := r.SSMClient.CreateDocument(ctx, createInput)
		if err != nil {
			return fmt.Errorf("create ssm document: %w", err)
		}
		if out.DocumentDescription != nil {
			obj.Status.DocumentVersion = aws.ToString(out.DocumentDescription.DocumentVersion)
			obj.Status.Status = string(out.DocumentDescription.Status)
		}
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionSD(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "SSMDocument reconciled")
}

func (r *SSMDocumentReconciler) deleteDocument(ctx context.Context, obj *awsv1alpha1.SSMDocument) error {
	_, err := r.SSMClient.DeleteDocument(ctx, &awsssm.DeleteDocumentInput{
		Name: aws.String(obj.Spec.Name),
	})
	if ssmhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SSMDocumentReconciler) setConditionSD(ctx context.Context, obj *awsv1alpha1.SSMDocument, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *SSMDocumentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.SSMDocument{}).
		Complete(r)
}
