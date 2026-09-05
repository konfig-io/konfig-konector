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
	awssecurityhub "github.com/aws/aws-sdk-go-v2/service/securityhub"
	securityhubtypes "github.com/aws/aws-sdk-go-v2/service/securityhub/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	securityhubhelper "github.com/konfig-io/konfig-konector/internal/aws/securityhub"
)

// SecurityHubAccountAWSAPI is the subset of the Security Hub API used by this controller.
type SecurityHubAccountAWSAPI interface {
	DescribeHub(ctx context.Context, params *awssecurityhub.DescribeHubInput, optFns ...func(*awssecurityhub.Options)) (*awssecurityhub.DescribeHubOutput, error)
	EnableSecurityHub(ctx context.Context, params *awssecurityhub.EnableSecurityHubInput, optFns ...func(*awssecurityhub.Options)) (*awssecurityhub.EnableSecurityHubOutput, error)
	DisableSecurityHub(ctx context.Context, params *awssecurityhub.DisableSecurityHubInput, optFns ...func(*awssecurityhub.Options)) (*awssecurityhub.DisableSecurityHubOutput, error)
	UpdateSecurityHubConfiguration(ctx context.Context, params *awssecurityhub.UpdateSecurityHubConfigurationInput, optFns ...func(*awssecurityhub.Options)) (*awssecurityhub.UpdateSecurityHubConfigurationOutput, error)
}

// SecurityHubAccountReconciler reconciles SecurityHubAccount objects.
type SecurityHubAccountReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	SecurityHubClient SecurityHubAccountAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=securityhubaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=securityhubaccounts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=securityhubaccounts/finalizers,verbs=update

func (r *SecurityHubAccountReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	hub := &awsv1alpha1.SecurityHubAccount{}
	if err := r.Get(ctx, req.NamespacedName, hub); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, hub); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !hub.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(hub, awsv1alpha1.FinalizerName) {
			if shouldAbandon(hub) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(hub, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, hub)
			}
			if err := r.disableHub(ctx); err != nil {
				logger.Error(err, "failed to disable Security Hub")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(hub, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, hub)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(hub, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(hub, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, hub); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileHub(ctx, hub); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, hub, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *SecurityHubAccountReconciler) reconcileHub(ctx context.Context, hub *awsv1alpha1.SecurityHubAccount) error {
	descOut, err := r.SecurityHubClient.DescribeHub(ctx, &awssecurityhub.DescribeHubInput{})
	if securityhubhelper.IsNotFound(err) {
		if _, err := r.SecurityHubClient.EnableSecurityHub(ctx, &awssecurityhub.EnableSecurityHubInput{
			EnableDefaultStandards:  hub.Spec.EnableDefaultStandards,
			ControlFindingGenerator: securityhubtypes.ControlFindingGenerator(hub.Spec.ControlFindingGenerator),
			Tags:                    hub.Spec.Tags,
		}); err != nil {
			return fmt.Errorf("enable Security Hub: %w", err)
		}
		descOut, err = r.SecurityHubClient.DescribeHub(ctx, &awssecurityhub.DescribeHubInput{})
		if err != nil {
			return fmt.Errorf("describe hub after enable: %w", err)
		}
		// Persist the hub ARN immediately after enable.
		hub.Status.HubARN = aws.ToString(descOut.HubArn)
		if err := persistStatus(ctx, r.Client, hub); err != nil {
			return fmt.Errorf("persist hub ARN after enable: %w", err)
		}
	} else if err != nil {
		return err
	} else if hub.Status.ObservedGeneration != hub.Generation && hub.Spec.ControlFindingGenerator != "" &&
		string(descOut.ControlFindingGenerator) != hub.Spec.ControlFindingGenerator {
		if _, err := r.SecurityHubClient.UpdateSecurityHubConfiguration(ctx, &awssecurityhub.UpdateSecurityHubConfigurationInput{
			ControlFindingGenerator: securityhubtypes.ControlFindingGenerator(hub.Spec.ControlFindingGenerator),
		}); err != nil {
			return fmt.Errorf("update Security Hub configuration: %w", err)
		}
	}

	hub.Status.HubARN = aws.ToString(descOut.HubArn)
	hub.Status.SubscribedAt = aws.ToString(descOut.SubscribedAt)
	hub.Status.ObservedGeneration = hub.Generation
	now := metav1.Now()
	hub.Status.LastSyncTime = &now
	return r.setCondition(ctx, hub, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Security Hub reconciled")
}

func (r *SecurityHubAccountReconciler) disableHub(ctx context.Context) error {
	// Security Hub is a singleton per account/region; disabling needs no identifier.
	_, err := r.SecurityHubClient.DisableSecurityHub(ctx, &awssecurityhub.DisableSecurityHubInput{})
	if securityhubhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SecurityHubAccountReconciler) setCondition(ctx context.Context, hub *awsv1alpha1.SecurityHubAccount, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&hub.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: hub.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, hub); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *SecurityHubAccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.SecurityHubAccount{}).
		Complete(r)
}
