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

// OpenSearchAccessPolicySpec defines the desired state of an OpenSearch Serverless access policy.
type OpenSearchAccessPolicySpec struct {
	// Name is the name of the access policy. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Type is the access policy type (data).
	// +kubebuilder:validation:Enum=data
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="type is immutable"
	Type string `json:"type"`

	// PolicyDocument is the JSON policy document.
	PolicyDocument string `json:"policyDocument"`

	// Description is a description of the policy.
	// +optional
	Description string `json:"description,omitempty"`
}

// OpenSearchAccessPolicyStatus defines the observed state of OpenSearchAccessPolicy.
type OpenSearchAccessPolicyStatus struct {
	// PolicyVersion is the current version of the policy.
	// +optional
	PolicyVersion string `json:"policyVersion,omitempty"`

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
// +kubebuilder:printcolumn:name="PolicyVersion",type="string",JSONPath=".status.policyVersion"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenSearchAccessPolicy is the Schema for managing OpenSearch Serverless access policies.
type OpenSearchAccessPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenSearchAccessPolicySpec   `json:"spec,omitempty"`
	Status OpenSearchAccessPolicyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OpenSearchAccessPolicyList contains a list of OpenSearchAccessPolicy.
type OpenSearchAccessPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenSearchAccessPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenSearchAccessPolicy{}, &OpenSearchAccessPolicyList{})
}
