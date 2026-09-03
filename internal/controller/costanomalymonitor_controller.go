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
	awsce "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	cehelper "github.com/konfig-io/konfig-konector/internal/aws/costexplorer"
)

// CostAnomalyMonitorAWSAPI is the subset of the Cost Explorer API used by this controller.
type CostAnomalyMonitorAWSAPI interface {
	GetAnomalyMonitors(ctx context.Context, params *awsce.GetAnomalyMonitorsInput, optFns ...func(*awsce.Options)) (*awsce.GetAnomalyMonitorsOutput, error)
	CreateAnomalyMonitor(ctx context.Context, params *awsce.CreateAnomalyMonitorInput, optFns ...func(*awsce.Options)) (*awsce.CreateAnomalyMonitorOutput, error)
	UpdateAnomalyMonitor(ctx context.Context, params *awsce.UpdateAnomalyMonitorInput, optFns ...func(*awsce.Options)) (*awsce.UpdateAnomalyMonitorOutput, error)
	DeleteAnomalyMonitor(ctx context.Context, params *awsce.DeleteAnomalyMonitorInput, optFns ...func(*awsce.Options)) (*awsce.DeleteAnomalyMonitorOutput, error)
}

// CostAnomalyMonitorReconciler reconciles CostAnomalyMonitor objects.
type CostAnomalyMonitorReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	CostExplorerClient CostAnomalyMonitorAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=costanomalymonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=costanomalymonitors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=costanomalymonitors/finalizers,verbs=update

func (r *CostAnomalyMonitorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	m := &awsv1alpha1.CostAnomalyMonitor{}
	if err := r.Get(ctx, req.NamespacedName, m); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !m.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(m, awsv1alpha1.FinalizerName) {
			if shouldAbandon(m) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(m, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, m)
			}
			if err := r.deleteMonitor(ctx, m); err != nil {
				logger.Error(err, "failed to delete cost anomaly monitor")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(m, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, m)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(m, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(m, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, m); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileMonitor(ctx, m); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, m, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

// findMonitorByName scans all anomaly monitors for one with the spec name.
func (r *CostAnomalyMonitorReconciler) findMonitorByName(ctx context.Context, name string) (*cetypes.AnomalyMonitor, error) {
	var next *string
	for {
		out, err := r.CostExplorerClient.GetAnomalyMonitors(ctx, &awsce.GetAnomalyMonitorsInput{NextPageToken: next})
		if err != nil {
			return nil, err
		}
		for i := range out.AnomalyMonitors {
			if aws.ToString(out.AnomalyMonitors[i].MonitorName) == name {
				return &out.AnomalyMonitors[i], nil
			}
		}
		if out.NextPageToken == nil {
			return nil, nil
		}
		next = out.NextPageToken
	}
}

func (r *CostAnomalyMonitorReconciler) reconcileMonitor(ctx context.Context, m *awsv1alpha1.CostAnomalyMonitor) error {
	var existing *cetypes.AnomalyMonitor
	if m.Status.ARN != "" {
		out, err := r.CostExplorerClient.GetAnomalyMonitors(ctx, &awsce.GetAnomalyMonitorsInput{
			MonitorArnList: []string{m.Status.ARN},
		})
		if err != nil && !cehelper.IsNotFound(err) {
			return err
		}
		if err == nil && len(out.AnomalyMonitors) > 0 {
			existing = &out.AnomalyMonitors[0]
		}
	} else {
		found, err := r.findMonitorByName(ctx, m.Spec.MonitorName)
		if err != nil {
			return err
		}
		existing = found
	}

	if existing == nil {
		monitor := &cetypes.AnomalyMonitor{
			MonitorName: aws.String(m.Spec.MonitorName),
			MonitorType: cetypes.MonitorType(m.Spec.MonitorType),
		}
		if m.Spec.MonitorDimension != "" {
			monitor.MonitorDimension = cetypes.MonitorDimension(m.Spec.MonitorDimension)
		}
		created, err := r.CostExplorerClient.CreateAnomalyMonitor(ctx, &awsce.CreateAnomalyMonitorInput{
			AnomalyMonitor: monitor,
		})
		if err != nil {
			return fmt.Errorf("create cost anomaly monitor: %w", err)
		}
		m.Status.ARN = aws.ToString(created.MonitorArn)
		// Persist the ARN immediately: the AWS resource now exists, and losing
		// the identifier would orphan it on delete.
		if err := persistStatus(ctx, r.Client, m); err != nil {
			return fmt.Errorf("persist monitor ARN after create: %w", err)
		}
	} else {
		m.Status.ARN = aws.ToString(existing.MonitorArn)
		if m.Status.ObservedGeneration != m.Generation && aws.ToString(existing.MonitorName) != m.Spec.MonitorName {
			if _, err := r.CostExplorerClient.UpdateAnomalyMonitor(ctx, &awsce.UpdateAnomalyMonitorInput{
				MonitorArn:  existing.MonitorArn,
				MonitorName: aws.String(m.Spec.MonitorName),
			}); err != nil {
				return fmt.Errorf("update cost anomaly monitor: %w", err)
			}
		}
	}

	m.Status.ObservedGeneration = m.Generation
	now := metav1.Now()
	m.Status.LastSyncTime = &now
	return r.setCondition(ctx, m, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "cost anomaly monitor reconciled")
}

func (r *CostAnomalyMonitorReconciler) deleteMonitor(ctx context.Context, m *awsv1alpha1.CostAnomalyMonitor) error {
	arn := m.Status.ARN
	if arn == "" {
		// Status may have been lost before it was persisted; fall back to a
		// deterministic name lookup.
		found, err := r.findMonitorByName(ctx, m.Spec.MonitorName)
		if err != nil {
			return err
		}
		if found == nil {
			return nil
		}
		arn = aws.ToString(found.MonitorArn)
	}
	_, err := r.CostExplorerClient.DeleteAnomalyMonitor(ctx, &awsce.DeleteAnomalyMonitorInput{
		MonitorArn: aws.String(arn),
	})
	if cehelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *CostAnomalyMonitorReconciler) setCondition(ctx context.Context, m *awsv1alpha1.CostAnomalyMonitor, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&m.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: m.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, m); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *CostAnomalyMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CostAnomalyMonitor{}).
		Complete(r)
}
