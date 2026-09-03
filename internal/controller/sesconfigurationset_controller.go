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
	awssesv2 "github.com/aws/aws-sdk-go-v2/service/sesv2"
	sestypes "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	seshelper "github.com/konfig-io/konfig-konector/internal/aws/sesv2"
)

// SESConfigurationSetReconciler reconciles SESConfigurationSet objects.
type SESConfigurationSetReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	SESv2Client *awssesv2.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=sesconfigurationsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=sesconfigurationsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=sesconfigurationsets/finalizers,verbs=update

func (r *SESConfigurationSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.SESConfigurationSet{}
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
			if err := r.deleteConfigurationSet(ctx, obj); err != nil {
				logger.Error(err, "failed to delete SESConfigurationSet")
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

	if err := r.reconcileConfigurationSet(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionSCS(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *SESConfigurationSetReconciler) reconcileConfigurationSet(ctx context.Context, obj *awsv1alpha1.SESConfigurationSet) error {
	_, err := r.SESv2Client.GetConfigurationSet(ctx, &awssesv2.GetConfigurationSetInput{
		ConfigurationSetName: aws.String(obj.Spec.ConfigurationSetName),
	})
	if err != nil && !seshelper.IsNotFound(err) {
		return fmt.Errorf("get ses configuration set: %w", err)
	}

	if err == nil {
		// Update sending options if specified.
		if obj.Spec.SendingEnabled != nil {
			_, err := r.SESv2Client.PutConfigurationSetSendingOptions(ctx, &awssesv2.PutConfigurationSetSendingOptionsInput{
				ConfigurationSetName: aws.String(obj.Spec.ConfigurationSetName),
				SendingEnabled:       *obj.Spec.SendingEnabled,
			})
			if err != nil {
				return fmt.Errorf("put ses configuration set sending options: %w", err)
			}
		}
		if obj.Spec.ReputationMetricsEnabled != nil {
			_, err := r.SESv2Client.PutConfigurationSetReputationOptions(ctx, &awssesv2.PutConfigurationSetReputationOptionsInput{
				ConfigurationSetName:     aws.String(obj.Spec.ConfigurationSetName),
				ReputationMetricsEnabled: *obj.Spec.ReputationMetricsEnabled,
			})
			if err != nil {
				return fmt.Errorf("put ses configuration set reputation options: %w", err)
			}
		}
	} else {
		tags := make([]sestypes.Tag, 0, len(obj.Spec.Tags))
		for k, v := range obj.Spec.Tags {
			k, v := k, v
			tags = append(tags, sestypes.Tag{Key: &k, Value: &v})
		}

		createInput := &awssesv2.CreateConfigurationSetInput{
			ConfigurationSetName: aws.String(obj.Spec.ConfigurationSetName),
		}
		if obj.Spec.SendingEnabled != nil {
			createInput.SendingOptions = &sestypes.SendingOptions{
				SendingEnabled: *obj.Spec.SendingEnabled,
			}
		}
		if obj.Spec.ReputationMetricsEnabled != nil {
			createInput.ReputationOptions = &sestypes.ReputationOptions{
				ReputationMetricsEnabled: *obj.Spec.ReputationMetricsEnabled,
			}
		}
		if len(tags) > 0 {
			createInput.Tags = tags
		}
		_, err := r.SESv2Client.CreateConfigurationSet(ctx, createInput)
		if err != nil {
			return fmt.Errorf("create ses configuration set: %w", err)
		}
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionSCS(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "SESConfigurationSet reconciled")
}

func (r *SESConfigurationSetReconciler) deleteConfigurationSet(ctx context.Context, obj *awsv1alpha1.SESConfigurationSet) error {
	_, err := r.SESv2Client.DeleteConfigurationSet(ctx, &awssesv2.DeleteConfigurationSetInput{
		ConfigurationSetName: aws.String(obj.Spec.ConfigurationSetName),
	})
	if seshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SESConfigurationSetReconciler) setConditionSCS(ctx context.Context, obj *awsv1alpha1.SESConfigurationSet, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *SESConfigurationSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.SESConfigurationSet{}).
		Complete(r)
}
