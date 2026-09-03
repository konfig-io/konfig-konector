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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// S3BlockPublicAccess configures the S3 Block Public Access settings.
type S3BlockPublicAccess struct {
	// BlockPublicAcls blocks public ACLs.
	// +optional
	BlockPublicAcls bool `json:"blockPublicAcls,omitempty"`
	// BlockPublicPolicy blocks public bucket policies.
	// +optional
	BlockPublicPolicy bool `json:"blockPublicPolicy,omitempty"`
	// IgnorePublicAcls ignores existing public ACLs.
	// +optional
	IgnorePublicAcls bool `json:"ignorePublicAcls,omitempty"`
	// RestrictPublicBuckets restricts public bucket access.
	// +optional
	RestrictPublicBuckets bool `json:"restrictPublicBuckets,omitempty"`
}

// S3LifecycleTransition defines a storage class transition.
type S3LifecycleTransition struct {
	// Days is the number of days after object creation.
	// +optional
	Days *int32 `json:"days,omitempty"`
	// Date is an ISO 8601 date for the transition.
	// +optional
	Date string `json:"date,omitempty"`
	// StorageClass is the target storage class (e.g. STANDARD_IA, GLACIER).
	StorageClass string `json:"storageClass"`
}

// S3NoncurrentVersionTransition defines a storage class transition for noncurrent versions.
type S3NoncurrentVersionTransition struct {
	// NoncurrentDays is the number of days before transitioning.
	// +optional
	NoncurrentDays *int32 `json:"noncurrentDays,omitempty"`
	// StorageClass is the target storage class.
	StorageClass string `json:"storageClass"`
}

// S3LifecycleRule defines a single S3 lifecycle rule.
type S3LifecycleRule struct {
	// ID is a unique identifier for the rule.
	// +optional
	ID string `json:"id,omitempty"`
	// Status is Enabled or Disabled.
	// +kubebuilder:validation:Enum=Enabled;Disabled
	Status string `json:"status"`
	// Prefix is the object key prefix to apply the rule to.
	// +optional
	Prefix string `json:"prefix,omitempty"`
	// ExpirationDays is the number of days before objects expire.
	// +optional
	ExpirationDays *int32 `json:"expirationDays,omitempty"`
	// ExpirationDate is the ISO 8601 date for expiration.
	// +optional
	ExpirationDate string `json:"expirationDate,omitempty"`
	// NoncurrentVersionExpirationDays is the number of days before noncurrent versions expire.
	// +optional
	NoncurrentVersionExpirationDays *int32 `json:"noncurrentVersionExpirationDays,omitempty"`
	// Transitions defines storage class transitions.
	// +optional
	Transitions []S3LifecycleTransition `json:"transitions,omitempty"`
	// NoncurrentVersionTransitions defines storage class transitions for noncurrent versions.
	// +optional
	NoncurrentVersionTransitions []S3NoncurrentVersionTransition `json:"noncurrentVersionTransitions,omitempty"`
	// AbortIncompleteMultipartUploadDays cancels incomplete multipart uploads after this many days.
	// +optional
	AbortIncompleteMultipartUploadDays *int32 `json:"abortIncompleteMultipartUploadDays,omitempty"`
}

// S3CORSRule defines a CORS rule for an S3 bucket.
type S3CORSRule struct {
	// ID is an optional unique identifier for the rule.
	// +optional
	ID string `json:"id,omitempty"`
	// AllowedHeaders specifies which headers are allowed in a pre-flight request.
	// +optional
	AllowedHeaders []string `json:"allowedHeaders,omitempty"`
	// AllowedMethods specifies the HTTP methods allowed (GET, PUT, POST, DELETE, HEAD).
	AllowedMethods []string `json:"allowedMethods"`
	// AllowedOrigins specifies the origins that are allowed.
	AllowedOrigins []string `json:"allowedOrigins"`
	// ExposeHeaders specifies headers the browser can access.
	// +optional
	ExposeHeaders []string `json:"exposeHeaders,omitempty"`
	// MaxAgeSeconds specifies how long the browser can cache the preflight response.
	// +optional
	MaxAgeSeconds *int32 `json:"maxAgeSeconds,omitempty"`
}

// S3LambdaNotification configures a Lambda function notification.
type S3LambdaNotification struct {
	// ID is an optional unique identifier.
	// +optional
	ID string `json:"id,omitempty"`
	// LambdaFunctionARN is the ARN of the Lambda function.
	LambdaFunctionARN string `json:"lambdaFunctionArn"`
	// Events is the list of S3 event types that trigger the notification.
	Events []string `json:"events"`
	// FilterPrefix restricts notifications to keys with this prefix.
	// +optional
	FilterPrefix string `json:"filterPrefix,omitempty"`
	// FilterSuffix restricts notifications to keys with this suffix.
	// +optional
	FilterSuffix string `json:"filterSuffix,omitempty"`
}

// S3QueueNotification configures an SQS queue notification.
type S3QueueNotification struct {
	// ID is an optional unique identifier.
	// +optional
	ID string `json:"id,omitempty"`
	// QueueARN is the ARN of the SQS queue.
	QueueARN string `json:"queueArn"`
	// Events is the list of S3 event types that trigger the notification.
	Events []string `json:"events"`
	// FilterPrefix restricts notifications to keys with this prefix.
	// +optional
	FilterPrefix string `json:"filterPrefix,omitempty"`
	// FilterSuffix restricts notifications to keys with this suffix.
	// +optional
	FilterSuffix string `json:"filterSuffix,omitempty"`
}

// S3TopicNotification configures an SNS topic notification.
type S3TopicNotification struct {
	// ID is an optional unique identifier.
	// +optional
	ID string `json:"id,omitempty"`
	// TopicARN is the ARN of the SNS topic.
	TopicARN string `json:"topicArn"`
	// Events is the list of S3 event types that trigger the notification.
	Events []string `json:"events"`
	// FilterPrefix restricts notifications to keys with this prefix.
	// +optional
	FilterPrefix string `json:"filterPrefix,omitempty"`
	// FilterSuffix restricts notifications to keys with this suffix.
	// +optional
	FilterSuffix string `json:"filterSuffix,omitempty"`
}

// S3NotificationConfig defines bucket event notifications.
type S3NotificationConfig struct {
	// LambdaFunctionConfigurations configures Lambda function notifications.
	// +optional
	LambdaFunctionConfigurations []S3LambdaNotification `json:"lambdaFunctionConfigurations,omitempty"`
	// QueueConfigurations configures SQS queue notifications.
	// +optional
	QueueConfigurations []S3QueueNotification `json:"queueConfigurations,omitempty"`
	// TopicConfigurations configures SNS topic notifications.
	// +optional
	TopicConfigurations []S3TopicNotification `json:"topicConfigurations,omitempty"`
	// EventBridgeEnabled enables routing events to Amazon EventBridge.
	// +optional
	EventBridgeEnabled bool `json:"eventBridgeEnabled,omitempty"`
}

// S3RedirectAllTo configures redirect of all requests to another host.
type S3RedirectAllTo struct {
	// HostName is the hostname to redirect to.
	HostName string `json:"hostName"`
	// Protocol is http or https.
	// +optional
	Protocol string `json:"protocol,omitempty"`
}

// S3RoutingRuleCondition defines the condition for a routing rule.
type S3RoutingRuleCondition struct {
	// HttpErrorCodeReturnedEquals triggers the rule when this HTTP error code is returned.
	// +optional
	HttpErrorCodeReturnedEquals string `json:"httpErrorCodeReturnedEquals,omitempty"`
	// KeyPrefixEquals triggers the rule when the key prefix matches.
	// +optional
	KeyPrefixEquals string `json:"keyPrefixEquals,omitempty"`
}

// S3Redirect defines the redirect target for a routing rule.
type S3Redirect struct {
	// HostName replaces the hostname in the redirect.
	// +optional
	HostName string `json:"hostName,omitempty"`
	// HttpRedirectCode is the HTTP redirect code (default 301).
	// +optional
	HttpRedirectCode string `json:"httpRedirectCode,omitempty"`
	// Protocol is http or https.
	// +optional
	Protocol string `json:"protocol,omitempty"`
	// ReplaceKeyPrefixWith replaces the key prefix in the redirect.
	// +optional
	ReplaceKeyPrefixWith string `json:"replaceKeyPrefixWith,omitempty"`
	// ReplaceKeyWith replaces the entire key in the redirect.
	// +optional
	ReplaceKeyWith string `json:"replaceKeyWith,omitempty"`
}

// S3RoutingRule defines a routing rule for website configuration.
type S3RoutingRule struct {
	// Condition is the condition that triggers this rule.
	// +optional
	Condition *S3RoutingRuleCondition `json:"condition,omitempty"`
	// Redirect defines where to redirect matching requests.
	Redirect S3Redirect `json:"redirect"`
}

// S3WebsiteConfig configures static website hosting.
type S3WebsiteConfig struct {
	// IndexDocument is the suffix for the index document (e.g. index.html).
	// +optional
	IndexDocument string `json:"indexDocument,omitempty"`
	// ErrorDocument is the key name used for 4XX errors.
	// +optional
	ErrorDocument string `json:"errorDocument,omitempty"`
	// RedirectAllTo redirects all requests to another hostname.
	// +optional
	RedirectAllTo *S3RedirectAllTo `json:"redirectAllTo,omitempty"`
	// RoutingRules defines conditional redirect rules.
	// +optional
	RoutingRules []S3RoutingRule `json:"routingRules,omitempty"`
}

// S3DefaultRetention defines the default object lock retention settings.
type S3DefaultRetention struct {
	// Mode is GOVERNANCE or COMPLIANCE.
	// +kubebuilder:validation:Enum=GOVERNANCE;COMPLIANCE
	Mode string `json:"mode"`
	// Days is the retention period in days.
	// +optional
	Days *int32 `json:"days,omitempty"`
	// Years is the retention period in years.
	// +optional
	Years *int32 `json:"years,omitempty"`
}

// S3ObjectLockRule defines the default retention rule.
type S3ObjectLockRule struct {
	// DefaultRetention is the default retention settings.
	DefaultRetention S3DefaultRetention `json:"defaultRetention"`
}

// S3ObjectLockConfig configures S3 Object Lock.
type S3ObjectLockConfig struct {
	// ObjectLockEnabled enables Object Lock. Immutable after creation.
	ObjectLockEnabled bool `json:"objectLockEnabled"`
	// Rule is the default retention rule.
	// +optional
	Rule *S3ObjectLockRule `json:"rule,omitempty"`
}

// S3LoggingConfig configures S3 server access logging.
type S3LoggingConfig struct {
	// TargetBucket is the bucket that receives access logs.
	TargetBucket string `json:"targetBucket"`
	// TargetPrefix is the prefix for log object keys.
	// +optional
	TargetPrefix string `json:"targetPrefix,omitempty"`
}

// S3BucketEncryption defines server-side encryption configuration.
type S3BucketEncryption struct {
	// SSEAlgorithm is the server-side encryption algorithm: AES256 or aws:kms.
	// +kubebuilder:validation:MinLength=1
	SSEAlgorithm string `json:"sseAlgorithm"`

	// KMSKeyID is the KMS key ARN or ID (only for aws:kms).
	// +optional
	KMSKeyID string `json:"kmsKeyId,omitempty"`
}

// S3BucketSpec defines the desired state of an S3 Bucket.
type S3BucketSpec struct {
	// BucketName is the globally unique name of the bucket. Immutable after creation.
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="bucketName is immutable"
	BucketName string `json:"bucketName"`

	// Region is the AWS region where the bucket is created.
	// Defaults to the operator's region if not specified.
	// +optional
	Region string `json:"region,omitempty"`

	// Versioning enables S3 versioning on the bucket.
	// +optional
	Versioning bool `json:"versioning,omitempty"`

	// ServerSideEncryption configures default SSE for new objects.
	// +optional
	ServerSideEncryption *S3BucketEncryption `json:"serverSideEncryption,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`

	// BlockPublicAccess configures S3 Block Public Access settings.
	// +optional
	BlockPublicAccess *S3BlockPublicAccess `json:"blockPublicAccess,omitempty"`

	// LifecycleRules configures lifecycle management for objects in the bucket.
	// +optional
	LifecycleRules []S3LifecycleRule `json:"lifecycleRules,omitempty"`

	// CORSRules configures cross-origin resource sharing.
	// +optional
	CORSRules []S3CORSRule `json:"corsRules,omitempty"`

	// NotificationConfig configures event notifications.
	// +optional
	NotificationConfig *S3NotificationConfig `json:"notificationConfig,omitempty"`

	// WebsiteConfig configures static website hosting.
	// +optional
	WebsiteConfig *S3WebsiteConfig `json:"websiteConfig,omitempty"`

	// AccelerateStatus enables S3 Transfer Acceleration: Enabled or Suspended.
	// +kubebuilder:validation:Enum=Enabled;Suspended
	// +optional
	AccelerateStatus string `json:"accelerateStatus,omitempty"`

	// ObjectLockConfig configures S3 Object Lock. ObjectLockEnabled is immutable after creation.
	// +optional
	ObjectLockConfig *S3ObjectLockConfig `json:"objectLockConfig,omitempty"`

	// LoggingConfig configures S3 server access logging.
	// +optional
	LoggingConfig *S3LoggingConfig `json:"loggingConfig,omitempty"`
}

// S3BucketStatus defines the observed state of S3Bucket.
type S3BucketStatus struct {
	// ARN is the Amazon Resource Name of the bucket.
	// +optional
	ARN string `json:"arn,omitempty"`

	// DomainName is the bucket's regional domain name.
	// +optional
	DomainName string `json:"domainName,omitempty"`

	// Conditions describe the current state of the resource.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent .metadata.generation reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastSyncTime is when the resource was last successfully reconciled.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// WebsiteEndpoint is the website endpoint URL when website hosting is enabled.
	// +optional
	WebsiteEndpoint string `json:"websiteEndpoint,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Bucket",type="string",JSONPath=".spec.bucketName"
// +kubebuilder:printcolumn:name="Region",type="string",JSONPath=".spec.region"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// S3Bucket is the Schema for managing S3 Buckets.
type S3Bucket struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   S3BucketSpec   `json:"spec,omitempty"`
	Status S3BucketStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// S3BucketList contains a list of S3Bucket
type S3BucketList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []S3Bucket `json:"items"`
}

func init() {
	SchemeBuilder.Register(&S3Bucket{}, &S3BucketList{})
}
