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
	awsct "github.com/aws/aws-sdk-go-v2/service/controltower"
	cttypes "github.com/aws/aws-sdk-go-v2/service/controltower/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	cthelper "github.com/konfig-io/konfig-konector/internal/aws/controltower"
)

// CTEnabledControlAWSAPI is the subset of the Control Tower API used by this controller.
type CTEnabledControlAWSAPI interface {
	EnableControl(ctx context.Context, params *awsct.EnableControlInput, optFns ...func(*awsct.Options)) (*awsct.EnableControlOutput, error)
	DisableControl(ctx context.Context, params *awsct.DisableControlInput, optFns ...func(*awsct.Options)) (*awsct.DisableControlOutput, error)
	GetControlOperation(ctx context.Context, params *awsct.GetControlOperationInput, optFns ...func(*awsct.Options)) (*awsct.GetControlOperationOutput, error)
	ListEnabledControls(ctx context.Context, params *awsct.ListEnabledControlsInput, optFns ...func(*awsct.Options)) (*awsct.ListEnabledControlsOutput, error)
}

// CTEnabledControlReconciler reconciles CTEnabledControl objects. Enable and
// disable are asynchronous: EnableControl/DisableControl return an operation
// identifier polled via GetControlOperation; overall state is confirmed via
// ListEnabledControls.
type CTEnabledControlReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	ControlTowerClient CTEnabledControlAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=ctenabledcontrols,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ctenabledcontrols/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ctenabledcontrols/finalizers,verbs=update

func (r *CTEnabledControlReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ec := &awsv1alpha1.CTEnabledControl{}
	if err := r.Get(ctx, req.NamespacedName, ec); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, ec); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !ec.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ec, awsv1alpha1.FinalizerName) {
			if shouldAbandon(ec) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(ec, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, ec)
			}
			if err := r.disableControl(ctx, ec); err != nil {
				logger.Error(err, "failed to disable control")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(ec, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, ec)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(ec, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(ec, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, ec); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	result, err := r.reconcileControl(ctx, ec)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, ec, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

// findEnabledControl looks up the enabled control on the target OU via
// ListEnabledControls and returns its summary if present.
func (r *CTEnabledControlReconciler) findEnabledControl(ctx context.Context, ec *awsv1alpha1.CTEnabledControl) (*cttypes.EnabledControlSummary, error) {
	var next *string
	for {
		out, err := r.ControlTowerClient.ListEnabledControls(ctx, &awsct.ListEnabledControlsInput{
			TargetIdentifier: aws.String(ec.Spec.TargetIdentifier),
			NextToken:        next,
		})
		if err != nil {
			if cthelper.IsNotFound(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("list enabled controls: %w", err)
		}
		for i := range out.EnabledControls {
			if aws.ToString(out.EnabledControls[i].ControlIdentifier) == ec.Spec.ControlIdentifier {
				return &out.EnabledControls[i], nil
			}
		}
		if out.NextToken == nil {
			return nil, nil
		}
		next = out.NextToken
	}
}

func (r *CTEnabledControlReconciler) reconcileControl(ctx context.Context, ec *awsv1alpha1.CTEnabledControl) (ctrl.Result, error) {
	// Poll an in-flight enable operation first.
	if ec.Status.OperationIdentifier != "" {
		out, err := r.ControlTowerClient.GetControlOperation(ctx, &awsct.GetControlOperationInput{
			OperationIdentifier: aws.String(ec.Status.OperationIdentifier),
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("get control operation: %w", err)
		}
		op := out.ControlOperation
		ec.Status.State = string(op.Status)
		switch op.Status {
		case cttypes.ControlOperationStatusInProgress:
			if err := r.setCondition(ctx, ec, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "control enablement in progress"); err != nil {
				return ctrl.Result{}, err
			}
			return requeueOrgPolling, nil
		case cttypes.ControlOperationStatusFailed:
			msg := aws.ToString(op.StatusMessage)
			ec.Status.OperationIdentifier = ""
			_ = persistStatus(ctx, r.Client, ec)
			return ctrl.Result{}, fmt.Errorf("control operation failed: %s", msg)
		case cttypes.ControlOperationStatusSucceeded:
			ec.Status.OperationIdentifier = ""
			if err := persistStatus(ctx, r.Client, ec); err != nil {
				return ctrl.Result{}, fmt.Errorf("persist operation completion: %w", err)
			}
		}
	}

	summary, err := r.findEnabledControl(ctx, ec)
	if err != nil {
		return ctrl.Result{}, err
	}

	if summary == nil {
		out, err := r.ControlTowerClient.EnableControl(ctx, &awsct.EnableControlInput{
			ControlIdentifier: aws.String(ec.Spec.ControlIdentifier),
			TargetIdentifier:  aws.String(ec.Spec.TargetIdentifier),
			Tags:              ec.Spec.Tags,
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("enable control: %w", err)
		}
		// Persist the operation identifier (and ARN when returned)
		// immediately after the enable call.
		ec.Status.OperationIdentifier = aws.ToString(out.OperationIdentifier)
		ec.Status.ARN = aws.ToString(out.Arn)
		ec.Status.State = string(cttypes.ControlOperationStatusInProgress)
		if err := persistStatus(ctx, r.Client, ec); err != nil {
			return ctrl.Result{}, fmt.Errorf("persist operation identifier after enable: %w", err)
		}
		if err := r.setCondition(ctx, ec, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "control enablement in progress"); err != nil {
			return ctrl.Result{}, err
		}
		return requeueOrgPolling, nil
	}

	ec.Status.ARN = aws.ToString(summary.Arn)
	if summary.StatusSummary != nil {
		ec.Status.State = string(summary.StatusSummary.Status)
	}
	ec.Status.ObservedGeneration = ec.Generation
	now := metav1.Now()
	ec.Status.LastSyncTime = &now
	if err := r.setCondition(ctx, ec, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "enabled control reconciled"); err != nil {
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *CTEnabledControlReconciler) disableControl(ctx context.Context, ec *awsv1alpha1.CTEnabledControl) error {
	// DisableControl is keyed by control+target identifiers from the spec, so
	// no stored identifier is needed. The disable operation continues
	// server-side after the CR is gone; no need to poll before removing the
	// finalizer.
	_, err := r.ControlTowerClient.DisableControl(ctx, &awsct.DisableControlInput{
		ControlIdentifier: aws.String(ec.Spec.ControlIdentifier),
		TargetIdentifier:  aws.String(ec.Spec.TargetIdentifier),
	})
	if cthelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *CTEnabledControlReconciler) setCondition(ctx context.Context, ec *awsv1alpha1.CTEnabledControl, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&ec.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: ec.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, ec); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *CTEnabledControlReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CTEnabledControl{}).
		Complete(r)
}
