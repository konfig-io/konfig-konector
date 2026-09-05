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

// CloudFrontCachePolicySpec defines the desired state of a CloudFront Cache Policy.
type CloudFrontCachePolicySpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the unique name of the cache policy.
	Name string `json:"name"`

	// MinTTL is the minimum time-to-live in seconds for cached objects.
	MinTTL int64 `json:"minTtl"`

	// DefaultTTL is the default TTL in seconds.
	// +optional
	DefaultTTL *int64 `json:"defaultTtl,omitempty"`

	// MaxTTL is the maximum TTL in seconds.
	// +optional
	MaxTTL *int64 `json:"maxTtl,omitempty"`

	// Comment is an optional description.
	// +optional
	Comment string `json:"comment,omitempty"`
}

// CloudFrontCachePolicyStatus defines the observed state of CloudFrontCachePolicy.
type CloudFrontCachePolicyStatus struct {
	// PolicyID is the ID of the cache policy.
	// +optional
	PolicyID string `json:"policyId,omitempty"`

	// ETag is the ETag for the cache policy.
	// +optional
	ETag string `json:"etag,omitempty"`

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
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.policyId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CloudFrontCachePolicy is the Schema for managing CloudFront cache policies.
type CloudFrontCachePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudFrontCachePolicySpec   `json:"spec,omitempty"`
	Status CloudFrontCachePolicyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CloudFrontCachePolicyList contains a list of CloudFrontCachePolicy.
type CloudFrontCachePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudFrontCachePolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudFrontCachePolicy{}, &CloudFrontCachePolicyList{})
}
