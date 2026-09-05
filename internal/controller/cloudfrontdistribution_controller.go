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

// CloudFrontDistributionReconciler reconciles CloudFrontDistribution objects.
type CloudFrontDistributionReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	CloudFrontClient *multi.CloudFront
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudfrontdistributions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudfrontdistributions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudfrontdistributions/finalizers,verbs=update

func (r *CloudFrontDistributionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.CloudFrontDistribution{}
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
			if err := r.deleteDistribution(ctx, obj); err != nil {
				logger.Error(err, "failed to delete CloudFrontDistribution")
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

	if err := r.reconcileDistribution(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionCFD(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func buildDistributionConfig(obj *awsv1alpha1.CloudFrontDistribution) *cftypes.DistributionConfig {
	origins := make([]cftypes.Origin, 0, len(obj.Spec.Origins))
	for _, o := range obj.Spec.Origins {
		o := o
		origin := cftypes.Origin{
			Id:         &o.ID,
			DomainName: &o.DomainName,
		}
		if o.OriginPath != "" {
			origin.OriginPath = &o.OriginPath
		}
		origins = append(origins, origin)
	}

	dcb := &cftypes.DefaultCacheBehavior{
		TargetOriginId:       aws.String(obj.Spec.DefaultCacheBehavior.TargetOriginID),
		ViewerProtocolPolicy: cftypes.ViewerProtocolPolicy(obj.Spec.DefaultCacheBehavior.ViewerProtocolPolicy),
		ForwardedValues: &cftypes.ForwardedValues{
			QueryString: aws.Bool(false),
			Cookies: &cftypes.CookiePreference{
				Forward: cftypes.ItemSelectionNone,
			},
		},
	}
	if obj.Spec.DefaultCacheBehavior.CachePolicyID != "" {
		dcb.CachePolicyId = aws.String(obj.Spec.DefaultCacheBehavior.CachePolicyID)
		dcb.ForwardedValues = nil
	}

	enabled := true
	if obj.Spec.Enabled != nil {
		enabled = *obj.Spec.Enabled
	}

	cfg := &cftypes.DistributionConfig{
		CallerReference:      aws.String(obj.Name + "-" + string(obj.UID)),
		Comment:              aws.String(obj.Spec.Comment),
		DefaultCacheBehavior: dcb,
		Enabled:              aws.Bool(enabled),
		Origins: &cftypes.Origins{
			Items:    origins,
			Quantity: aws.Int32(int32(len(origins))),
		},
	}

	if len(obj.Spec.Aliases) > 0 {
		cfg.Aliases = &cftypes.Aliases{
			Items:    obj.Spec.Aliases,
			Quantity: aws.Int32(int32(len(obj.Spec.Aliases))),
		}
	}
	if obj.Spec.PriceClass != "" {
		cfg.PriceClass = cftypes.PriceClass(obj.Spec.PriceClass)
	}

	return cfg
}

func (r *CloudFrontDistributionReconciler) reconcileDistribution(ctx context.Context, obj *awsv1alpha1.CloudFrontDistribution) error {
	if obj.Status.DistributionID != "" {
		getOut, err := r.CloudFrontClient.GetDistribution(ctx, &awscf.GetDistributionInput{
			Id: aws.String(obj.Status.DistributionID),
		})
		if err != nil && !cfhelper.IsNotFound(err) {
			return fmt.Errorf("get cloudfront distribution: %w", err)
		}
		if err == nil && getOut.Distribution != nil {
			obj.Status.DomainName = aws.ToString(getOut.Distribution.DomainName)
			obj.Status.Status = aws.ToString(getOut.Distribution.Status)
			obj.Status.ETag = aws.ToString(getOut.ETag)

			// Only push an update when the spec has changed since the last
			// successful reconcile; the describe above keeps status fresh.
			if obj.Status.ObservedGeneration != obj.Generation {
				updateCfg := buildDistributionConfig(obj)
				// CallerReference must not change on update.
				if getOut.Distribution.DistributionConfig != nil {
					updateCfg.CallerReference = getOut.Distribution.DistributionConfig.CallerReference
				}
				_, err := r.CloudFrontClient.UpdateDistribution(ctx, &awscf.UpdateDistributionInput{
					Id:                 aws.String(obj.Status.DistributionID),
					IfMatch:            aws.String(obj.Status.ETag),
					DistributionConfig: updateCfg,
				})
				if err != nil {
					return fmt.Errorf("update cloudfront distribution: %w", err)
				}
			}
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionCFD(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "CloudFrontDistribution reconciled")
		}
		obj.Status.DistributionID = ""
	}

	tags := make([]cftypes.Tag, 0, len(obj.Spec.Tags))
	for k, v := range obj.Spec.Tags {
		k, v := k, v
		tags = append(tags, cftypes.Tag{Key: &k, Value: &v})
	}

	createInput := &awscf.CreateDistributionInput{
		DistributionConfig: buildDistributionConfig(obj),
	}
	if len(tags) > 0 {
		createInput = nil
		withTagsInput := &awscf.CreateDistributionWithTagsInput{
			DistributionConfigWithTags: &cftypes.DistributionConfigWithTags{
				DistributionConfig: buildDistributionConfig(obj),
				Tags: &cftypes.Tags{
					Items: tags,
				},
			},
		}
		out, err := r.CloudFrontClient.CreateDistributionWithTags(ctx, withTagsInput)
		if err != nil {
			return fmt.Errorf("create cloudfront distribution with tags: %w", err)
		}
		if out.Distribution != nil {
			obj.Status.DistributionID = aws.ToString(out.Distribution.Id)
			obj.Status.DistributionARN = aws.ToString(out.Distribution.ARN)
			obj.Status.DomainName = aws.ToString(out.Distribution.DomainName)
			obj.Status.Status = aws.ToString(out.Distribution.Status)
			obj.Status.ETag = aws.ToString(out.ETag)
			// The AWS resource now exists; losing the ID would orphan it
			// (a retry would create a second distribution).
			if err := persistStatus(ctx, r.Client, obj); err != nil {
				return fmt.Errorf("persist status after create: %w", err)
			}
		}
	}

	if createInput != nil {
		out, err := r.CloudFrontClient.CreateDistribution(ctx, createInput)
		if err != nil {
			return fmt.Errorf("create cloudfront distribution: %w", err)
		}
		if out.Distribution != nil {
			obj.Status.DistributionID = aws.ToString(out.Distribution.Id)
			obj.Status.DistributionARN = aws.ToString(out.Distribution.ARN)
			obj.Status.DomainName = aws.ToString(out.Distribution.DomainName)
			obj.Status.Status = aws.ToString(out.Distribution.Status)
			obj.Status.ETag = aws.ToString(out.ETag)
			// The AWS resource now exists; losing the ID would orphan it
			// (a retry would create a second distribution).
			if err := persistStatus(ctx, r.Client, obj); err != nil {
				return fmt.Errorf("persist status after create: %w", err)
			}
		}
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionCFD(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "CloudFrontDistribution created")
}

func (r *CloudFrontDistributionReconciler) deleteDistribution(ctx context.Context, obj *awsv1alpha1.CloudFrontDistribution) error {
	if obj.Status.DistributionID == "" {
		return nil
	}

	// Must disable before deleting.
	getOut, err := r.CloudFrontClient.GetDistribution(ctx, &awscf.GetDistributionInput{
		Id: aws.String(obj.Status.DistributionID),
	})
	if cfhelper.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if getOut.Distribution != nil && getOut.Distribution.DistributionConfig != nil {
		if aws.ToBool(getOut.Distribution.DistributionConfig.Enabled) {
			cfg := getOut.Distribution.DistributionConfig
			cfg.Enabled = aws.Bool(false)
			_, err := r.CloudFrontClient.UpdateDistribution(ctx, &awscf.UpdateDistributionInput{
				Id:                 aws.String(obj.Status.DistributionID),
				IfMatch:            getOut.ETag,
				DistributionConfig: cfg,
			})
			if err != nil {
				return fmt.Errorf("disable cloudfront distribution: %w", err)
			}
			// Distribution is still deploying; requeue.
			return fmt.Errorf("cloudfront distribution is being disabled, requeue")
		}

		if aws.ToString(getOut.Distribution.Status) == "InProgress" {
			return fmt.Errorf("cloudfront distribution deployment in progress, requeue")
		}
	}

	_, err = r.CloudFrontClient.DeleteDistribution(ctx, &awscf.DeleteDistributionInput{
		Id:      aws.String(obj.Status.DistributionID),
		IfMatch: getOut.ETag,
	})
	if cfhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *CloudFrontDistributionReconciler) setConditionCFD(ctx context.Context, obj *awsv1alpha1.CloudFrontDistribution, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *CloudFrontDistributionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CloudFrontDistribution{}).
		Complete(r)
}
