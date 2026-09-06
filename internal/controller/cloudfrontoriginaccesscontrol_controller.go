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
	awscf "github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	cfhelper "github.com/konfig-io/konfig-konector/internal/aws/cloudfront"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// CloudFrontOriginAccessControlReconciler reconciles CloudFrontOriginAccessControl objects.
type CloudFrontOriginAccessControlReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	CloudFrontClient *multi.CloudFront
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudfrontoriginaccesscontrols,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudfrontoriginaccesscontrols/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudfrontoriginaccesscontrols/finalizers,verbs=update

func (r *CloudFrontOriginAccessControlReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.CloudFrontOriginAccessControl{}
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
			if err := r.deleteOAC(ctx, obj); err != nil {
				logger.Error(err, "failed to delete CloudFrontOriginAccessControl")
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

	if err := r.reconcileOAC(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionCFOAC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func buildOACConfig(obj *awsv1alpha1.CloudFrontOriginAccessControl) *cftypes.OriginAccessControlConfig {
	signingProtocol := cftypes.OriginAccessControlSigningProtocolsSigv4
	cfg := &cftypes.OriginAccessControlConfig{
		Name:                          aws.String(obj.Spec.Name),
		OriginAccessControlOriginType: cftypes.OriginAccessControlOriginTypes(obj.Spec.OriginAccessControlOriginType),
		SigningBehavior:               cftypes.OriginAccessControlSigningBehaviors(obj.Spec.SigningBehavior),
		SigningProtocol:               signingProtocol,
	}
	if obj.Spec.Description != "" {
		cfg.Description = aws.String(obj.Spec.Description)
	}
	return cfg
}

func (r *CloudFrontOriginAccessControlReconciler) reconcileOAC(ctx context.Context, obj *awsv1alpha1.CloudFrontOriginAccessControl) error {
	if obj.Status.ID != "" {
		getOut, err := r.CloudFrontClient.GetOriginAccessControl(ctx, &awscf.GetOriginAccessControlInput{
			Id: aws.String(obj.Status.ID),
		})
		if err != nil && !cfhelper.IsNotFound(err) {
			return fmt.Errorf("get cloudfront oac: %w", err)
		}
		if err == nil && getOut.OriginAccessControl != nil {
			obj.Status.ETag = aws.ToString(getOut.ETag)
			// CloudFront control-plane APIs are rate limited; only update on spec change.
			if obj.Status.ObservedGeneration != obj.Generation {
				if _, err := r.CloudFrontClient.UpdateOriginAccessControl(ctx, &awscf.UpdateOriginAccessControlInput{
					Id:                        aws.String(obj.Status.ID),
					IfMatch:                   aws.String(obj.Status.ETag),
					OriginAccessControlConfig: buildOACConfig(obj),
				}); err != nil {
					return fmt.Errorf("update cloudfront oac: %w", err)
				}
			}
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionCFOAC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "CloudFrontOriginAccessControl reconciled")
		}
		obj.Status.ID = ""
	}

	out, err := r.CloudFrontClient.CreateOriginAccessControl(ctx, &awscf.CreateOriginAccessControlInput{
		OriginAccessControlConfig: buildOACConfig(obj),
	})
	if err != nil {
		return fmt.Errorf("create cloudfront oac: %w", err)
	}
	if out.OriginAccessControl != nil {
		obj.Status.ID = aws.ToString(out.OriginAccessControl.Id)
	}
	obj.Status.ETag = aws.ToString(out.ETag)
	// The AWS resource now exists; losing the ID would orphan it.
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist status after create: %w", err)
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionCFOAC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "CloudFrontOriginAccessControl created")
}

// findOACIDByName looks up an origin access control by its unique name.
// CloudFront enforces unique OAC names per account, so an exact name match
// is the one this CR created. Returns "" when no match is found.
func (r *CloudFrontOriginAccessControlReconciler) findOACIDByName(ctx context.Context, name string) (string, error) {
	var marker *string
	for {
		listOut, err := r.CloudFrontClient.ListOriginAccessControls(ctx, &awscf.ListOriginAccessControlsInput{
			Marker: marker,
		})
		if err != nil {
			return "", fmt.Errorf("list cloudfront origin access controls: %w", err)
		}
		if listOut.OriginAccessControlList == nil {
			return "", nil
		}
		for _, item := range listOut.OriginAccessControlList.Items {
			if aws.ToString(item.Name) == name {
				return aws.ToString(item.Id), nil
			}
		}
		if listOut.OriginAccessControlList.NextMarker == nil {
			return "", nil
		}
		marker = listOut.OriginAccessControlList.NextMarker
	}
}

func (r *CloudFrontOriginAccessControlReconciler) deleteOAC(ctx context.Context, obj *awsv1alpha1.CloudFrontOriginAccessControl) error {
	if obj.Status.ID == "" {
		// Status may have been lost after a successful create; fall back to
		// looking the OAC up by its unique name before giving up.
		id, err := r.findOACIDByName(ctx, obj.Spec.Name)
		if err != nil {
			return err
		}
		if id == "" {
			return nil
		}
		obj.Status.ID = id
	}
	// Always fetch the current ETag (see CloudFrontFunction).
	var etag string
	{
		getOut, err := r.CloudFrontClient.GetOriginAccessControl(ctx, &awscf.GetOriginAccessControlInput{
			Id: aws.String(obj.Status.ID),
		})
		if cfhelper.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		etag = aws.ToString(getOut.ETag)
	}
	_, err := r.CloudFrontClient.DeleteOriginAccessControl(ctx, &awscf.DeleteOriginAccessControlInput{
		Id:      aws.String(obj.Status.ID),
		IfMatch: aws.String(etag),
	})
	if cfhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *CloudFrontOriginAccessControlReconciler) setConditionCFOAC(ctx context.Context, obj *awsv1alpha1.CloudFrontOriginAccessControl, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *CloudFrontOriginAccessControlReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CloudFrontOriginAccessControl{}).
		Complete(r)
}
