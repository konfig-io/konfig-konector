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

// CodeArtifactDomainRef references either a managed CodeArtifactDomain CR or
// a direct AWS domain name.
type CodeArtifactDomainRef struct {
	// Name of a CodeArtifactDomain CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// DomainName is a direct AWS CodeArtifact domain name, bypassing CR lookup.
	// +optional
	DomainName string `json:"domainName,omitempty"`
}

// CodeArtifactRepositorySpec defines the desired state of a CodeArtifact repository.
type CodeArtifactRepositorySpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// RepositoryName is the name of the repository. Immutable.
	// +kubebuilder:validation:MinLength=2
	// +kubebuilder:validation:MaxLength=100
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="repositoryName is immutable"
	RepositoryName string `json:"repositoryName"`

	// DomainRef references the CodeArtifact domain that contains the repository.
	DomainRef CodeArtifactDomainRef `json:"domainRef"`

	// Description is a text description of the repository.
	// +optional
	Description string `json:"description,omitempty"`

	// Upstreams is an ordered list of upstream repository names in the same
	// domain. Priority follows list order.
	// +optional
	Upstreams []string `json:"upstreams,omitempty"`

	// ExternalConnections is a list of external connection names to associate,
	// e.g. "public:npmjs", "public:pypi", "public:maven-central".
	// A repository supports at most one external connection.
	// +optional
	ExternalConnections []string `json:"externalConnections,omitempty"`

	// Tags are AWS resource tags to apply at creation time.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// CodeArtifactRepositoryStatus defines the observed state of CodeArtifactRepository.
type CodeArtifactRepositoryStatus struct {
	// ARN is the ARN of the repository.
	// +optional
	ARN string `json:"arn,omitempty"`

	// DomainName is the resolved AWS domain name the repository was created in.
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
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CodeArtifactRepository is the Schema for managing CodeArtifact repositories.
type CodeArtifactRepository struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CodeArtifactRepositorySpec   `json:"spec,omitempty"`
	Status CodeArtifactRepositoryStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CodeArtifactRepositoryList contains a list of CodeArtifactRepository
type CodeArtifactRepositoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CodeArtifactRepository `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CodeArtifactRepository{}, &CodeArtifactRepositoryList{})
}
