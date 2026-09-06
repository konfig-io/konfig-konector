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
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	"github.com/konfig-io/konfig-konector/internal/aws/provider"
	s3helper "github.com/konfig-io/konfig-konector/internal/aws/s3"
)

// S3BucketAWSAPI is the subset of the S3 SDK client used by this controller.
// *awss3.Client satisfies it.
type S3BucketAWSAPI interface {
	HeadBucket(ctx context.Context, params *awss3.HeadBucketInput, optFns ...func(*awss3.Options)) (*awss3.HeadBucketOutput, error)
	CreateBucket(ctx context.Context, params *awss3.CreateBucketInput, optFns ...func(*awss3.Options)) (*awss3.CreateBucketOutput, error)
	PutBucketVersioning(ctx context.Context, params *awss3.PutBucketVersioningInput, optFns ...func(*awss3.Options)) (*awss3.PutBucketVersioningOutput, error)
	PutBucketEncryption(ctx context.Context, params *awss3.PutBucketEncryptionInput, optFns ...func(*awss3.Options)) (*awss3.PutBucketEncryptionOutput, error)
	PutBucketTagging(ctx context.Context, params *awss3.PutBucketTaggingInput, optFns ...func(*awss3.Options)) (*awss3.PutBucketTaggingOutput, error)
	PutPublicAccessBlock(ctx context.Context, params *awss3.PutPublicAccessBlockInput, optFns ...func(*awss3.Options)) (*awss3.PutPublicAccessBlockOutput, error)
	PutBucketLifecycleConfiguration(ctx context.Context, params *awss3.PutBucketLifecycleConfigurationInput, optFns ...func(*awss3.Options)) (*awss3.PutBucketLifecycleConfigurationOutput, error)
	DeleteBucketLifecycle(ctx context.Context, params *awss3.DeleteBucketLifecycleInput, optFns ...func(*awss3.Options)) (*awss3.DeleteBucketLifecycleOutput, error)
	PutBucketCors(ctx context.Context, params *awss3.PutBucketCorsInput, optFns ...func(*awss3.Options)) (*awss3.PutBucketCorsOutput, error)
	DeleteBucketCors(ctx context.Context, params *awss3.DeleteBucketCorsInput, optFns ...func(*awss3.Options)) (*awss3.DeleteBucketCorsOutput, error)
	PutBucketNotificationConfiguration(ctx context.Context, params *awss3.PutBucketNotificationConfigurationInput, optFns ...func(*awss3.Options)) (*awss3.PutBucketNotificationConfigurationOutput, error)
	PutBucketWebsite(ctx context.Context, params *awss3.PutBucketWebsiteInput, optFns ...func(*awss3.Options)) (*awss3.PutBucketWebsiteOutput, error)
	DeleteBucketWebsite(ctx context.Context, params *awss3.DeleteBucketWebsiteInput, optFns ...func(*awss3.Options)) (*awss3.DeleteBucketWebsiteOutput, error)
	PutBucketAccelerateConfiguration(ctx context.Context, params *awss3.PutBucketAccelerateConfigurationInput, optFns ...func(*awss3.Options)) (*awss3.PutBucketAccelerateConfigurationOutput, error)
	PutObjectLockConfiguration(ctx context.Context, params *awss3.PutObjectLockConfigurationInput, optFns ...func(*awss3.Options)) (*awss3.PutObjectLockConfigurationOutput, error)
	PutBucketLogging(ctx context.Context, params *awss3.PutBucketLoggingInput, optFns ...func(*awss3.Options)) (*awss3.PutBucketLoggingOutput, error)
	DeleteBucket(ctx context.Context, params *awss3.DeleteBucketInput, optFns ...func(*awss3.Options)) (*awss3.DeleteBucketOutput, error)
}

// S3BucketReconciler reconciles S3Bucket objects.
type S3BucketReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	S3Client S3BucketAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3buckets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3buckets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3buckets/finalizers,verbs=update

func (r *S3BucketReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	b := &awsv1alpha1.S3Bucket{}
	if err := r.Get(ctx, req.NamespacedName, b); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, b); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !b.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(b, awsv1alpha1.FinalizerName) {
			if shouldAbandon(b) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(b, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, b)
			}
			if err := r.deleteBucket(ctx, b); err != nil {
				logger.Error(err, "failed to delete S3 bucket")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(b, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, b)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(b, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(b, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, b); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileBucket(ctx, b); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, b, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *S3BucketReconciler) reconcileBucket(ctx context.Context, b *awsv1alpha1.S3Bucket) error {
	region := b.Spec.Region
	if region == "" {
		// Prefer the AWSProvider scope's region (multi-account/region), then
		// the operator's own.
		if sc := provider.ScopeFrom(ctx); sc != nil && sc.Region != "" {
			region = sc.Region
		} else {
			region = os.Getenv("AWS_REGION")
		}
	}

	// Check if bucket exists. HeadBucket returns HTTP 404 for missing buckets.
	_, err := r.S3Client.HeadBucket(ctx, &awss3.HeadBucketInput{
		Bucket: aws.String(b.Spec.BucketName),
	})
	if err != nil {
		if !s3helper.IsNotFound(err) {
			return fmt.Errorf("head bucket: %w", err)
		}
		// Bucket doesn't exist — create it.
		createInput := &awss3.CreateBucketInput{
			Bucket: aws.String(b.Spec.BucketName),
		}
		// Non-us-east-1 regions require LocationConstraint.
		if region != "" && region != "us-east-1" {
			createInput.CreateBucketConfiguration = &s3types.CreateBucketConfiguration{
				LocationConstraint: s3types.BucketLocationConstraint(region),
			}
		}
		if _, err := r.S3Client.CreateBucket(ctx, createInput); err != nil {
			return fmt.Errorf("create bucket: %w", err)
		}
		// Persist the identifiers immediately: the AWS resource now exists, and
		// losing them would orphan the bucket if a later Put* step fails.
		b.Status.ARN = fmt.Sprintf("arn:aws:s3:::%s", b.Spec.BucketName)
		b.Status.DomainName = fmt.Sprintf("%s.s3.amazonaws.com", b.Spec.BucketName)
		if err := persistStatus(ctx, r.Client, b); err != nil {
			return fmt.Errorf("persist bucket identifiers after create: %w", err)
		}
	}

	// Sync versioning.
	if b.Spec.Versioning {
		if _, err := r.S3Client.PutBucketVersioning(ctx, &awss3.PutBucketVersioningInput{
			Bucket: aws.String(b.Spec.BucketName),
			VersioningConfiguration: &s3types.VersioningConfiguration{
				Status: s3types.BucketVersioningStatusEnabled,
			},
		}); err != nil {
			return fmt.Errorf("put bucket versioning: %w", err)
		}
	}

	// Sync SSE.
	if b.Spec.ServerSideEncryption != nil {
		rule := s3types.ServerSideEncryptionRule{
			ApplyServerSideEncryptionByDefault: &s3types.ServerSideEncryptionByDefault{
				SSEAlgorithm: s3types.ServerSideEncryption(b.Spec.ServerSideEncryption.SSEAlgorithm),
			},
		}
		if b.Spec.ServerSideEncryption.KMSKeyID != "" {
			rule.ApplyServerSideEncryptionByDefault.KMSMasterKeyID = aws.String(b.Spec.ServerSideEncryption.KMSKeyID)
		}
		if _, err := r.S3Client.PutBucketEncryption(ctx, &awss3.PutBucketEncryptionInput{
			Bucket: aws.String(b.Spec.BucketName),
			ServerSideEncryptionConfiguration: &s3types.ServerSideEncryptionConfiguration{
				Rules: []s3types.ServerSideEncryptionRule{rule},
			},
		}); err != nil {
			return fmt.Errorf("put bucket encryption: %w", err)
		}
	}

	// Sync tags.
	if len(b.Spec.Tags) > 0 {
		tags := make([]s3types.Tag, 0, len(b.Spec.Tags))
		for k, v := range b.Spec.Tags {
			k, v := k, v
			tags = append(tags, s3types.Tag{Key: &k, Value: &v})
		}
		if _, err := r.S3Client.PutBucketTagging(ctx, &awss3.PutBucketTaggingInput{
			Bucket:  aws.String(b.Spec.BucketName),
			Tagging: &s3types.Tagging{TagSet: tags},
		}); err != nil {
			return fmt.Errorf("put bucket tagging: %w", err)
		}
	}

	// Sync block public access.
	if b.Spec.BlockPublicAccess != nil {
		bpa := b.Spec.BlockPublicAccess
		if _, err := r.S3Client.PutPublicAccessBlock(ctx, &awss3.PutPublicAccessBlockInput{
			Bucket: aws.String(b.Spec.BucketName),
			PublicAccessBlockConfiguration: &s3types.PublicAccessBlockConfiguration{
				BlockPublicAcls:       aws.Bool(bpa.BlockPublicAcls),
				BlockPublicPolicy:     aws.Bool(bpa.BlockPublicPolicy),
				IgnorePublicAcls:      aws.Bool(bpa.IgnorePublicAcls),
				RestrictPublicBuckets: aws.Bool(bpa.RestrictPublicBuckets),
			},
		}); err != nil {
			return fmt.Errorf("put public access block: %w", err)
		}
	}

	// Sync lifecycle rules.
	if len(b.Spec.LifecycleRules) > 0 {
		rules := make([]s3types.LifecycleRule, 0, len(b.Spec.LifecycleRules))
		for _, lr := range b.Spec.LifecycleRules {
			rule := s3types.LifecycleRule{
				Status: s3types.ExpirationStatus(lr.Status),
				Filter: &s3types.LifecycleRuleFilter{Prefix: aws.String(lr.Prefix)},
			}
			if lr.ID != "" {
				rule.ID = aws.String(lr.ID)
			}
			if lr.ExpirationDays != nil {
				rule.Expiration = &s3types.LifecycleExpiration{Days: lr.ExpirationDays}
			} else if lr.ExpirationDate != "" {
				rule.Expiration = &s3types.LifecycleExpiration{ExpiredObjectDeleteMarker: aws.Bool(false)}
			}
			if lr.NoncurrentVersionExpirationDays != nil {
				rule.NoncurrentVersionExpiration = &s3types.NoncurrentVersionExpiration{NoncurrentDays: lr.NoncurrentVersionExpirationDays}
			}
			if lr.AbortIncompleteMultipartUploadDays != nil {
				rule.AbortIncompleteMultipartUpload = &s3types.AbortIncompleteMultipartUpload{DaysAfterInitiation: lr.AbortIncompleteMultipartUploadDays}
			}
			for _, t := range lr.Transitions {
				st := s3types.Transition{StorageClass: s3types.TransitionStorageClass(t.StorageClass)}
				if t.Days != nil {
					st.Days = t.Days
				}
				rule.Transitions = append(rule.Transitions, st)
			}
			for _, nvt := range lr.NoncurrentVersionTransitions {
				st := s3types.NoncurrentVersionTransition{StorageClass: s3types.TransitionStorageClass(nvt.StorageClass)}
				if nvt.NoncurrentDays != nil {
					st.NoncurrentDays = nvt.NoncurrentDays
				}
				rule.NoncurrentVersionTransitions = append(rule.NoncurrentVersionTransitions, st)
			}
			rules = append(rules, rule)
		}
		if _, err := r.S3Client.PutBucketLifecycleConfiguration(ctx, &awss3.PutBucketLifecycleConfigurationInput{
			Bucket:                 aws.String(b.Spec.BucketName),
			LifecycleConfiguration: &s3types.BucketLifecycleConfiguration{Rules: rules},
		}); err != nil {
			return fmt.Errorf("put bucket lifecycle: %w", err)
		}
	} else {
		if _, err := r.S3Client.DeleteBucketLifecycle(ctx, &awss3.DeleteBucketLifecycleInput{
			Bucket: aws.String(b.Spec.BucketName),
		}); err != nil && !s3helper.IsNotFound(err) {
			return fmt.Errorf("delete bucket lifecycle: %w", err)
		}
	}

	// Sync CORS rules.
	if len(b.Spec.CORSRules) > 0 {
		corsRules := make([]s3types.CORSRule, 0, len(b.Spec.CORSRules))
		for _, cr := range b.Spec.CORSRules {
			rule := s3types.CORSRule{
				AllowedMethods: cr.AllowedMethods,
				AllowedOrigins: cr.AllowedOrigins,
				AllowedHeaders: cr.AllowedHeaders,
				ExposeHeaders:  cr.ExposeHeaders,
			}
			if cr.ID != "" {
				rule.ID = aws.String(cr.ID)
			}
			if cr.MaxAgeSeconds != nil {
				rule.MaxAgeSeconds = cr.MaxAgeSeconds
			}
			corsRules = append(corsRules, rule)
		}
		if _, err := r.S3Client.PutBucketCors(ctx, &awss3.PutBucketCorsInput{
			Bucket:            aws.String(b.Spec.BucketName),
			CORSConfiguration: &s3types.CORSConfiguration{CORSRules: corsRules},
		}); err != nil {
			return fmt.Errorf("put bucket CORS: %w", err)
		}
	} else {
		if _, err := r.S3Client.DeleteBucketCors(ctx, &awss3.DeleteBucketCorsInput{
			Bucket: aws.String(b.Spec.BucketName),
		}); err != nil && !s3helper.IsNotFound(err) {
			return fmt.Errorf("delete bucket CORS: %w", err)
		}
	}

	// Sync notifications (always put, empty config clears).
	if b.Spec.NotificationConfig != nil || true {
		nc := &s3types.NotificationConfiguration{}
		if b.Spec.NotificationConfig != nil {
			cfg := b.Spec.NotificationConfig
			for _, ln := range cfg.LambdaFunctionConfigurations {
				lc := s3types.LambdaFunctionConfiguration{
					LambdaFunctionArn: aws.String(ln.LambdaFunctionARN),
					Events:            s3eventsFromStrings(ln.Events),
				}
				if ln.ID != "" {
					lc.Id = aws.String(ln.ID)
				}
				lc.Filter = s3keyFilter(ln.FilterPrefix, ln.FilterSuffix)
				nc.LambdaFunctionConfigurations = append(nc.LambdaFunctionConfigurations, lc)
			}
			for _, qn := range cfg.QueueConfigurations {
				qc := s3types.QueueConfiguration{
					QueueArn: aws.String(qn.QueueARN),
					Events:   s3eventsFromStrings(qn.Events),
				}
				if qn.ID != "" {
					qc.Id = aws.String(qn.ID)
				}
				qc.Filter = s3keyFilter(qn.FilterPrefix, qn.FilterSuffix)
				nc.QueueConfigurations = append(nc.QueueConfigurations, qc)
			}
			for _, tn := range cfg.TopicConfigurations {
				tc := s3types.TopicConfiguration{
					TopicArn: aws.String(tn.TopicARN),
					Events:   s3eventsFromStrings(tn.Events),
				}
				if tn.ID != "" {
					tc.Id = aws.String(tn.ID)
				}
				tc.Filter = s3keyFilter(tn.FilterPrefix, tn.FilterSuffix)
				nc.TopicConfigurations = append(nc.TopicConfigurations, tc)
			}
			if cfg.EventBridgeEnabled {
				nc.EventBridgeConfiguration = &s3types.EventBridgeConfiguration{}
			}
		}
		if _, err := r.S3Client.PutBucketNotificationConfiguration(ctx, &awss3.PutBucketNotificationConfigurationInput{
			Bucket:                    aws.String(b.Spec.BucketName),
			NotificationConfiguration: nc,
		}); err != nil {
			return fmt.Errorf("put bucket notifications: %w", err)
		}
	}

	// Sync website config.
	if b.Spec.WebsiteConfig != nil {
		wc := b.Spec.WebsiteConfig
		websiteConf := &s3types.WebsiteConfiguration{}
		if wc.IndexDocument != "" {
			websiteConf.IndexDocument = &s3types.IndexDocument{Suffix: aws.String(wc.IndexDocument)}
		}
		if wc.ErrorDocument != "" {
			websiteConf.ErrorDocument = &s3types.ErrorDocument{Key: aws.String(wc.ErrorDocument)}
		}
		if wc.RedirectAllTo != nil {
			websiteConf.RedirectAllRequestsTo = &s3types.RedirectAllRequestsTo{HostName: aws.String(wc.RedirectAllTo.HostName)}
			if wc.RedirectAllTo.Protocol != "" {
				websiteConf.RedirectAllRequestsTo.Protocol = s3types.Protocol(wc.RedirectAllTo.Protocol)
			}
		}
		if _, err := r.S3Client.PutBucketWebsite(ctx, &awss3.PutBucketWebsiteInput{
			Bucket:               aws.String(b.Spec.BucketName),
			WebsiteConfiguration: websiteConf,
		}); err != nil {
			return fmt.Errorf("put bucket website: %w", err)
		}
		b.Status.WebsiteEndpoint = fmt.Sprintf("%s.s3-website.%s.amazonaws.com", b.Spec.BucketName, region)
	} else {
		if _, err := r.S3Client.DeleteBucketWebsite(ctx, &awss3.DeleteBucketWebsiteInput{
			Bucket: aws.String(b.Spec.BucketName),
		}); err != nil && !s3helper.IsNotFound(err) {
			return fmt.Errorf("delete bucket website: %w", err)
		}
		b.Status.WebsiteEndpoint = ""
	}

	// Sync accelerate status.
	if b.Spec.AccelerateStatus != "" {
		if _, err := r.S3Client.PutBucketAccelerateConfiguration(ctx, &awss3.PutBucketAccelerateConfigurationInput{
			Bucket: aws.String(b.Spec.BucketName),
			AccelerateConfiguration: &s3types.AccelerateConfiguration{
				Status: s3types.BucketAccelerateStatus(b.Spec.AccelerateStatus),
			},
		}); err != nil {
			return fmt.Errorf("put bucket accelerate: %w", err)
		}
	}

	// Sync object lock config.
	if b.Spec.ObjectLockConfig != nil {
		olc := b.Spec.ObjectLockConfig
		input := &awss3.PutObjectLockConfigurationInput{
			Bucket:                  aws.String(b.Spec.BucketName),
			ObjectLockConfiguration: &s3types.ObjectLockConfiguration{},
		}
		if olc.ObjectLockEnabled {
			input.ObjectLockConfiguration.ObjectLockEnabled = s3types.ObjectLockEnabledEnabled
		}
		if olc.Rule != nil {
			dr := olc.Rule.DefaultRetention
			retention := s3types.DefaultRetention{Mode: s3types.ObjectLockRetentionMode(dr.Mode)}
			if dr.Days != nil {
				retention.Days = dr.Days
			}
			if dr.Years != nil {
				retention.Years = dr.Years
			}
			input.ObjectLockConfiguration.Rule = &s3types.ObjectLockRule{
				DefaultRetention: &retention,
			}
		}
		if _, err := r.S3Client.PutObjectLockConfiguration(ctx, input); err != nil {
			return fmt.Errorf("put object lock config: %w", err)
		}
	}

	// Sync logging config.
	if b.Spec.LoggingConfig != nil {
		lc := b.Spec.LoggingConfig
		if _, err := r.S3Client.PutBucketLogging(ctx, &awss3.PutBucketLoggingInput{
			Bucket: aws.String(b.Spec.BucketName),
			BucketLoggingStatus: &s3types.BucketLoggingStatus{
				LoggingEnabled: &s3types.LoggingEnabled{
					TargetBucket: aws.String(lc.TargetBucket),
					TargetPrefix: aws.String(lc.TargetPrefix),
				},
			},
		}); err != nil {
			return fmt.Errorf("put bucket logging: %w", err)
		}
	} else {
		if _, err := r.S3Client.PutBucketLogging(ctx, &awss3.PutBucketLoggingInput{
			Bucket:              aws.String(b.Spec.BucketName),
			BucketLoggingStatus: &s3types.BucketLoggingStatus{},
		}); err != nil && !s3helper.IsNotFound(err) {
			return fmt.Errorf("disable bucket logging: %w", err)
		}
	}

	b.Status.ARN = fmt.Sprintf("arn:aws:s3:::%s", b.Spec.BucketName)
	b.Status.DomainName = fmt.Sprintf("%s.s3.amazonaws.com", b.Spec.BucketName)
	b.Status.ObservedGeneration = b.Generation
	now := metav1.Now()
	b.Status.LastSyncTime = &now
	return r.setCondition(ctx, b, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "S3 bucket reconciled")
}

func s3eventsFromStrings(events []string) []s3types.Event {
	out := make([]s3types.Event, 0, len(events))
	for _, e := range events {
		out = append(out, s3types.Event(e))
	}
	return out
}

func s3keyFilter(prefix, suffix string) *s3types.NotificationConfigurationFilter {
	if prefix == "" && suffix == "" {
		return nil
	}
	var rules []s3types.FilterRule
	if prefix != "" {
		rules = append(rules, s3types.FilterRule{Name: s3types.FilterRuleNamePrefix, Value: aws.String(prefix)})
	}
	if suffix != "" {
		rules = append(rules, s3types.FilterRule{Name: s3types.FilterRuleNameSuffix, Value: aws.String(suffix)})
	}
	return &s3types.NotificationConfigurationFilter{Key: &s3types.S3KeyFilter{FilterRules: rules}}
}

func (r *S3BucketReconciler) deleteBucket(ctx context.Context, b *awsv1alpha1.S3Bucket) error {
	_, err := r.S3Client.DeleteBucket(ctx, &awss3.DeleteBucketInput{
		Bucket: aws.String(b.Spec.BucketName),
	})
	if s3helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *S3BucketReconciler) setCondition(ctx context.Context, b *awsv1alpha1.S3Bucket, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&b.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: b.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, b); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *S3BucketReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.S3Bucket{}).
		Complete(r)
}
