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

// ECRRepositoryRef references either a managed ECRRepository CR or a direct repository name.
type ECRRepositoryRef struct {
	// Name of an ECRRepository CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// RepositoryName is a direct AWS ECR repository name.
	// +optional
	RepositoryName string `json:"repositoryName,omitempty"`
}

// ECRRepositoryPolicySpec defines the desired state of an ECR Repository Policy.
type ECRRepositoryPolicySpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// RepositoryRef references the ECR repository.
	RepositoryRef ECRRepositoryRef `json:"repositoryRef"`

	// PolicyDocument is the JSON resource policy to apply.
	PolicyDocument string `json:"policyDocument"`
}

// ECRRepositoryPolicyStatus defines the observed state of ECRRepositoryPolicy.
type ECRRepositoryPolicyStatus struct {
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

// ECRRepositoryPolicy is the Schema for managing ECR Repository Policies.
type ECRRepositoryPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ECRRepositoryPolicySpec   `json:"spec,omitempty"`
	Status ECRRepositoryPolicyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ECRRepositoryPolicyList contains a list of ECRRepositoryPolicy
type ECRRepositoryPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ECRRepositoryPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ECRRepositoryPolicy{}, &ECRRepositoryPolicyList{})
}
