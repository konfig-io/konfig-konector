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
	awskafka "github.com/aws/aws-sdk-go-v2/service/kafka"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	kafkahelper "github.com/konfig-io/konfig-konector/internal/aws/kafka"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// MSKConfigurationReconciler reconciles MSKConfiguration objects.
type MSKConfigurationReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	KafkaClient *multi.Kafka
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=mskconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=mskconfigurations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=mskconfigurations/finalizers,verbs=update

func (r *MSKConfigurationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.MSKConfiguration{}
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
			if err := r.deleteConfiguration(ctx, obj); err != nil {
				logger.Error(err, "failed to delete MSKConfiguration")
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

	if err := r.reconcileConfiguration(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionMC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *MSKConfigurationReconciler) reconcileConfiguration(ctx context.Context, obj *awsv1alpha1.MSKConfiguration) error {
	serverProps := []byte(obj.Spec.ServerProperties)

	if obj.Status.ConfigurationARN != "" {
		descOut, err := r.KafkaClient.DescribeConfiguration(ctx, &awskafka.DescribeConfigurationInput{
			Arn: aws.String(obj.Status.ConfigurationARN),
		})
		if err != nil && !kafkahelper.IsNotFound(err) {
			return fmt.Errorf("describe msk configuration: %w", err)
		}
		if err == nil && descOut.Arn != nil {
			updateOut, err := r.KafkaClient.UpdateConfiguration(ctx, &awskafka.UpdateConfigurationInput{
				Arn:              aws.String(obj.Status.ConfigurationARN),
				ServerProperties: serverProps,
			})
			if err != nil {
				return fmt.Errorf("update msk configuration: %w", err)
			}
			if updateOut.LatestRevision != nil {
				obj.Status.LatestRevision = aws.ToInt64(updateOut.LatestRevision.Revision)
			}
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionMC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "MSKConfiguration reconciled")
		}
		obj.Status.ConfigurationARN = ""
	}

	input := &awskafka.CreateConfigurationInput{
		Name:             aws.String(obj.Spec.Name),
		ServerProperties: serverProps,
	}
	if obj.Spec.Description != "" {
		input.Description = aws.String(obj.Spec.Description)
	}
	if len(obj.Spec.KafkaVersions) > 0 {
		input.KafkaVersions = obj.Spec.KafkaVersions
	}

	out, err := r.KafkaClient.CreateConfiguration(ctx, input)
	if err != nil {
		return fmt.Errorf("create msk configuration: %w", err)
	}

	obj.Status.ConfigurationARN = aws.ToString(out.Arn)
	if out.LatestRevision != nil {
		obj.Status.LatestRevision = aws.ToInt64(out.LatestRevision.Revision)
	}
	// The AWS resource now exists; losing the ARN would orphan it and a
	// retried create-by-name would conflict.
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist status after create: %w", err)
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionMC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "MSKConfiguration created")
}

func (r *MSKConfigurationReconciler) deleteConfiguration(ctx context.Context, obj *awsv1alpha1.MSKConfiguration) error {
	if obj.Status.ConfigurationARN == "" {
		// Status may have been lost after a successful create; the create
		// path always uses spec.name, so look it up before giving up.
		paginator := awskafka.NewListConfigurationsPaginator(r.KafkaClient, &awskafka.ListConfigurationsInput{})
		for paginator.HasMorePages() && obj.Status.ConfigurationARN == "" {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				return fmt.Errorf("list msk configurations: %w", err)
			}
			for _, c := range page.Configurations {
				if aws.ToString(c.Name) == obj.Spec.Name {
					obj.Status.ConfigurationARN = aws.ToString(c.Arn)
					break
				}
			}
		}
		if obj.Status.ConfigurationARN == "" {
			return nil
		}
	}
	_, err := r.KafkaClient.DeleteConfiguration(ctx, &awskafka.DeleteConfigurationInput{
		Arn: aws.String(obj.Status.ConfigurationARN),
	})
	if kafkahelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *MSKConfigurationReconciler) setConditionMC(ctx context.Context, obj *awsv1alpha1.MSKConfiguration, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *MSKConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.MSKConfiguration{}).
		Complete(r)
}
