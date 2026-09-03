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

// TrailSpec defines the desired state of a CloudTrail trail.
type TrailSpec struct {
	// TrailName is the name of the trail. Immutable after creation.
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="trailName is immutable"
	TrailName string `json:"trailName"`

	// S3BucketName is the S3 bucket designated for publishing log files.
	// +kubebuilder:validation:MinLength=1
	S3BucketName string `json:"s3BucketName"`

	// S3KeyPrefix is the S3 key prefix that follows the bucket name.
	// +optional
	S3KeyPrefix string `json:"s3KeyPrefix,omitempty"`

	// IncludeGlobalServiceEvents publishes events from global services such as IAM.
	// +optional
	IncludeGlobalServiceEvents bool `json:"includeGlobalServiceEvents,omitempty"`

	// IsMultiRegionTrail creates the trail in all Regions.
	// +optional
	IsMultiRegionTrail bool `json:"isMultiRegionTrail,omitempty"`

	// EnableLogFileValidation enables log file integrity validation.
	// +optional
	EnableLogFileValidation bool `json:"enableLogFileValidation,omitempty"`

	// CloudWatchLogsLogGroupArn is the log group ARN to which CloudTrail logs
	// are delivered. Requires CloudWatchLogsRoleArn.
	// +optional
	CloudWatchLogsLogGroupArn string `json:"cloudWatchLogsLogGroupArn,omitempty"`

	// CloudWatchLogsRoleArn is the role for the CloudWatch Logs endpoint to
	// assume to write to the log group.
	// +optional
	CloudWatchLogsRoleArn string `json:"cloudWatchLogsRoleArn,omitempty"`

	// KMSKeyID is the KMS key ID/ARN/alias used to encrypt delivered logs.
	// +optional
	KMSKeyID string `json:"kmsKeyId,omitempty"`

	// EnableLogging starts (true) or stops (false) recording of API calls.
	// +optional
	// +kubebuilder:default=true
	EnableLogging *bool `json:"enableLogging,omitempty"`

	// Tags are AWS resource tags to apply to the trail.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// TrailStatus defines the observed state of Trail.
type TrailStatus struct {
	// TrailARN is the ARN of the trail.
	// +optional
	TrailARN string `json:"trailArn,omitempty"`

	// IsLogging reports whether the trail is currently logging.
	// +optional
	IsLogging bool `json:"isLogging,omitempty"`

	// Conditions describe the current state of the resource.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent .metadata.generation reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastSyncTime is when the resource was last successfully reconciled.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Trail-ARN",type="string",JSONPath=".status.trailArn"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Trail is the Schema for managing CloudTrail trails.
type Trail struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TrailSpec   `json:"spec,omitempty"`
	Status TrailStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TrailList contains a list of Trail
type TrailList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Trail `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Trail{}, &TrailList{})
}
