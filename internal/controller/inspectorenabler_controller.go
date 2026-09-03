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
	awsinspector2 "github.com/aws/aws-sdk-go-v2/service/inspector2"
	inspector2types "github.com/aws/aws-sdk-go-v2/service/inspector2/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	inspector2helper "github.com/konfig-io/konfig-konector/internal/aws/inspector2"
)

// InspectorEnablerAWSAPI is the subset of the Inspector2 API used by this controller.
type InspectorEnablerAWSAPI interface {
	BatchGetAccountStatus(ctx context.Context, params *awsinspector2.BatchGetAccountStatusInput, optFns ...func(*awsinspector2.Options)) (*awsinspector2.BatchGetAccountStatusOutput, error)
	Enable(ctx context.Context, params *awsinspector2.EnableInput, optFns ...func(*awsinspector2.Options)) (*awsinspector2.EnableOutput, error)
	Disable(ctx context.Context, params *awsinspector2.DisableInput, optFns ...func(*awsinspector2.Options)) (*awsinspector2.DisableOutput, error)
}

// InspectorEnablerReconciler reconciles InspectorEnabler objects.
type InspectorEnablerReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	InspectorClient InspectorEnablerAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=inspectorenablers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=inspectorenablers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=inspectorenablers/finalizers,verbs=update

func (r *InspectorEnablerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ins := &awsv1alpha1.InspectorEnabler{}
	if err := r.Get(ctx, req.NamespacedName, ins); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !ins.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ins, awsv1alpha1.FinalizerName) {
			if shouldAbandon(ins) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(ins, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, ins)
			}
			if err := r.disableInspector(ctx, ins); err != nil {
				logger.Error(err, "failed to disable Inspector")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(ins, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, ins)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(ins, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(ins, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, ins); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileInspector(ctx, ins); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, ins, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *InspectorEnablerReconciler) reconcileInspector(ctx context.Context, ins *awsv1alpha1.InspectorEnabler) error {
	statusOut, err := r.InspectorClient.BatchGetAccountStatus(ctx, &awsinspector2.BatchGetAccountStatusInput{})
	if err != nil {
		return fmt.Errorf("get account status: %w", err)
	}
	if len(statusOut.Accounts) == 0 {
		return fmt.Errorf("BatchGetAccountStatus returned no accounts")
	}
	account := statusOut.Accounts[0]

	// Determine which of the desired scan types are not yet enabled.
	var missing []inspector2types.ResourceScanType
	for _, rt := range ins.Spec.ResourceTypes {
		scanType := inspector2types.ResourceScanType(string(rt))
		if inspectorScanStatus(account.ResourceState, scanType) != inspector2types.StatusEnabled {
			missing = append(missing, scanType)
		}
	}

	if len(missing) > 0 {
		if _, err := r.InspectorClient.Enable(ctx, &awsinspector2.EnableInput{
			ResourceTypes: missing,
		}); err != nil {
			return fmt.Errorf("enable Inspector: %w", err)
		}
		// Re-read the account status after enabling.
		statusOut, err = r.InspectorClient.BatchGetAccountStatus(ctx, &awsinspector2.BatchGetAccountStatusInput{})
		if err != nil {
			return fmt.Errorf("get account status after enable: %w", err)
		}
		if len(statusOut.Accounts) > 0 {
			account = statusOut.Accounts[0]
		}
	}

	ins.Status.AccountID = aws.ToString(account.AccountId)
	if account.State != nil {
		ins.Status.AccountStatus = string(account.State.Status)
	}
	if rs := account.ResourceState; rs != nil {
		ins.Status.ResourceStatus = &awsv1alpha1.InspectorResourceStatus{
			EC2:        string(inspectorScanStatus(rs, inspector2types.ResourceScanTypeEc2)),
			ECR:        string(inspectorScanStatus(rs, inspector2types.ResourceScanTypeEcr)),
			Lambda:     string(inspectorScanStatus(rs, inspector2types.ResourceScanTypeLambda)),
			LambdaCode: string(inspectorScanStatus(rs, inspector2types.ResourceScanTypeLambdaCode)),
		}
	}
	ins.Status.ObservedGeneration = ins.Generation
	now := metav1.Now()
	ins.Status.LastSyncTime = &now
	return r.setCondition(ctx, ins, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Inspector activation reconciled")
}

func (r *InspectorEnablerReconciler) disableInspector(ctx context.Context, ins *awsv1alpha1.InspectorEnabler) error {
	// Inspector activation is a singleton per account/region; disable the
	// scan types this CR manages.
	types := make([]inspector2types.ResourceScanType, 0, len(ins.Spec.ResourceTypes))
	for _, rt := range ins.Spec.ResourceTypes {
		types = append(types, inspector2types.ResourceScanType(string(rt)))
	}
	_, err := r.InspectorClient.Disable(ctx, &awsinspector2.DisableInput{
		ResourceTypes: types,
	})
	if inspector2helper.IsNotFound(err) {
		return nil
	}
	return err
}

// inspectorScanStatus extracts a scan type's status from a ResourceState.
func inspectorScanStatus(rs *inspector2types.ResourceState, scanType inspector2types.ResourceScanType) inspector2types.Status {
	if rs == nil {
		return ""
	}
	var st *inspector2types.State
	switch scanType {
	case inspector2types.ResourceScanTypeEc2:
		st = rs.Ec2
	case inspector2types.ResourceScanTypeEcr:
		st = rs.Ecr
	case inspector2types.ResourceScanTypeLambda:
		st = rs.Lambda
	case inspector2types.ResourceScanTypeLambdaCode:
		st = rs.LambdaCode
	}
	if st == nil {
		return ""
	}
	return st.Status
}

func (r *InspectorEnablerReconciler) setCondition(ctx context.Context, ins *awsv1alpha1.InspectorEnabler, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&ins.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: ins.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, ins); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *InspectorEnablerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.InspectorEnabler{}).
		Complete(r)
}
