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
	awscc "github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	cctypes "github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	cchelper "github.com/konfig-io/konfig-konector/internal/aws/cloudcontrol"
)

// CloudControlAWSAPI is the subset of the Cloud Control API used by this controller.
type CloudControlAWSAPI interface {
	CreateResource(ctx context.Context, params *awscc.CreateResourceInput, optFns ...func(*awscc.Options)) (*awscc.CreateResourceOutput, error)
	GetResource(ctx context.Context, params *awscc.GetResourceInput, optFns ...func(*awscc.Options)) (*awscc.GetResourceOutput, error)
	UpdateResource(ctx context.Context, params *awscc.UpdateResourceInput, optFns ...func(*awscc.Options)) (*awscc.UpdateResourceOutput, error)
	DeleteResource(ctx context.Context, params *awscc.DeleteResourceInput, optFns ...func(*awscc.Options)) (*awscc.DeleteResourceOutput, error)
	GetResourceRequestStatus(ctx context.Context, params *awscc.GetResourceRequestStatusInput, optFns ...func(*awscc.Options)) (*awscc.GetResourceRequestStatusOutput, error)
}

// CloudControlResourceReconciler drives any Cloud Control resource type
// through create → poll → drift-correct → delete.
type CloudControlResourceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	CCClient CloudControlAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudcontrolresources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudcontrolresources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudcontrolresources/finalizers,verbs=update

func (r *CloudControlResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	obj := &awsv1alpha1.CloudControlResource{}
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
			done, err := r.deleteResource(ctx, obj)
			if err != nil {
				logger.Error(err, "failed to delete Cloud Control resource")
				return ctrl.Result{}, err
			}
			if !done {
				return requeuePending, nil
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

	if err := r.reconcileResource(ctx, obj); err != nil {
		if errors.Is(err, errPendingAcceptance) {
			return requeuePending, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

// pollRequest advances an in-flight operation. It returns true when the
// operation finished successfully, errPendingAcceptance while it runs, and a
// descriptive error when it failed.
func (r *CloudControlResourceReconciler) pollRequest(ctx context.Context, obj *awsv1alpha1.CloudControlResource) (bool, error) {
	if obj.Status.RequestToken == "" {
		return true, nil
	}
	out, err := r.CCClient.GetResourceRequestStatus(ctx, &awscc.GetResourceRequestStatusInput{RequestToken: aws.String(obj.Status.RequestToken)})
	if err != nil {
		if cchelper.IsNotFound(err) {
			// Token expired (requests are retained ~7 days); fall back to the live resource.
			obj.Status.RequestToken, obj.Status.Operation = "", ""
			return true, persistStatus(ctx, r.Client, obj)
		}
		return false, fmt.Errorf("get resource request status: %w", err)
	}
	ev := out.ProgressEvent
	obj.Status.OperationStatus = string(ev.OperationStatus)
	if id := aws.ToString(ev.Identifier); id != "" {
		obj.Status.Identifier = id
	}
	switch ev.OperationStatus {
	case cctypes.OperationStatusSuccess:
		obj.Status.RequestToken, obj.Status.Operation = "", ""
		return true, persistStatus(ctx, r.Client, obj)
	case cctypes.OperationStatusFailed, cctypes.OperationStatusCancelComplete:
		if obj.Status.Operation == "DELETE" && ev.ErrorCode == cctypes.HandlerErrorCodeNotFound {
			// The resource (or its parent) is already gone: deletion is complete.
			obj.Status.RequestToken, obj.Status.Operation = "", ""
			return true, persistStatus(ctx, r.Client, obj)
		}
		msg := fmt.Sprintf("%s %s: %s (%s)", obj.Status.Operation, ev.OperationStatus, aws.ToString(ev.StatusMessage), ev.ErrorCode)
		obj.Status.RequestToken, obj.Status.Operation = "", ""
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return false, err
		}
		return false, errors.New(msg)
	default:
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return false, err
		}
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonCreated,
			fmt.Sprintf("%s %s", obj.Status.Operation, ev.OperationStatus))
		return false, errPendingAcceptance
	}
}

func (r *CloudControlResourceReconciler) reconcileResource(ctx context.Context, obj *awsv1alpha1.CloudControlResource) error {
	if done, err := r.pollRequest(ctx, obj); err != nil || !done {
		return err
	}
	if obj.Status.Identifier == "" && obj.Spec.Identifier != "" {
		obj.Status.Identifier = obj.Spec.Identifier
	}
	desired := obj.Spec.DesiredState.Raw

	// Live state.
	var live *awscc.GetResourceOutput
	if obj.Status.Identifier != "" {
		out, err := r.CCClient.GetResource(ctx, &awscc.GetResourceInput{
			TypeName: aws.String(obj.Spec.TypeName), Identifier: aws.String(obj.Status.Identifier),
			RoleArn: optString(obj.Spec.RoleARN), TypeVersionId: optString(obj.Spec.TypeVersionID),
		})
		if err != nil && !cchelper.IsNotFound(err) {
			return fmt.Errorf("get resource: %w", err)
		}
		if err == nil {
			live = out
		} else {
			obj.Status.Identifier = ""
		}
	}

	if live == nil {
		out, err := r.CCClient.CreateResource(ctx, &awscc.CreateResourceInput{
			TypeName:      aws.String(obj.Spec.TypeName),
			DesiredState:  aws.String(string(desired)),
			ClientToken:   aws.String(clientToken(obj)),
			RoleArn:       optString(obj.Spec.RoleARN),
			TypeVersionId: optString(obj.Spec.TypeVersionID),
		})
		if err != nil {
			return fmt.Errorf("create resource: %w", err)
		}
		return r.trackOperation(ctx, obj, "CREATE", out.ProgressEvent)
	}

	// Drift correction / spec change: patch every desired property that differs.
	liveJSON := aws.ToString(live.ResourceDescription.Properties)
	patch, err := cchelper.BuildPatch([]byte(liveJSON), desired, nil)
	if err != nil {
		return err
	}
	if patch != nil {
		out, err := r.CCClient.UpdateResource(ctx, &awscc.UpdateResourceInput{
			TypeName: aws.String(obj.Spec.TypeName), Identifier: aws.String(obj.Status.Identifier),
			PatchDocument: aws.String(string(patch)), ClientToken: aws.String(clientToken(obj)),
			RoleArn: optString(obj.Spec.RoleARN), TypeVersionId: optString(obj.Spec.TypeVersionID),
		})
		if err != nil {
			var nu *cctypes.NotUpdatableException
			if errors.As(err, &nu) {
				return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonUpdateNotSupported,
					"desired state changes a create-only property: "+aws.ToString(nu.Message))
			}
			return fmt.Errorf("update resource: %w", err)
		}
		return r.trackOperation(ctx, obj, "UPDATE", out.ProgressEvent)
	}

	obj.Status.Properties = &apiextensionsv1.JSON{Raw: []byte(liveJSON)}
	obj.Status.OperationStatus = string(cctypes.OperationStatusSuccess)
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, obj.Spec.TypeName+" in sync")
}

func (r *CloudControlResourceReconciler) trackOperation(ctx context.Context, obj *awsv1alpha1.CloudControlResource, op string, ev *cctypes.ProgressEvent) error {
	if ev == nil {
		return fmt.Errorf("%s returned no progress event", op)
	}
	obj.Status.Operation = op
	obj.Status.RequestToken = aws.ToString(ev.RequestToken)
	obj.Status.OperationStatus = string(ev.OperationStatus)
	if id := aws.ToString(ev.Identifier); id != "" {
		obj.Status.Identifier = id
	}
	// Persist the token immediately: it is the only handle on the in-flight
	// resource until the identifier is known.
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist request token after %s: %w", op, err)
	}
	if ev.OperationStatus == cctypes.OperationStatusFailed {
		obj.Status.RequestToken, obj.Status.Operation = "", ""
		return fmt.Errorf("%s failed: %s (%s)", op, aws.ToString(ev.StatusMessage), ev.ErrorCode)
	}
	if ev.OperationStatus == cctypes.OperationStatusSuccess {
		obj.Status.RequestToken, obj.Status.Operation = "", ""
		return persistStatus(ctx, r.Client, obj)
	}
	_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonCreated, op+" in progress")
	return errPendingAcceptance
}

// deleteResource returns done=true once the resource is gone.
func (r *CloudControlResourceReconciler) deleteResource(ctx context.Context, obj *awsv1alpha1.CloudControlResource) (bool, error) {
	if obj.Status.RequestToken != "" && obj.Status.Operation == "DELETE" {
		done, err := r.pollRequest(ctx, obj)
		if errors.Is(err, errPendingAcceptance) {
			return false, nil
		}
		return done, err
	}
	if obj.Status.Identifier == "" {
		return true, nil
	}
	out, err := r.CCClient.DeleteResource(ctx, &awscc.DeleteResourceInput{
		TypeName: aws.String(obj.Spec.TypeName), Identifier: aws.String(obj.Status.Identifier),
		ClientToken: aws.String(clientToken(obj) + "-del"), RoleArn: optString(obj.Spec.RoleARN), TypeVersionId: optString(obj.Spec.TypeVersionID),
	})
	if cchelper.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	obj.Status.Operation = "DELETE"
	obj.Status.RequestToken = aws.ToString(out.ProgressEvent.RequestToken)
	obj.Status.OperationStatus = string(out.ProgressEvent.OperationStatus)
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return false, err
	}
	if out.ProgressEvent.OperationStatus == cctypes.OperationStatusFailed && out.ProgressEvent.ErrorCode == cctypes.HandlerErrorCodeNotFound {
		return true, nil
	}
	return out.ProgressEvent.OperationStatus == cctypes.OperationStatusSuccess, nil
}

// clientToken derives an idempotency token from the object's UID and
// generation so a crashed create is not repeated by the retry.
func clientToken(obj *awsv1alpha1.CloudControlResource) string {
	t := fmt.Sprintf("%s-%d", obj.UID, obj.Generation)
	if len(t) > 64 {
		t = t[len(t)-64:]
	}
	return t
}

func optString(s string) *string {
	if s == "" {
		return nil
	}
	return aws.String(s)
}

func (r *CloudControlResourceReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.CloudControlResource, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
		Type: condType, Status: status, ObservedGeneration: obj.Generation, Reason: reason, Message: message,
	})
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *CloudControlResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CloudControlResource{}).
		Complete(r)
}
