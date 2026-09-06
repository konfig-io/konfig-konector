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

// CodeCommitRepositorySpec defines the desired state of a CodeCommit repository.
type CodeCommitRepositorySpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// RepositoryName is the name of the repository. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="repositoryName is immutable"
	RepositoryName string `json:"repositoryName"`

	// RepositoryDescription is an optional description of the repository.
	// +optional
	RepositoryDescription string `json:"repositoryDescription,omitempty"`

	// Tags are metadata tags for the repository.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// CodeCommitRepositoryStatus defines the observed state of CodeCommitRepository.
type CodeCommitRepositoryStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// RepositoryID is the CodeCommit repository ID.
	// +optional
	RepositoryID string `json:"repositoryID,omitempty"`

	// RepositoryARN is the ARN of the CodeCommit repository.
	// +optional
	RepositoryARN string `json:"repositoryARN,omitempty"`

	// CloneURLHTTP is the HTTPS clone URL for the repository.
	// +optional
	CloneURLHTTP string `json:"cloneURLHTTP,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.repositoryARN"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CodeCommitRepository is the Schema for managing AWS CodeCommit repositories.
type CodeCommitRepository struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CodeCommitRepositorySpec   `json:"spec,omitempty"`
	Status CodeCommitRepositoryStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CodeCommitRepositoryList contains a list of CodeCommitRepository.
type CodeCommitRepositoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CodeCommitRepository `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CodeCommitRepository{}, &CodeCommitRepositoryList{})
}
