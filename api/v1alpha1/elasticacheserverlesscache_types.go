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

// ElastiCacheServerlessCacheSpec defines the desired state of an ElastiCache Serverless Cache.
type ElastiCacheServerlessCacheSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ServerlessCacheName is the unique name for the serverless cache.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverlessCacheName is immutable"
	ServerlessCacheName string `json:"serverlessCacheName"`

	// Engine is the cache engine (valkey or redis).
	// +kubebuilder:validation:Enum=redis;valkey
	Engine string `json:"engine"`

	// Description is an optional description.
	// +optional
	Description string `json:"description,omitempty"`

	// KMSKeyID is the ARN or ID of the KMS key for encryption.
	// +optional
	KMSKeyID string `json:"kmsKeyId,omitempty"`

	// SecurityGroupIDs are the VPC security group IDs.
	// +optional
	SecurityGroupIDs []string `json:"securityGroupIds,omitempty"`

	// SubnetIDs are the subnet IDs for the cache.
	// +optional
	SubnetIDs []string `json:"subnetIds,omitempty"`

	// SnapshotRetentionLimit is the number of days to retain backups.
	// +optional
	SnapshotRetentionLimit *int32 `json:"snapshotRetentionLimit,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ElastiCacheServerlessCacheStatus defines the observed state of ElastiCacheServerlessCache.
type ElastiCacheServerlessCacheStatus struct {
	// ARN is the ARN of the serverless cache.
	// +optional
	ARN string `json:"arn,omitempty"`

	// Endpoint is the cache endpoint.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// Status is the current state of the cache.
	// +optional
	Status string `json:"status,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.arn"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ElastiCacheServerlessCache is the Schema for managing ElastiCache serverless caches.
type ElastiCacheServerlessCache struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ElastiCacheServerlessCacheSpec   `json:"spec,omitempty"`
	Status ElastiCacheServerlessCacheStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ElastiCacheServerlessCacheList contains a list of ElastiCacheServerlessCache.
type ElastiCacheServerlessCacheList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ElastiCacheServerlessCache `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ElastiCacheServerlessCache{}, &ElastiCacheServerlessCacheList{})
}
