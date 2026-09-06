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
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscc "github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	cctypes "github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	cchelper "github.com/konfig-io/konfig-konector/internal/aws/cloudcontrol"
	"github.com/konfig-io/konfig-konector/internal/cfn"
)

// CloudControlKind describes one generated Cloud Control-backed kind.
type CloudControlKind struct {
	Kind     string
	TypeName string
	New      func() cfn.CloudControlObject
}

// cloudControlKinds is populated by the generated zz_cc_registry.go.
var cloudControlKinds []CloudControlKind

// RegisterCloudControlKinds appends generated kinds to the registry.
func RegisterCloudControlKinds(kinds ...CloudControlKind) {
	cloudControlKinds = append(cloudControlKinds, kinds...)
}

// CloudControlKinds returns the registered generated kinds.
func CloudControlKinds() []CloudControlKind { return cloudControlKinds }

// CloudControlKindReconciler is the shared engine behind every generated
// kind: typed spec → DesiredState, create/poll, drift patch, delete.
type CloudControlKindReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	CCClient CloudControlAWSAPI
	Kind     CloudControlKind
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=*,verbs=get;list;watch;create;update;patch;delete

func (r *CloudControlKindReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	obj := r.Kind.New()
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, obj); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}
	st := obj.CloudControlStatusRef()

	if !obj.GetDeletionTimestamp().IsZero() {
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
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcile(ctx, obj); err != nil {
		if errors.Is(err, errPendingAcceptance) {
			return requeuePending, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, obj, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	_ = st
	return requeueResult(), nil
}

func (r *CloudControlKindReconciler) pollRequest(ctx context.Context, obj cfn.CloudControlObject) (bool, error) {
	st := obj.CloudControlStatusRef()
	if st.RequestToken == "" {
		return true, nil
	}
	out, err := r.CCClient.GetResourceRequestStatus(ctx, &awscc.GetResourceRequestStatusInput{RequestToken: aws.String(st.RequestToken)})
	if err != nil {
		if cchelper.IsNotFound(err) {
			st.RequestToken, st.Operation = "", ""
			return true, persistStatus(ctx, r.Client, obj)
		}
		return false, fmt.Errorf("get resource request status: %w", err)
	}
	ev := out.ProgressEvent
	st.OperationStatus = string(ev.OperationStatus)
	if id := aws.ToString(ev.Identifier); id != "" {
		st.Identifier = id
	}
	switch ev.OperationStatus {
	case cctypes.OperationStatusSuccess:
		st.RequestToken, st.Operation = "", ""
		return true, persistStatus(ctx, r.Client, obj)
	case cctypes.OperationStatusFailed, cctypes.OperationStatusCancelComplete:
		if st.Operation == "DELETE" && ev.ErrorCode == cctypes.HandlerErrorCodeNotFound {
			// The resource (or its parent) is already gone: deletion is complete.
			st.RequestToken, st.Operation = "", ""
			return true, persistStatus(ctx, r.Client, obj)
		}
		msg := fmt.Sprintf("%s %s: %s (%s)", st.Operation, ev.OperationStatus, aws.ToString(ev.StatusMessage), ev.ErrorCode)
		st.RequestToken, st.Operation = "", ""
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return false, err
		}
		return false, errors.New(msg)
	default:
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return false, err
		}
		_ = r.setCondition(ctx, obj, metav1.ConditionFalse, awsv1alpha1.ReasonCreated, fmt.Sprintf("%s %s", st.Operation, ev.OperationStatus))
		return false, errPendingAcceptance
	}
}

func (r *CloudControlKindReconciler) reconcile(ctx context.Context, obj cfn.CloudControlObject) error {
	st := obj.CloudControlStatusRef()
	if done, err := r.pollRequest(ctx, obj); err != nil || !done {
		return err
	}
	desired, err := cfn.ToDesiredState(obj.CloudControlSpec())
	if err != nil {
		return fmt.Errorf("render desired state: %w", err)
	}
	typeName := obj.CloudControlTypeName()

	var live *awscc.GetResourceOutput
	if st.Identifier != "" {
		out, err := r.CCClient.GetResource(ctx, &awscc.GetResourceInput{TypeName: aws.String(typeName), Identifier: aws.String(st.Identifier)})
		if err != nil && !cchelper.IsNotFound(err) {
			return fmt.Errorf("get resource: %w", err)
		}
		if err == nil {
			live = out
		} else {
			st.Identifier = ""
		}
	}

	if live == nil {
		out, err := r.CCClient.CreateResource(ctx, &awscc.CreateResourceInput{
			TypeName: aws.String(typeName), DesiredState: aws.String(string(desired)), ClientToken: aws.String(ccClientToken(obj, "create")),
		})
		if err != nil {
			return fmt.Errorf("create %s: %w", typeName, err)
		}
		return r.trackOperation(ctx, obj, "CREATE", out.ProgressEvent)
	}

	liveJSON := aws.ToString(live.ResourceDescription.Properties)
	patch, err := cchelper.BuildPatch([]byte(liveJSON), desired, nil)
	if err != nil {
		return err
	}
	if patch != nil {
		out, err := r.CCClient.UpdateResource(ctx, &awscc.UpdateResourceInput{
			TypeName: aws.String(typeName), Identifier: aws.String(st.Identifier),
			PatchDocument: aws.String(string(patch)), ClientToken: aws.String(ccClientToken(obj, "update-"+cchelper.ShortHash(patch))),
		})
		if err != nil {
			var nu *cctypes.NotUpdatableException
			if errors.As(err, &nu) {
				return r.setCondition(ctx, obj, metav1.ConditionFalse, awsv1alpha1.ReasonUpdateNotSupported,
					"spec changes a create-only property: "+aws.ToString(nu.Message))
			}
			return fmt.Errorf("update %s: %w", typeName, err)
		}
		return r.trackOperation(ctx, obj, "UPDATE", out.ProgressEvent)
	}

	if err := cfn.FromProperties([]byte(liveJSON), obj.CloudControlObserved()); err != nil {
		log.FromContext(ctx).Info("could not map live properties into status", "error", err.Error())
	}
	st.OperationStatus = string(cctypes.OperationStatusSuccess)
	st.ObservedGeneration = obj.GetGeneration()
	now := metav1.Now()
	st.LastSyncTime = &now
	return r.setCondition(ctx, obj, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, typeName+" in sync")
}

func (r *CloudControlKindReconciler) trackOperation(ctx context.Context, obj cfn.CloudControlObject, op string, ev *cctypes.ProgressEvent) error {
	if ev == nil {
		return fmt.Errorf("%s returned no progress event", op)
	}
	st := obj.CloudControlStatusRef()
	st.Operation = op
	st.RequestToken = aws.ToString(ev.RequestToken)
	st.OperationStatus = string(ev.OperationStatus)
	if id := aws.ToString(ev.Identifier); id != "" {
		st.Identifier = id
	}
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist request token after %s: %w", op, err)
	}
	if ev.OperationStatus == cctypes.OperationStatusFailed {
		st.RequestToken, st.Operation = "", ""
		return fmt.Errorf("%s failed: %s (%s)", op, aws.ToString(ev.StatusMessage), ev.ErrorCode)
	}
	if ev.OperationStatus == cctypes.OperationStatusSuccess {
		st.RequestToken, st.Operation = "", ""
		return persistStatus(ctx, r.Client, obj)
	}
	_ = r.setCondition(ctx, obj, metav1.ConditionFalse, awsv1alpha1.ReasonCreated, op+" in progress")
	return errPendingAcceptance
}

func (r *CloudControlKindReconciler) deleteResource(ctx context.Context, obj cfn.CloudControlObject) (bool, error) {
	st := obj.CloudControlStatusRef()
	if st.RequestToken != "" && st.Operation == "DELETE" {
		done, err := r.pollRequest(ctx, obj)
		if errors.Is(err, errPendingAcceptance) {
			return false, nil
		}
		return done, err
	}
	if st.Identifier == "" {
		return true, nil
	}
	out, err := r.CCClient.DeleteResource(ctx, &awscc.DeleteResourceInput{
		TypeName: aws.String(obj.CloudControlTypeName()), Identifier: aws.String(st.Identifier), ClientToken: aws.String(ccClientToken(obj, "delete")),
	})
	if cchelper.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	st.Operation = "DELETE"
	st.RequestToken = aws.ToString(out.ProgressEvent.RequestToken)
	st.OperationStatus = string(out.ProgressEvent.OperationStatus)
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return false, err
	}
	if out.ProgressEvent.OperationStatus == cctypes.OperationStatusFailed && out.ProgressEvent.ErrorCode == cctypes.HandlerErrorCodeNotFound {
		return true, nil
	}
	return out.ProgressEvent.OperationStatus == cctypes.OperationStatusSuccess, nil
}

// ccClientToken derives a Cloud Control idempotency token that is stable for
// retries of the same operation on the same generation but distinct across
// operations (create/update/delete) and across different patches, so a retry
// after a crash is deduplicated while a follow-up update is not rejected with
// ClientTokenConflictException.
func ccClientToken(obj metav1.Object, op string) string {
	t := fmt.Sprintf("%s-%d-%s", obj.GetUID(), obj.GetGeneration(), op)
	if len(t) > 64 {
		t = t[len(t)-64:]
	}
	return t
}

func (r *CloudControlKindReconciler) setCondition(ctx context.Context, obj cfn.CloudControlObject, status metav1.ConditionStatus, reason, message string) error {
	st := obj.CloudControlStatusRef()
	meta.SetStatusCondition(&st.Conditions, metav1.Condition{
		Type: awsv1alpha1.ConditionReady, Status: status, ObservedGeneration: obj.GetGeneration(), Reason: reason, Message: message,
	})
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

// SetupWithManager registers the controller for this kind under a unique name.
func (r *CloudControlKindReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("cc-" + strings.ToLower(r.Kind.Kind)).
		For(r.Kind.New()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}

// SetupCloudControlKinds starts a controller for every generated kind whose
// CRD is installed in the cluster. Kinds without an installed CRD are skipped
// so operators can opt into service bundles (config/crd/cloudcontrol/<service>.yaml)
// without paying for a thousand watches.
func SetupCloudControlKinds(mgr ctrl.Manager, cc CloudControlAWSAPI) (started, skipped int, err error) {
	mapper := mgr.GetRESTMapper()
	for _, k := range cloudControlKinds {
		if _, mErr := mapper.RESTMapping(schema.GroupKind{Group: awsv1alpha1.GroupVersion.Group, Kind: k.Kind}); mErr != nil {
			skipped++
			continue
		}
		if err := (&CloudControlKindReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), CCClient: cc, Kind: k}).SetupWithManager(mgr); err != nil {
			return started, skipped, fmt.Errorf("%s: %w", k.Kind, err)
		}
		started++
	}
	return started, skipped, nil
}
