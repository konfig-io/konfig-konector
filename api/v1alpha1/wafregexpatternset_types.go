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

// WAFRegexPatternSetSpec defines the desired state of a WAFv2 Regex Pattern Set.
type WAFRegexPatternSetSpec struct {
	// Name is the name of the regex pattern set. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Scope is CLOUDFRONT or REGIONAL.
	// +kubebuilder:validation:Enum=CLOUDFRONT;REGIONAL
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="scope is immutable"
	Scope string `json:"scope"`

	// RegularExpressionList is the list of regex patterns.
	// +kubebuilder:validation:MinItems=1
	RegularExpressionList []string `json:"regularExpressionList"`

	// Description is a description of the set.
	// +optional
	Description string `json:"description,omitempty"`

	// Tags are metadata tags for the regex pattern set.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// WAFRegexPatternSetStatus defines the observed state of WAFRegexPatternSet.
type WAFRegexPatternSetStatus struct {
	// ID is the identifier of the regex pattern set.
	// +optional
	ID string `json:"id,omitempty"`

	// ARN is the ARN of the regex pattern set.
	// +optional
	ARN string `json:"arn,omitempty"`

	// LockToken is required for updates and deletes.
	// +optional
	LockToken string `json:"lockToken,omitempty"`

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
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.id"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// WAFRegexPatternSet is the Schema for managing WAFv2 regex pattern sets.
type WAFRegexPatternSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WAFRegexPatternSetSpec   `json:"spec,omitempty"`
	Status WAFRegexPatternSetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// WAFRegexPatternSetList contains a list of WAFRegexPatternSet.
type WAFRegexPatternSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WAFRegexPatternSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WAFRegexPatternSet{}, &WAFRegexPatternSetList{})
}
