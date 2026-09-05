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
	awscloudtrail "github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	cloudtrailtypes "github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	cloudtrailhelper "github.com/konfig-io/konfig-konector/internal/aws/cloudtrail"
)

// TrailAWSAPI is the subset of the CloudTrail API used by this controller.
type TrailAWSAPI interface {
	GetTrail(ctx context.Context, params *awscloudtrail.GetTrailInput, optFns ...func(*awscloudtrail.Options)) (*awscloudtrail.GetTrailOutput, error)
	CreateTrail(ctx context.Context, params *awscloudtrail.CreateTrailInput, optFns ...func(*awscloudtrail.Options)) (*awscloudtrail.CreateTrailOutput, error)
	UpdateTrail(ctx context.Context, params *awscloudtrail.UpdateTrailInput, optFns ...func(*awscloudtrail.Options)) (*awscloudtrail.UpdateTrailOutput, error)
	DeleteTrail(ctx context.Context, params *awscloudtrail.DeleteTrailInput, optFns ...func(*awscloudtrail.Options)) (*awscloudtrail.DeleteTrailOutput, error)
	StartLogging(ctx context.Context, params *awscloudtrail.StartLoggingInput, optFns ...func(*awscloudtrail.Options)) (*awscloudtrail.StartLoggingOutput, error)
	StopLogging(ctx context.Context, params *awscloudtrail.StopLoggingInput, optFns ...func(*awscloudtrail.Options)) (*awscloudtrail.StopLoggingOutput, error)
	GetTrailStatus(ctx context.Context, params *awscloudtrail.GetTrailStatusInput, optFns ...func(*awscloudtrail.Options)) (*awscloudtrail.GetTrailStatusOutput, error)
	AddTags(ctx context.Context, params *awscloudtrail.AddTagsInput, optFns ...func(*awscloudtrail.Options)) (*awscloudtrail.AddTagsOutput, error)
}

// TrailReconciler reconciles Trail objects.
type TrailReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	CloudTrailClient TrailAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=trails,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=trails/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=trails/finalizers,verbs=update

func (r *TrailReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	trail := &awsv1alpha1.Trail{}
	if err := r.Get(ctx, req.NamespacedName, trail); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, trail); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !trail.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(trail, awsv1alpha1.FinalizerName) {
			if shouldAbandon(trail) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(trail, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, trail)
			}
			if err := r.deleteTrail(ctx, trail); err != nil {
				logger.Error(err, "failed to delete CloudTrail trail")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(trail, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, trail)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(trail, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(trail, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, trail); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileTrail(ctx, trail); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, trail, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *TrailReconciler) reconcileTrail(ctx context.Context, trail *awsv1alpha1.Trail) error {
	getOut, err := r.CloudTrailClient.GetTrail(ctx, &awscloudtrail.GetTrailInput{
		Name: aws.String(r.trailIdentifier(trail)),
	})

	var trailARN string
	if cloudtrailhelper.IsNotFound(err) {
		createOut, err := r.CloudTrailClient.CreateTrail(ctx, &awscloudtrail.CreateTrailInput{
			Name:                       aws.String(trail.Spec.TrailName),
			S3BucketName:               aws.String(trail.Spec.S3BucketName),
			S3KeyPrefix:                govOptionalStr(trail.Spec.S3KeyPrefix),
			IncludeGlobalServiceEvents: aws.Bool(trail.Spec.IncludeGlobalServiceEvents),
			IsMultiRegionTrail:         aws.Bool(trail.Spec.IsMultiRegionTrail),
			EnableLogFileValidation:    aws.Bool(trail.Spec.EnableLogFileValidation),
			CloudWatchLogsLogGroupArn:  govOptionalStr(trail.Spec.CloudWatchLogsLogGroupArn),
			CloudWatchLogsRoleArn:      govOptionalStr(trail.Spec.CloudWatchLogsRoleArn),
			KmsKeyId:                   govOptionalStr(trail.Spec.KMSKeyID),
			TagsList:                   cloudtrailTags(trail.Spec.Tags),
		})
		if err != nil {
			return fmt.Errorf("create trail: %w", err)
		}
		trailARN = aws.ToString(createOut.TrailARN)
		// Persist the ARN immediately: the AWS resource now exists, and
		// losing the identifier would orphan it on delete.
		trail.Status.TrailARN = trailARN
		if err := persistStatus(ctx, r.Client, trail); err != nil {
			return fmt.Errorf("persist trail ARN after create: %w", err)
		}
	} else if err != nil {
		return err
	} else {
		trailARN = aws.ToString(getOut.Trail.TrailARN)
		if trail.Status.TrailARN == "" {
			trail.Status.TrailARN = trailARN
			if err := persistStatus(ctx, r.Client, trail); err != nil {
				return fmt.Errorf("persist trail ARN after lookup: %w", err)
			}
		}

		if trail.Status.ObservedGeneration != trail.Generation {
			if _, err := r.CloudTrailClient.UpdateTrail(ctx, &awscloudtrail.UpdateTrailInput{
				Name:                       aws.String(trailARN),
				S3BucketName:               aws.String(trail.Spec.S3BucketName),
				S3KeyPrefix:                govOptionalStr(trail.Spec.S3KeyPrefix),
				IncludeGlobalServiceEvents: aws.Bool(trail.Spec.IncludeGlobalServiceEvents),
				IsMultiRegionTrail:         aws.Bool(trail.Spec.IsMultiRegionTrail),
				EnableLogFileValidation:    aws.Bool(trail.Spec.EnableLogFileValidation),
				CloudWatchLogsLogGroupArn:  govOptionalStr(trail.Spec.CloudWatchLogsLogGroupArn),
				CloudWatchLogsRoleArn:      govOptionalStr(trail.Spec.CloudWatchLogsRoleArn),
				KmsKeyId:                   govOptionalStr(trail.Spec.KMSKeyID),
			}); err != nil {
				return fmt.Errorf("update trail: %w", err)
			}
			if len(trail.Spec.Tags) > 0 {
				if _, err := r.CloudTrailClient.AddTags(ctx, &awscloudtrail.AddTagsInput{
					ResourceId: aws.String(trailARN),
					TagsList:   cloudtrailTags(trail.Spec.Tags),
				}); err != nil {
					return fmt.Errorf("add tags: %w", err)
				}
			}
		}
	}

	// Converge the logging state.
	statusOut, err := r.CloudTrailClient.GetTrailStatus(ctx, &awscloudtrail.GetTrailStatusInput{
		Name: aws.String(trailARN),
	})
	if err != nil {
		return fmt.Errorf("get trail status: %w", err)
	}
	isLogging := aws.ToBool(statusOut.IsLogging)
	wantLogging := trail.Spec.EnableLogging == nil || *trail.Spec.EnableLogging
	if wantLogging && !isLogging {
		if _, err := r.CloudTrailClient.StartLogging(ctx, &awscloudtrail.StartLoggingInput{Name: aws.String(trailARN)}); err != nil {
			return fmt.Errorf("start logging: %w", err)
		}
		isLogging = true
	} else if !wantLogging && isLogging {
		if _, err := r.CloudTrailClient.StopLogging(ctx, &awscloudtrail.StopLoggingInput{Name: aws.String(trailARN)}); err != nil {
			return fmt.Errorf("stop logging: %w", err)
		}
		isLogging = false
	}

	trail.Status.TrailARN = trailARN
	trail.Status.IsLogging = isLogging
	trail.Status.ObservedGeneration = trail.Generation
	now := metav1.Now()
	trail.Status.LastSyncTime = &now
	return r.setCondition(ctx, trail, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "CloudTrail trail reconciled")
}

// trailIdentifier prefers the persisted ARN (which works for multi-region
// trails from any region) and falls back to the spec name.
func (r *TrailReconciler) trailIdentifier(trail *awsv1alpha1.Trail) string {
	if trail.Status.TrailARN != "" {
		return trail.Status.TrailARN
	}
	return trail.Spec.TrailName
}

func (r *TrailReconciler) deleteTrail(ctx context.Context, trail *awsv1alpha1.Trail) error {
	// The trail name is a deterministic spec-based identifier, so deletion
	// works even if the status ARN was never persisted.
	_, err := r.CloudTrailClient.DeleteTrail(ctx, &awscloudtrail.DeleteTrailInput{
		Name: aws.String(r.trailIdentifier(trail)),
	})
	if cloudtrailhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *TrailReconciler) setCondition(ctx context.Context, trail *awsv1alpha1.Trail, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&trail.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: trail.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, trail); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

// govOptionalStr returns nil for empty strings so optional AWS fields are omitted.
func govOptionalStr(s string) *string {
	if s == "" {
		return nil
	}
	return aws.String(s)
}

func cloudtrailTags(tags map[string]string) []cloudtrailtypes.Tag {
	if len(tags) == 0 {
		return nil
	}
	out := make([]cloudtrailtypes.Tag, 0, len(tags))
	for k, v := range tags {
		out = append(out, cloudtrailtypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	return out
}

func (r *TrailReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.Trail{}).
		Complete(r)
}
