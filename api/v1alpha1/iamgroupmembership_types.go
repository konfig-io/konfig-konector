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

// IAMGroupMembershipSpec defines the desired state of IAMGroupMembership.
type IAMGroupMembershipSpec struct {
	// GroupRef references the IAMGroup CR or a direct AWS group name.
	GroupRef GroupRef `json:"groupRef"`

	// UserRef references the IAMUser CR or a direct AWS user name.
	UserRef UserRef `json:"userRef"`
}

// IAMGroupMembershipStatus defines the observed state of IAMGroupMembership.
type IAMGroupMembershipStatus struct {
	// Member indicates whether the user is currently a member of the group.
	// +optional
	Member bool `json:"member,omitempty"`

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
// +kubebuilder:printcolumn:name="Member",type="boolean",JSONPath=".status.member"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// IAMGroupMembership is the Schema for adding AWS IAM users to groups.
type IAMGroupMembership struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IAMGroupMembershipSpec   `json:"spec,omitempty"`
	Status IAMGroupMembershipStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IAMGroupMembershipList contains a list of IAMGroupMembership
type IAMGroupMembershipList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IAMGroupMembership `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IAMGroupMembership{}, &IAMGroupMembershipList{})
}
