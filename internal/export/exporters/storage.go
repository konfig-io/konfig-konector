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

package exporters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsecr "github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	awsefs "github.com/aws/aws-sdk-go-v2/service/efs"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	// Buckets and file systems (40) before their subresources (41+).
	export.Register(export.Exporter{Kind: "S3Bucket", Service: "s3", Order: 40, Fn: exportS3Buckets})
	export.Register(export.Exporter{Kind: "S3BucketPolicy", Service: "s3", Order: 41, Fn: exportS3BucketPolicies})
	export.Register(export.Exporter{Kind: "S3BucketCORS", Service: "s3", Order: 41, Fn: exportS3BucketCORS})
	export.Register(export.Exporter{Kind: "S3BucketLifecycle", Service: "s3", Order: 41, Fn: exportS3BucketLifecycles})
	export.Register(export.Exporter{Kind: "S3BucketNotification", Service: "s3", Order: 41, Fn: exportS3BucketNotifications})
	export.Register(export.Exporter{Kind: "S3BucketReplication", Service: "s3", Order: 41, Fn: exportS3BucketReplications})
	export.Register(export.Exporter{Kind: "EFSFileSystem", Service: "efs", Order: 40, Fn: exportEFSFileSystems})
	export.Register(export.Exporter{Kind: "EFSMountTarget", Service: "efs", Order: 41, Fn: exportEFSMountTargets})
	export.Register(export.Exporter{Kind: "EFSAccessPoint", Service: "efs", Order: 41, Fn: exportEFSAccessPoints})
	export.Register(export.Exporter{Kind: "ECRRepository", Service: "ecr", Order: 40, Fn: exportECRRepositories})
	export.Register(export.Exporter{Kind: "ECRLifecyclePolicy", Service: "ecr", Order: 41, Fn: exportECRLifecyclePolicies})
	export.Register(export.Exporter{Kind: "ECRRepositoryPolicy", Service: "ecr", Order: 41, Fn: exportECRRepositoryPolicies})
}

// s3IgnoreNotFound swallows the NoSuchX / XNotFound error family that S3
// returns for unset bucket subresources — for export purposes those simply
// mean "not configured", not a failure.
func s3IgnoreNotFound(err error) error {
	if err == nil {
		return nil
	}
	var ae smithy.APIError
	if errors.As(err, &ae) {
		switch ae.ErrorCode() {
		case "NoSuchBucketPolicy",
			"NoSuchCORSConfiguration",
			"NoSuchLifecycleConfiguration",
			"NoSuchWebsiteConfiguration",
			"NoSuchTagSet",
			"NoSuchPublicAccessBlockConfiguration",
			"ObjectLockConfigurationNotFoundError",
			"ObjectLockConfigurationNotFound",
			"ReplicationConfigurationNotFoundError",
			"ServerSideEncryptionConfigurationNotFoundError",
			"NotFound", "NoSuchBucket":
			return nil
		}
	}
	return err
}

// listOwnedBuckets returns bucket names that live in the client's region,
// skipping buckets homed elsewhere (their subresource calls would fail with
// redirects and their config belongs to another region's export).
func listOwnedBuckets(ctx context.Context, clients *awsclient.Clients) ([]string, error) {
	out, err := clients.S3.ListBuckets(ctx, &awss3.ListBucketsInput{})
	if err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}
	region := clients.Config.Region
	var names []string
	for _, b := range out.Buckets {
		name := aws.ToString(b.Name)
		loc, err := clients.S3.GetBucketLocation(ctx, &awss3.GetBucketLocationInput{Bucket: b.Name})
		if err != nil {
			continue // per-bucket errors: skip
		}
		bucketRegion := string(loc.LocationConstraint)
		if bucketRegion == "" {
			bucketRegion = "us-east-1" // legacy null constraint
		}
		if bucketRegion != region {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

func exportS3Buckets(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	names, err := listOwnedBuckets(ctx, clients)
	if err != nil {
		return nil, err
	}
	region := clients.Config.Region
	var objs []client.Object
	for _, name := range names {
		b := &awsv1alpha1.S3Bucket{
			ObjectMeta: export.ObjectMeta(name, opts),
			Spec: awsv1alpha1.S3BucketSpec{
				BucketName: name,
				Region:     region,
			},
		}
		bucket := aws.String(name)

		// Versioning.
		if v, err := clients.S3.GetBucketVersioning(ctx, &awss3.GetBucketVersioningInput{Bucket: bucket}); err == nil {
			b.Spec.Versioning = v.Status == s3types.BucketVersioningStatusEnabled
		}

		// Default encryption. NotFound just means SSE was never configured.
		if enc, err := clients.S3.GetBucketEncryption(ctx, &awss3.GetBucketEncryptionInput{Bucket: bucket}); s3IgnoreNotFound(err) == nil && err == nil {
			if enc.ServerSideEncryptionConfiguration != nil && len(enc.ServerSideEncryptionConfiguration.Rules) > 0 {
				r := enc.ServerSideEncryptionConfiguration.Rules[0]
				if r.ApplyServerSideEncryptionByDefault != nil {
					b.Spec.ServerSideEncryption = &awsv1alpha1.S3BucketEncryption{
						SSEAlgorithm: string(r.ApplyServerSideEncryptionByDefault.SSEAlgorithm),
						KMSKeyID:     aws.ToString(r.ApplyServerSideEncryptionByDefault.KMSMasterKeyID),
					}
				}
			}
		}

		// Block Public Access.
		if pab, err := clients.S3.GetPublicAccessBlock(ctx, &awss3.GetPublicAccessBlockInput{Bucket: bucket}); s3IgnoreNotFound(err) == nil && err == nil {
			if c := pab.PublicAccessBlockConfiguration; c != nil {
				b.Spec.BlockPublicAccess = &awsv1alpha1.S3BlockPublicAccess{
					BlockPublicAcls:       aws.ToBool(c.BlockPublicAcls),
					BlockPublicPolicy:     aws.ToBool(c.BlockPublicPolicy),
					IgnorePublicAcls:      aws.ToBool(c.IgnorePublicAcls),
					RestrictPublicBuckets: aws.ToBool(c.RestrictPublicBuckets),
				}
			}
		}

		// Tags.
		if tags, err := clients.S3.GetBucketTagging(ctx, &awss3.GetBucketTaggingInput{Bucket: bucket}); s3IgnoreNotFound(err) == nil && err == nil {
			m := make(map[string]string, len(tags.TagSet))
			for _, t := range tags.TagSet {
				m[aws.ToString(t.Key)] = aws.ToString(t.Value)
			}
			b.Spec.Tags = export.TagMap(m)
		}

		// Lifecycle rules (also modeled on the standalone S3BucketLifecycle CRD;
		// the bucket spec carries them so a bucket-only export round-trips).
		if lc, err := clients.S3.GetBucketLifecycleConfiguration(ctx, &awss3.GetBucketLifecycleConfigurationInput{Bucket: bucket}); s3IgnoreNotFound(err) == nil && err == nil {
			b.Spec.LifecycleRules = s3LifecycleRules(lc.Rules)
		}

		// CORS.
		if cors, err := clients.S3.GetBucketCors(ctx, &awss3.GetBucketCorsInput{Bucket: bucket}); s3IgnoreNotFound(err) == nil && err == nil {
			b.Spec.CORSRules = s3CORSRules(cors.CORSRules)
		}

		// Notifications (never errors for unset config; returns empty lists).
		if nc, err := clients.S3.GetBucketNotificationConfiguration(ctx, &awss3.GetBucketNotificationConfigurationInput{Bucket: bucket}); err == nil {
			cfg := &awsv1alpha1.S3NotificationConfig{
				EventBridgeEnabled: nc.EventBridgeConfiguration != nil,
			}
			for _, l := range nc.LambdaFunctionConfigurations {
				prefix, suffix := s3FilterPrefixSuffix(l.Filter)
				cfg.LambdaFunctionConfigurations = append(cfg.LambdaFunctionConfigurations, awsv1alpha1.S3LambdaNotification{
					ID:                aws.ToString(l.Id),
					LambdaFunctionARN: aws.ToString(l.LambdaFunctionArn),
					Events:            s3Events(l.Events),
					FilterPrefix:      prefix,
					FilterSuffix:      suffix,
				})
			}
			for _, q := range nc.QueueConfigurations {
				prefix, suffix := s3FilterPrefixSuffix(q.Filter)
				cfg.QueueConfigurations = append(cfg.QueueConfigurations, awsv1alpha1.S3QueueNotification{
					ID:           aws.ToString(q.Id),
					QueueARN:     aws.ToString(q.QueueArn),
					Events:       s3Events(q.Events),
					FilterPrefix: prefix,
					FilterSuffix: suffix,
				})
			}
			for _, t := range nc.TopicConfigurations {
				prefix, suffix := s3FilterPrefixSuffix(t.Filter)
				cfg.TopicConfigurations = append(cfg.TopicConfigurations, awsv1alpha1.S3TopicNotification{
					ID:           aws.ToString(t.Id),
					TopicARN:     aws.ToString(t.TopicArn),
					Events:       s3Events(t.Events),
					FilterPrefix: prefix,
					FilterSuffix: suffix,
				})
			}
			if cfg.EventBridgeEnabled || len(cfg.LambdaFunctionConfigurations) > 0 ||
				len(cfg.QueueConfigurations) > 0 || len(cfg.TopicConfigurations) > 0 {
				b.Spec.NotificationConfig = cfg
			}
		}

		// Website hosting.
		if web, err := clients.S3.GetBucketWebsite(ctx, &awss3.GetBucketWebsiteInput{Bucket: bucket}); s3IgnoreNotFound(err) == nil && err == nil {
			wc := &awsv1alpha1.S3WebsiteConfig{}
			if web.IndexDocument != nil {
				wc.IndexDocument = aws.ToString(web.IndexDocument.Suffix)
			}
			if web.ErrorDocument != nil {
				wc.ErrorDocument = aws.ToString(web.ErrorDocument.Key)
			}
			if web.RedirectAllRequestsTo != nil {
				wc.RedirectAllTo = &awsv1alpha1.S3RedirectAllTo{
					HostName: aws.ToString(web.RedirectAllRequestsTo.HostName),
					Protocol: string(web.RedirectAllRequestsTo.Protocol),
				}
			}
			for _, rr := range web.RoutingRules {
				rule := awsv1alpha1.S3RoutingRule{}
				if rr.Condition != nil {
					rule.Condition = &awsv1alpha1.S3RoutingRuleCondition{
						HttpErrorCodeReturnedEquals: aws.ToString(rr.Condition.HttpErrorCodeReturnedEquals),
						KeyPrefixEquals:             aws.ToString(rr.Condition.KeyPrefixEquals),
					}
				}
				if rr.Redirect != nil {
					rule.Redirect = awsv1alpha1.S3Redirect{
						HostName:             aws.ToString(rr.Redirect.HostName),
						HttpRedirectCode:     aws.ToString(rr.Redirect.HttpRedirectCode),
						Protocol:             string(rr.Redirect.Protocol),
						ReplaceKeyPrefixWith: aws.ToString(rr.Redirect.ReplaceKeyPrefixWith),
						ReplaceKeyWith:       aws.ToString(rr.Redirect.ReplaceKeyWith),
					}
				}
				wc.RoutingRules = append(wc.RoutingRules, rule)
			}
			b.Spec.WebsiteConfig = wc
		}

		// Transfer acceleration (empty status means never configured).
		if acc, err := clients.S3.GetBucketAccelerateConfiguration(ctx, &awss3.GetBucketAccelerateConfigurationInput{Bucket: bucket}); err == nil && acc.Status != "" {
			b.Spec.AccelerateStatus = string(acc.Status)
		}

		// Object Lock.
		if ol, err := clients.S3.GetObjectLockConfiguration(ctx, &awss3.GetObjectLockConfigurationInput{Bucket: bucket}); s3IgnoreNotFound(err) == nil && err == nil {
			if c := ol.ObjectLockConfiguration; c != nil && c.ObjectLockEnabled == s3types.ObjectLockEnabledEnabled {
				olc := &awsv1alpha1.S3ObjectLockConfig{ObjectLockEnabled: true}
				if c.Rule != nil && c.Rule.DefaultRetention != nil {
					olc.Rule = &awsv1alpha1.S3ObjectLockRule{
						DefaultRetention: awsv1alpha1.S3DefaultRetention{
							Mode:  string(c.Rule.DefaultRetention.Mode),
							Days:  c.Rule.DefaultRetention.Days,
							Years: c.Rule.DefaultRetention.Years,
						},
					}
				}
				b.Spec.ObjectLockConfig = olc
			}
		}

		// Access logging.
		if lg, err := clients.S3.GetBucketLogging(ctx, &awss3.GetBucketLoggingInput{Bucket: bucket}); err == nil && lg.LoggingEnabled != nil {
			b.Spec.LoggingConfig = &awsv1alpha1.S3LoggingConfig{
				TargetBucket: aws.ToString(lg.LoggingEnabled.TargetBucket),
				TargetPrefix: aws.ToString(lg.LoggingEnabled.TargetPrefix),
			}
		}

		opts.Index.Add(name, b.Name)
		opts.Index.Add("arn:aws:s3:::"+name, b.Name)
		objs = append(objs, b)
	}
	return objs, nil
}

// s3Events converts typed S3 events to plain strings.
func s3Events(events []s3types.Event) []string {
	var out []string
	for _, e := range events {
		out = append(out, string(e))
	}
	return out
}

// s3FilterPrefixSuffix flattens an S3 notification key filter into the
// prefix/suffix pair the CRD models.
func s3FilterPrefixSuffix(f *s3types.NotificationConfigurationFilter) (prefix, suffix string) {
	if f == nil || f.Key == nil {
		return "", ""
	}
	for _, r := range f.Key.FilterRules {
		switch r.Name {
		case s3types.FilterRuleNamePrefix:
			prefix = aws.ToString(r.Value)
		case s3types.FilterRuleNameSuffix:
			suffix = aws.ToString(r.Value)
		}
	}
	return prefix, suffix
}

// s3LifecycleRules maps SDK lifecycle rules into the CRD shape. The CRD only
// models a prefix filter; tag/size filters cannot be represented and are
// dropped (the rest of the rule is still exported).
func s3LifecycleRules(rules []s3types.LifecycleRule) []awsv1alpha1.S3LifecycleRule {
	var out []awsv1alpha1.S3LifecycleRule
	for _, r := range rules {
		rule := awsv1alpha1.S3LifecycleRule{
			ID:     aws.ToString(r.ID),
			Status: string(r.Status),
			Prefix: aws.ToString(r.Prefix),
		}
		switch f := r.Filter.(type) {
		case *s3types.LifecycleRuleFilterMemberPrefix:
			rule.Prefix = f.Value
		case *s3types.LifecycleRuleFilterMemberAnd:
			rule.Prefix = aws.ToString(f.Value.Prefix)
		}
		if r.Expiration != nil {
			rule.ExpirationDays = r.Expiration.Days
			if r.Expiration.Date != nil {
				rule.ExpirationDate = r.Expiration.Date.Format(time.RFC3339)
			}
		}
		if r.NoncurrentVersionExpiration != nil {
			rule.NoncurrentVersionExpirationDays = r.NoncurrentVersionExpiration.NoncurrentDays
		}
		for _, t := range r.Transitions {
			tr := awsv1alpha1.S3LifecycleTransition{
				Days:         t.Days,
				StorageClass: string(t.StorageClass),
			}
			if t.Date != nil {
				tr.Date = t.Date.Format(time.RFC3339)
			}
			rule.Transitions = append(rule.Transitions, tr)
		}
		for _, t := range r.NoncurrentVersionTransitions {
			rule.NoncurrentVersionTransitions = append(rule.NoncurrentVersionTransitions, awsv1alpha1.S3NoncurrentVersionTransition{
				NoncurrentDays: t.NoncurrentDays,
				StorageClass:   string(t.StorageClass),
			})
		}
		if r.AbortIncompleteMultipartUpload != nil {
			rule.AbortIncompleteMultipartUploadDays = r.AbortIncompleteMultipartUpload.DaysAfterInitiation
		}
		out = append(out, rule)
	}
	return out
}

// s3CORSRules maps SDK CORS rules into the CRD shape.
func s3CORSRules(rules []s3types.CORSRule) []awsv1alpha1.S3CORSRule {
	var out []awsv1alpha1.S3CORSRule
	for _, r := range rules {
		out = append(out, awsv1alpha1.S3CORSRule{
			ID:             aws.ToString(r.ID),
			AllowedHeaders: r.AllowedHeaders,
			AllowedMethods: r.AllowedMethods,
			AllowedOrigins: r.AllowedOrigins,
			ExposeHeaders:  r.ExposeHeaders,
			MaxAgeSeconds:  r.MaxAgeSeconds,
		})
	}
	return out
}

func exportS3BucketPolicies(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	names, err := listOwnedBuckets(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	for _, name := range names {
		out, err := clients.S3.GetBucketPolicy(ctx, &awss3.GetBucketPolicyInput{Bucket: aws.String(name)})
		if err != nil || out.Policy == nil || *out.Policy == "" {
			continue // NoSuchBucketPolicy just means no policy is attached
		}
		bp := &awsv1alpha1.S3BucketPolicy{
			ObjectMeta: export.ObjectMeta(name+"-policy", opts),
			Spec: awsv1alpha1.S3BucketPolicySpec{
				PolicyDocument: *out.Policy,
			},
		}
		if crName, ok := opts.Index.Lookup(name); ok {
			bp.Spec.BucketRef = awsv1alpha1.S3BucketRef{Name: crName}
		} else {
			bp.Spec.BucketRef = awsv1alpha1.S3BucketRef{BucketName: name}
		}
		objs = append(objs, bp)
	}
	return objs, nil
}

func exportS3BucketCORS(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	names, err := listOwnedBuckets(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	for _, name := range names {
		out, err := clients.S3.GetBucketCors(ctx, &awss3.GetBucketCorsInput{Bucket: aws.String(name)})
		if err != nil || len(out.CORSRules) == 0 {
			continue // NoSuchCORSConfiguration just means unset
		}
		c := &awsv1alpha1.S3BucketCORS{
			ObjectMeta: export.ObjectMeta(name+"-cors", opts),
			Spec: awsv1alpha1.S3BucketCORSSpec{
				BucketName: name,
				CORSRules:  s3CORSRules(out.CORSRules),
			},
		}
		objs = append(objs, c)
	}
	return objs, nil
}

func exportS3BucketLifecycles(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	names, err := listOwnedBuckets(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	for _, name := range names {
		out, err := clients.S3.GetBucketLifecycleConfiguration(ctx, &awss3.GetBucketLifecycleConfigurationInput{Bucket: aws.String(name)})
		if err != nil || len(out.Rules) == 0 {
			continue // NoSuchLifecycleConfiguration just means unset
		}
		l := &awsv1alpha1.S3BucketLifecycle{
			ObjectMeta: export.ObjectMeta(name+"-lifecycle", opts),
			Spec: awsv1alpha1.S3BucketLifecycleSpec{
				BucketName: name,
				Rules:      s3LifecycleRules(out.Rules),
			},
		}
		objs = append(objs, l)
	}
	return objs, nil
}

func exportS3BucketNotifications(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	names, err := listOwnedBuckets(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	for _, name := range names {
		nc, err := clients.S3.GetBucketNotificationConfiguration(ctx, &awss3.GetBucketNotificationConfigurationInput{Bucket: aws.String(name)})
		if err != nil {
			continue
		}
		n := &awsv1alpha1.S3BucketNotification{
			ObjectMeta: export.ObjectMeta(name+"-notification", opts),
			Spec: awsv1alpha1.S3BucketNotificationSpec{
				BucketName:         name,
				EventBridgeEnabled: nc.EventBridgeConfiguration != nil,
			},
		}
		for _, l := range nc.LambdaFunctionConfigurations {
			// The standalone notification CRD has no filter fields; prefix/
			// suffix filters are only representable on the S3Bucket spec.
			n.Spec.LambdaFunctionConfigurations = append(n.Spec.LambdaFunctionConfigurations, awsv1alpha1.S3LambdaNotificationConfig{
				ID:                aws.ToString(l.Id),
				LambdaFunctionARN: aws.ToString(l.LambdaFunctionArn),
				Events:            s3Events(l.Events),
			})
		}
		for _, t := range nc.TopicConfigurations {
			n.Spec.TopicConfigurations = append(n.Spec.TopicConfigurations, awsv1alpha1.S3TopicNotificationConfig{
				ID:       aws.ToString(t.Id),
				TopicARN: aws.ToString(t.TopicArn),
				Events:   s3Events(t.Events),
			})
		}
		for _, q := range nc.QueueConfigurations {
			n.Spec.QueueConfigurations = append(n.Spec.QueueConfigurations, awsv1alpha1.S3QueueNotificationConfig{
				ID:       aws.ToString(q.Id),
				QueueARN: aws.ToString(q.QueueArn),
				Events:   s3Events(q.Events),
			})
		}
		if !n.Spec.EventBridgeEnabled && len(n.Spec.LambdaFunctionConfigurations) == 0 &&
			len(n.Spec.TopicConfigurations) == 0 && len(n.Spec.QueueConfigurations) == 0 {
			continue // nothing configured
		}
		objs = append(objs, n)
	}
	return objs, nil
}

func exportS3BucketReplications(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	names, err := listOwnedBuckets(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	for _, name := range names {
		out, err := clients.S3.GetBucketReplication(ctx, &awss3.GetBucketReplicationInput{Bucket: aws.String(name)})
		if err != nil || out.ReplicationConfiguration == nil {
			continue // ReplicationConfigurationNotFoundError just means unset
		}
		cfg := out.ReplicationConfiguration
		r := &awsv1alpha1.S3BucketReplication{
			ObjectMeta: export.ObjectMeta(name+"-replication", opts),
			Spec: awsv1alpha1.S3BucketReplicationSpec{
				BucketName: name,
				RoleARN:    aws.ToString(cfg.Role),
			},
		}
		for _, rr := range cfg.Rules {
			rule := awsv1alpha1.S3ReplicationRule{
				ID:       aws.ToString(rr.ID),
				Status:   string(rr.Status),
				Prefix:   aws.ToString(rr.Prefix),
				Priority: rr.Priority,
			}
			// The CRD only models a prefix filter; tag filters are dropped.
			switch f := rr.Filter.(type) {
			case *s3types.ReplicationRuleFilterMemberPrefix:
				rule.Prefix = f.Value
			case *s3types.ReplicationRuleFilterMemberAnd:
				rule.Prefix = aws.ToString(f.Value.Prefix)
			}
			if rr.Destination != nil {
				rule.Destination = awsv1alpha1.S3ReplicationDestination{
					Bucket:       aws.ToString(rr.Destination.Bucket),
					StorageClass: string(rr.Destination.StorageClass),
					Account:      aws.ToString(rr.Destination.Account),
				}
			}
			r.Spec.Rules = append(r.Spec.Rules, rule)
		}
		if len(r.Spec.Rules) == 0 {
			continue
		}
		objs = append(objs, r)
	}
	return objs, nil
}

func exportEFSFileSystems(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsefs.NewDescribeFileSystemsPaginator(clients.EFS, &awsefs.DescribeFileSystemsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe file systems: %w", err)
		}
		for _, fs := range page.FileSystems {
			id := aws.ToString(fs.FileSystemId)
			name := id
			m := map[string]string{}
			for _, t := range fs.Tags {
				k, v := aws.ToString(t.Key), aws.ToString(t.Value)
				m[k] = v
				if k == "Name" && v != "" {
					name = v
				}
			}
			f := &awsv1alpha1.EFSFileSystem{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.EFSFileSystemSpec{
					PerformanceMode:              string(fs.PerformanceMode),
					ThroughputMode:               string(fs.ThroughputMode),
					ProvisionedThroughputInMibps: fs.ProvisionedThroughputInMibps,
					Encrypted:                    aws.ToBool(fs.Encrypted),
					KMSKeyID:                     aws.ToString(fs.KmsKeyId),
					Tags:                         export.TagMap(m),
				},
			}
			opts.Index.Add(id, f.Name)
			opts.Index.Add(aws.ToString(fs.FileSystemArn), f.Name)
			objs = append(objs, f)
		}
	}
	return objs, nil
}

func exportEFSMountTargets(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	fsp := awsefs.NewDescribeFileSystemsPaginator(clients.EFS, &awsefs.DescribeFileSystemsInput{})
	for fsp.HasMorePages() {
		fspage, err := fsp.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe file systems: %w", err)
		}
		for _, fs := range fspage.FileSystems {
			mtp := awsefs.NewDescribeMountTargetsPaginator(clients.EFS, &awsefs.DescribeMountTargetsInput{
				FileSystemId: fs.FileSystemId,
			})
			for mtp.HasMorePages() {
				page, err := mtp.NextPage(ctx)
				if err != nil {
					break // per-filesystem errors: skip and keep exporting
				}
				for _, mt := range page.MountTargets {
					mtID := aws.ToString(mt.MountTargetId)
					m := &awsv1alpha1.EFSMountTarget{
						ObjectMeta: export.ObjectMeta(mtID, opts),
						Spec: awsv1alpha1.EFSMountTargetSpec{
							FileSystemID: aws.ToString(mt.FileSystemId),
							SubnetID:     aws.ToString(mt.SubnetId),
							IPAddress:    aws.ToString(mt.IpAddress),
						},
					}
					// Security groups are a separate call; tolerate errors.
					if sgs, err := clients.EFS.DescribeMountTargetSecurityGroups(ctx, &awsefs.DescribeMountTargetSecurityGroupsInput{
						MountTargetId: mt.MountTargetId,
					}); err == nil {
						m.Spec.SecurityGroups = sgs.SecurityGroups
					}
					opts.Index.Add(mtID, m.Name)
					objs = append(objs, m)
				}
			}
		}
	}
	return objs, nil
}

func exportEFSAccessPoints(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsefs.NewDescribeAccessPointsPaginator(clients.EFS, &awsefs.DescribeAccessPointsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe access points: %w", err)
		}
		for _, ap := range page.AccessPoints {
			id := aws.ToString(ap.AccessPointId)
			name := id
			m := map[string]string{}
			for _, t := range ap.Tags {
				k, v := aws.ToString(t.Key), aws.ToString(t.Value)
				m[k] = v
				if k == "Name" && v != "" {
					name = v
				}
			}
			a := &awsv1alpha1.EFSAccessPoint{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.EFSAccessPointSpec{
					FileSystemID: aws.ToString(ap.FileSystemId),
					Tags:         export.TagMap(m),
				},
			}
			if ap.PosixUser != nil {
				a.Spec.PosixUser = &awsv1alpha1.EFSPosixUser{
					UID:           aws.ToInt64(ap.PosixUser.Uid),
					GID:           aws.ToInt64(ap.PosixUser.Gid),
					SecondaryGIDs: ap.PosixUser.SecondaryGids,
				}
			}
			if ap.RootDirectory != nil {
				rd := &awsv1alpha1.EFSRootDirectory{Path: aws.ToString(ap.RootDirectory.Path)}
				if ci := ap.RootDirectory.CreationInfo; ci != nil {
					rd.CreationInfo = &awsv1alpha1.EFSCreationInfo{
						OwnerUID:    aws.ToInt64(ci.OwnerUid),
						OwnerGID:    aws.ToInt64(ci.OwnerGid),
						Permissions: aws.ToString(ci.Permissions),
					}
				}
				a.Spec.RootDirectory = rd
			}
			opts.Index.Add(id, a.Name)
			opts.Index.Add(aws.ToString(ap.AccessPointArn), a.Name)
			objs = append(objs, a)
		}
	}
	return objs, nil
}

func exportECRRepositories(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsecr.NewDescribeRepositoriesPaginator(clients.ECR, &awsecr.DescribeRepositoriesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe repositories: %w", err)
		}
		for _, r := range page.Repositories {
			name := aws.ToString(r.RepositoryName)
			repo := &awsv1alpha1.ECRRepository{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.ECRRepositorySpec{
					RepositoryName:     name,
					ImageTagMutability: string(r.ImageTagMutability),
				},
			}
			if r.ImageScanningConfiguration != nil {
				repo.Spec.ScanOnPush = r.ImageScanningConfiguration.ScanOnPush
			}
			if r.EncryptionConfiguration != nil {
				repo.Spec.EncryptionType = string(r.EncryptionConfiguration.EncryptionType)
				repo.Spec.KMSKeyARN = aws.ToString(r.EncryptionConfiguration.KmsKey)
			}
			if tags, err := clients.ECR.ListTagsForResource(ctx, &awsecr.ListTagsForResourceInput{
				ResourceArn: r.RepositoryArn,
			}); err == nil {
				m := make(map[string]string, len(tags.Tags))
				for _, t := range tags.Tags {
					m[aws.ToString(t.Key)] = aws.ToString(t.Value)
				}
				repo.Spec.Tags = export.TagMap(m)
			}
			opts.Index.Add(aws.ToString(r.RepositoryArn), repo.Name)
			opts.Index.Add(name, repo.Name)
			objs = append(objs, repo)
		}
	}
	return objs, nil
}

func exportECRLifecyclePolicies(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsecr.NewDescribeRepositoriesPaginator(clients.ECR, &awsecr.DescribeRepositoriesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe repositories: %w", err)
		}
		for _, r := range page.Repositories {
			name := aws.ToString(r.RepositoryName)
			out, err := clients.ECR.GetLifecyclePolicy(ctx, &awsecr.GetLifecyclePolicyInput{
				RepositoryName: r.RepositoryName,
			})
			if err != nil {
				// LifecyclePolicyNotFoundException just means no policy is set.
				var nf *ecrtypes.LifecyclePolicyNotFoundException
				if errors.As(err, &nf) {
					continue
				}
				continue // per-repo errors: skip
			}
			if out.LifecyclePolicyText == nil || *out.LifecyclePolicyText == "" {
				continue
			}
			lp := &awsv1alpha1.ECRLifecyclePolicy{
				ObjectMeta: export.ObjectMeta(name+"-lifecycle-policy", opts),
				Spec: awsv1alpha1.ECRLifecyclePolicySpec{
					LifecyclePolicyDocument: *out.LifecyclePolicyText,
				},
			}
			if crName, ok := opts.Index.Lookup(name); ok {
				lp.Spec.RepositoryRef = awsv1alpha1.ECRRepositoryRef{Name: crName}
			} else {
				lp.Spec.RepositoryRef = awsv1alpha1.ECRRepositoryRef{RepositoryName: name}
			}
			objs = append(objs, lp)
		}
	}
	return objs, nil
}

func exportECRRepositoryPolicies(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsecr.NewDescribeRepositoriesPaginator(clients.ECR, &awsecr.DescribeRepositoriesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe repositories: %w", err)
		}
		for _, r := range page.Repositories {
			name := aws.ToString(r.RepositoryName)
			out, err := clients.ECR.GetRepositoryPolicy(ctx, &awsecr.GetRepositoryPolicyInput{
				RepositoryName: r.RepositoryName,
			})
			if err != nil {
				// RepositoryPolicyNotFoundException just means no policy is set.
				var nf *ecrtypes.RepositoryPolicyNotFoundException
				if errors.As(err, &nf) {
					continue
				}
				continue // per-repo errors: skip
			}
			if out.PolicyText == nil || *out.PolicyText == "" {
				continue
			}
			rp := &awsv1alpha1.ECRRepositoryPolicy{
				ObjectMeta: export.ObjectMeta(name+"-repository-policy", opts),
				Spec: awsv1alpha1.ECRRepositoryPolicySpec{
					PolicyDocument: *out.PolicyText,
				},
			}
			if crName, ok := opts.Index.Lookup(name); ok {
				rp.Spec.RepositoryRef = awsv1alpha1.ECRRepositoryRef{Name: crName}
			} else {
				rp.Spec.RepositoryRef = awsv1alpha1.ECRRepositoryRef{RepositoryName: name}
			}
			objs = append(objs, rp)
		}
	}
	return objs, nil
}
