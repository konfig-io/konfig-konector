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
	"crypto/sha256"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsroute53 "github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	r53helper "github.com/konfig-io/konfig-konector/internal/aws/route53"
)

// healthCheckCallerRef produces a CallerReference ≤ 64 chars by hashing the UID.
// Route53 enforces a maxLength of 64 for HealthCheckNonce.
func healthCheckCallerRef(uid string) string {
	h := fmt.Sprintf("%x", sha256.Sum256([]byte(uid)))[:16]
	return "kk-" + h
}

// HealthCheckAWSAPI is the subset of the Route53 API used by
// HealthCheckReconciler. It is satisfied by *route53.Client.
type HealthCheckAWSAPI interface {
	CreateHealthCheck(ctx context.Context, params *awsroute53.CreateHealthCheckInput, optFns ...func(*awsroute53.Options)) (*awsroute53.CreateHealthCheckOutput, error)
	GetHealthCheck(ctx context.Context, params *awsroute53.GetHealthCheckInput, optFns ...func(*awsroute53.Options)) (*awsroute53.GetHealthCheckOutput, error)
	UpdateHealthCheck(ctx context.Context, params *awsroute53.UpdateHealthCheckInput, optFns ...func(*awsroute53.Options)) (*awsroute53.UpdateHealthCheckOutput, error)
	DeleteHealthCheck(ctx context.Context, params *awsroute53.DeleteHealthCheckInput, optFns ...func(*awsroute53.Options)) (*awsroute53.DeleteHealthCheckOutput, error)
	ListHealthChecks(ctx context.Context, params *awsroute53.ListHealthChecksInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ListHealthChecksOutput, error)
	ChangeTagsForResource(ctx context.Context, params *awsroute53.ChangeTagsForResourceInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ChangeTagsForResourceOutput, error)
}

// HealthCheckReconciler reconciles HealthCheck objects.
type HealthCheckReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	Route53Client   HealthCheckAWSAPI
	CallerReference string
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=healthchecks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=healthchecks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=healthchecks/finalizers,verbs=update

func (r *HealthCheckReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	hc := &awsv1alpha1.HealthCheck{}
	if err := r.Get(ctx, req.NamespacedName, hc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, hc); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !hc.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(hc, awsv1alpha1.FinalizerName) {
			if shouldAbandon(hc) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(hc, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, hc)
			}
			if err := r.deleteHealthCheck(ctx, hc); err != nil {
				logger.Error(err, "failed to delete health check")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(hc, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, hc)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(hc, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(hc, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, hc); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileHealthCheck(ctx, hc); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setHCCondition(ctx, hc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *HealthCheckReconciler) reconcileHealthCheck(ctx context.Context, hc *awsv1alpha1.HealthCheck) error {
	if hc.Status.HealthCheckID != "" {
		// Verify it still exists.
		out, err := r.Route53Client.GetHealthCheck(ctx, &awsroute53.GetHealthCheckInput{
			HealthCheckId: aws.String(hc.Status.HealthCheckID),
		})
		if err != nil && !r53helper.IsNotFound(err) {
			return err
		}
		if out != nil {
			// Only update if spec has drifted from observed generation.
			if hc.Status.ObservedGeneration != hc.Generation {
				cfg := r.buildHealthCheckConfig(hc)
				if _, err := r.Route53Client.UpdateHealthCheck(ctx, &awsroute53.UpdateHealthCheckInput{
					HealthCheckId:            aws.String(hc.Status.HealthCheckID),
					IPAddress:                cfg.IPAddress,
					FullyQualifiedDomainName: cfg.FullyQualifiedDomainName,
					Port:                     cfg.Port,
					ResourcePath:             cfg.ResourcePath,
					SearchString:             cfg.SearchString,
					FailureThreshold:         cfg.FailureThreshold,
				}); err != nil {
					return fmt.Errorf("update health check: %w", err)
				}
			}
			goto syncTags
		}
		hc.Status.HealthCheckID = ""
	}

	{
		cfg := r.buildHealthCheckConfig(hc)
		out, err := r.Route53Client.CreateHealthCheck(ctx, &awsroute53.CreateHealthCheckInput{
			CallerReference:   aws.String(healthCheckCallerRef(string(hc.UID))),
			HealthCheckConfig: cfg,
		})
		if err != nil {
			return fmt.Errorf("create health check: %w", err)
		}
		hc.Status.HealthCheckID = aws.ToString(out.HealthCheck.Id)
		// The AWS resource now exists; losing the ID would orphan it.
		if err := persistStatus(ctx, r.Client, hc); err != nil {
			return fmt.Errorf("persist status after create: %w", err)
		}
	}

syncTags:
	// Sync tags.
	var tagList []types.Tag
	for k, v := range hc.Spec.Tags {
		tagList = append(tagList, types.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	if len(tagList) > 0 {
		if _, err := r.Route53Client.ChangeTagsForResource(ctx, &awsroute53.ChangeTagsForResourceInput{
			ResourceType: types.TagResourceTypeHealthcheck,
			ResourceId:   aws.String(hc.Status.HealthCheckID),
			AddTags:      tagList,
		}); err != nil {
			return fmt.Errorf("sync tags: %w", err)
		}
	}

	hc.Status.ObservedGeneration = hc.Generation
	now := metav1.Now()
	hc.Status.LastSyncTime = &now
	return r.setHCCondition(ctx, hc, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "health check reconciled")
}

// deleteHealthCheck deletes the health check recorded in status. If the
// status ID was lost after a successful create, it falls back to listing
// health checks and matching the CallerReference derived from this CR's UID,
// which uniquely identifies the health check this CR created.
func (r *HealthCheckReconciler) deleteHealthCheck(ctx context.Context, hc *awsv1alpha1.HealthCheck) error {
	if hc.Status.HealthCheckID == "" {
		wantRef := healthCheckCallerRef(string(hc.UID))
		paginator := awsroute53.NewListHealthChecksPaginator(r.Route53Client, &awsroute53.ListHealthChecksInput{})
		for paginator.HasMorePages() && hc.Status.HealthCheckID == "" {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				return fmt.Errorf("list health checks: %w", err)
			}
			for _, check := range page.HealthChecks {
				if aws.ToString(check.CallerReference) == wantRef {
					hc.Status.HealthCheckID = aws.ToString(check.Id)
					break
				}
			}
		}
		if hc.Status.HealthCheckID == "" {
			return nil
		}
	}
	if _, err := r.Route53Client.DeleteHealthCheck(ctx, &awsroute53.DeleteHealthCheckInput{
		HealthCheckId: aws.String(hc.Status.HealthCheckID),
	}); err != nil && !r53helper.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *HealthCheckReconciler) buildHealthCheckConfig(hc *awsv1alpha1.HealthCheck) *types.HealthCheckConfig {
	cfg := &types.HealthCheckConfig{
		Type: types.HealthCheckType(hc.Spec.Type),
	}
	if hc.Spec.IPAddress != "" {
		cfg.IPAddress = aws.String(hc.Spec.IPAddress)
	}
	if hc.Spec.FQDN != "" {
		cfg.FullyQualifiedDomainName = aws.String(hc.Spec.FQDN)
	}
	if hc.Spec.Port > 0 {
		cfg.Port = aws.Int32(hc.Spec.Port)
	}
	if hc.Spec.ResourcePath != "" {
		cfg.ResourcePath = aws.String(hc.Spec.ResourcePath)
	}
	if hc.Spec.SearchString != "" {
		cfg.SearchString = aws.String(hc.Spec.SearchString)
	}
	if hc.Spec.RequestInterval > 0 {
		cfg.RequestInterval = aws.Int32(hc.Spec.RequestInterval)
	}
	if hc.Spec.FailureThreshold > 0 {
		cfg.FailureThreshold = aws.Int32(hc.Spec.FailureThreshold)
	}
	return cfg
}

func (r *HealthCheckReconciler) setHCCondition(ctx context.Context, hc *awsv1alpha1.HealthCheck, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&hc.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: hc.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, hc); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *HealthCheckReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.HealthCheck{}).
		Complete(r)
}
