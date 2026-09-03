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

// WAFRuleGroupSpec defines the desired state of a WAFv2 Rule Group.
type WAFRuleGroupSpec struct {
	// Name is the name of the rule group. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Scope is CLOUDFRONT or REGIONAL.
	// +kubebuilder:validation:Enum=CLOUDFRONT;REGIONAL
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="scope is immutable"
	Scope string `json:"scope"`

	// Capacity is the web ACL capacity units (WCUs) required for the rule group. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="capacity is immutable"
	Capacity int64 `json:"capacity"`

	// Description is a description of the rule group.
	// +optional
	Description string `json:"description,omitempty"`

	// Tags are metadata tags for the rule group.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// WAFRuleGroupStatus defines the observed state of WAFRuleGroup.
type WAFRuleGroupStatus struct {
	// ID is the identifier of the rule group.
	// +optional
	ID string `json:"id,omitempty"`

	// ARN is the ARN of the rule group.
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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.arn"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// WAFRuleGroup is the Schema for managing WAFv2 rule groups.
type WAFRuleGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WAFRuleGroupSpec   `json:"spec,omitempty"`
	Status WAFRuleGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// WAFRuleGroupList contains a list of WAFRuleGroup.
type WAFRuleGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WAFRuleGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WAFRuleGroup{}, &WAFRuleGroupList{})
}
