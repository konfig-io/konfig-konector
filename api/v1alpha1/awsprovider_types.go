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

const (
	// ProviderAnnotation on a Namespace names the AWSProvider used by every
	// resource in that namespace that does not set spec.providerRef.
	ProviderAnnotation = "aws.konfig.io/provider"
)

// ProviderRef selects the AWSProvider (account + region) a resource is
// reconciled against. Resolution order: spec.providerRef, then the
// aws.konfig.io/provider annotation on the resource's Namespace, then the
// AWSProvider marked spec.default, then the operator's own credentials and
// region.
type ProviderRef struct {
	// Name of a cluster-scoped AWSProvider.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Region overrides the provider's region for this resource only.
	// +optional
	Region string `json:"region,omitempty"`
}

// ProviderStatus records which AWS account and region a resource was
// reconciled against. Every kind exposes it as status.awsProvider so the
// target account is confirmed on the object itself, not just on the provider.
type ProviderStatus struct {
	// Name of the AWSProvider used; empty when the operator's own credentials were used.
	// +optional
	Name string `json:"name,omitempty"`
	// AccountID the resource lives in.
	// +optional
	AccountID string `json:"accountId,omitempty"`
	// Region the resource lives in (empty for global services).
	// +optional
	Region string `json:"region,omitempty"`
}

// AWSProviderSpec defines how the operator obtains credentials for one AWS
// account (and optionally one region).
type AWSProviderSpec struct {
	// Default marks this provider as the primary account: every resource that
	// sets no providerRef and whose namespace carries no provider annotation
	// is reconciled here. At most one AWSProvider should be default; when
	// several are, the alphabetically first name wins.
	// +optional
	Default bool `json:"default,omitempty"`

	// RoleARN is the IAM role to assume in the target account. When empty the
	// operator's own credentials (EKS Pod Identity) are used, which makes the
	// provider a pure region override.
	// +optional
	RoleARN string `json:"roleArn,omitempty"`

	// Region is the default region for resources using this provider.
	// Falls back to the operator's region when empty.
	// +optional
	Region string `json:"region,omitempty"`

	// ExternalID is passed to sts:AssumeRole when set.
	// +optional
	ExternalID string `json:"externalId,omitempty"`

	// SessionName is the role session name; defaults to konfig-konector.
	// +optional
	SessionName string `json:"sessionName,omitempty"`

	// DurationSeconds is the assumed-role session lifetime (900-43200).
	// +kubebuilder:validation:Minimum=900
	// +kubebuilder:validation:Maximum=43200
	// +optional
	DurationSeconds int32 `json:"durationSeconds,omitempty"`

	// SourceProviderRef chains assumption: the role is assumed using
	// credentials from the named provider instead of the operator's own.
	// +optional
	SourceProviderRef *ProviderRef `json:"sourceProviderRef,omitempty"`

	// AllowedNamespaces restricts which namespaces may reference this
	// provider. Empty means every namespace. Supports exact names and a
	// trailing "*" glob (e.g. "team-*").
	// +optional
	AllowedNamespaces []string `json:"allowedNamespaces,omitempty"`
}

// AWSProviderStatus defines the observed state of AWSProvider.
type AWSProviderStatus struct {
	// AccountID is the account the resolved credentials belong to.
	// +optional
	AccountID string `json:"accountId,omitempty"`
	// AssumedRoleARN is the ARN returned by sts:GetCallerIdentity.
	// +optional
	AssumedRoleARN string `json:"assumedRoleArn,omitempty"`
	// Conditions describe the current state of the provider.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration is the generation last verified.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// LastSyncTime is the last successful credential verification.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=awsprov
// +kubebuilder:printcolumn:name="Default",type="boolean",JSONPath=".spec.default"
// +kubebuilder:printcolumn:name="Account",type="string",JSONPath=".status.accountId"
// +kubebuilder:printcolumn:name="Region",type="string",JSONPath=".spec.region"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AWSProvider is a cluster-scoped credential and region target that resources
// select with spec.providerRef. One operator instance can manage any number of
// AWS accounts by defining one AWSProvider per account (or per account+region).
type AWSProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AWSProviderSpec   `json:"spec,omitempty"`
	Status AWSProviderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AWSProviderList contains a list of AWSProvider.
type AWSProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AWSProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AWSProvider{}, &AWSProviderList{})
}
