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

// S3ReplicationDestination defines the destination for a replication rule.
type S3ReplicationDestination struct {
	// Bucket is the ARN of the destination bucket.
	// +kubebuilder:validation:MinLength=1
	Bucket string `json:"bucket"`

	// StorageClass overrides the storage class of replicated objects.
	// +optional
	StorageClass string `json:"storageClass,omitempty"`

	// Account is the destination account ID for cross-account replication.
	// +optional
	Account string `json:"account,omitempty"`
}

// S3ReplicationRule defines a replication rule.
type S3ReplicationRule struct {
	// ID is an optional unique identifier for this rule.
	// +optional
	ID string `json:"id,omitempty"`

	// Status indicates whether the rule is enabled.
	// +kubebuilder:validation:Enum=Enabled;Disabled
	Status string `json:"status"`

	// Prefix filters objects for replication by key prefix.
	// +optional
	Prefix string `json:"prefix,omitempty"`

	// Destination is the destination configuration for the rule.
	Destination S3ReplicationDestination `json:"destination"`

	// Priority is used when multiple rules apply to the same object.
	// +optional
	Priority *int32 `json:"priority,omitempty"`
}

// S3BucketReplicationSpec defines the desired state of S3BucketReplication.
type S3BucketReplicationSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// BucketName is the name of the source S3 bucket.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="bucketName is immutable"
	BucketName string `json:"bucketName"`

	// RoleARN is the IAM role ARN that S3 assumes for replication.
	// +kubebuilder:validation:MinLength=1
	RoleARN string `json:"roleArn"`

	// Rules are the replication rules.
	// +kubebuilder:validation:MinItems=1
	Rules []S3ReplicationRule `json:"rules"`
}

// S3BucketReplicationStatus defines the observed state of S3BucketReplication.
type S3BucketReplicationStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
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
// +kubebuilder:printcolumn:name="Bucket",type="string",JSONPath=".spec.bucketName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// S3BucketReplication is the Schema for managing S3 bucket replication configurations.
type S3BucketReplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   S3BucketReplicationSpec   `json:"spec,omitempty"`
	Status S3BucketReplicationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// S3BucketReplicationList contains a list of S3BucketReplication.
type S3BucketReplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []S3BucketReplication `json:"items"`
}

func init() {
	SchemeBuilder.Register(&S3BucketReplication{}, &S3BucketReplicationList{})
}
