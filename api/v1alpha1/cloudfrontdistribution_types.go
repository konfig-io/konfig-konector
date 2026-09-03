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

// CFOrigin defines a CloudFront distribution origin.
type CFOrigin struct {
	// ID is the unique identifier for the origin.
	ID string `json:"id"`

	// DomainName is the DNS domain name of the origin.
	DomainName string `json:"domainName"`

	// OriginPath is an optional element that causes CloudFront to request content from a specific directory.
	// +optional
	OriginPath string `json:"originPath,omitempty"`
}

// CFDefaultCacheBehavior defines the default cache behavior for a CloudFront distribution.
type CFDefaultCacheBehavior struct {
	// TargetOriginID specifies the value of the ID for the origin that CloudFront routes requests to.
	TargetOriginID string `json:"targetOriginId"`

	// ViewerProtocolPolicy defines how you want CloudFront to serve requests.
	// +kubebuilder:validation:Enum=allow-all;https-only;redirect-to-https
	ViewerProtocolPolicy string `json:"viewerProtocolPolicy"`

	// CachePolicyID is the ID of the cache policy attached to the default cache behavior.
	// +optional
	CachePolicyID string `json:"cachePolicyId,omitempty"`

	// AllowedMethods are the HTTP methods CloudFront processes and forwards to the origin.
	// +optional
	AllowedMethods []string `json:"allowedMethods,omitempty"`
}

// CloudFrontDistributionSpec defines the desired state of a CloudFront Distribution.
type CloudFrontDistributionSpec struct {
	// Origins is the list of origins for this distribution.
	// +kubebuilder:validation:MinItems=1
	Origins []CFOrigin `json:"origins"`

	// DefaultCacheBehavior is the default cache behavior configuration.
	DefaultCacheBehavior CFDefaultCacheBehavior `json:"defaultCacheBehavior"`

	// Comment is a description for the distribution.
	// +optional
	Comment string `json:"comment,omitempty"`

	// Enabled controls whether the distribution is enabled.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Aliases are the CNAMEs (alternate domain names) for this distribution.
	// +optional
	Aliases []string `json:"aliases,omitempty"`

	// PriceClass is the price class for this distribution.
	// +kubebuilder:validation:Enum=PriceClass_100;PriceClass_200;PriceClass_All
	// +optional
	PriceClass string `json:"priceClass,omitempty"`

	// Tags are metadata tags for the distribution.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// CloudFrontDistributionStatus defines the observed state of CloudFrontDistribution.
type CloudFrontDistributionStatus struct {
	// DistributionID is the ID of the distribution.
	// +optional
	DistributionID string `json:"distributionId,omitempty"`

	// DistributionARN is the ARN of the distribution.
	// +optional
	DistributionARN string `json:"distributionArn,omitempty"`

	// DomainName is the CloudFront domain name for the distribution.
	// +optional
	DomainName string `json:"domainName,omitempty"`

	// Status is the current deployment status of the distribution.
	// +optional
	Status string `json:"status,omitempty"`

	// ETag is the ETag for the distribution (required for updates and deletes).
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
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.distributionId"
// +kubebuilder:printcolumn:name="Domain",type="string",JSONPath=".status.domainName"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CloudFrontDistribution is the Schema for managing CloudFront distributions.
type CloudFrontDistribution struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudFrontDistributionSpec   `json:"spec,omitempty"`
	Status CloudFrontDistributionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CloudFrontDistributionList contains a list of CloudFrontDistribution.
type CloudFrontDistributionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudFrontDistribution `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudFrontDistribution{}, &CloudFrontDistributionList{})
}
