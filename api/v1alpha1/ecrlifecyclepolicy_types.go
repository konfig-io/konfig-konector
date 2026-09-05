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

// ECRLifecyclePolicySpec defines the desired state of an ECR Lifecycle Policy.
type ECRLifecyclePolicySpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// RepositoryRef references the ECR repository.
	RepositoryRef ECRRepositoryRef `json:"repositoryRef"`

	// LifecyclePolicyDocument is the JSON lifecycle policy.
	LifecyclePolicyDocument string `json:"lifecyclePolicyDocument"`
}

// ECRLifecyclePolicyStatus defines the observed state of ECRLifecyclePolicy.
type ECRLifecyclePolicyStatus struct {
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

// ECRLifecyclePolicy is the Schema for managing ECR Lifecycle Policies.
type ECRLifecyclePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ECRLifecyclePolicySpec   `json:"spec,omitempty"`
	Status ECRLifecyclePolicyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ECRLifecyclePolicyList contains a list of ECRLifecyclePolicy
type ECRLifecyclePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ECRLifecyclePolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ECRLifecyclePolicy{}, &ECRLifecyclePolicyList{})
}
