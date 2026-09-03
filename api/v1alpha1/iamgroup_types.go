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

// IAMGroupSpec defines the desired state of an AWS IAM Group.
type IAMGroupSpec struct {
	// GroupName is the name of the IAM group. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="groupName is immutable"
	GroupName string `json:"groupName"`

	// Path is the IAM path for the group. Defaults to "/".
	// +optional
	Path string `json:"path,omitempty"`
}

// IAMGroupStatus defines the observed state of IAMGroup.
type IAMGroupStatus struct {
	// ARN is the Amazon Resource Name of the IAM group.
	// +optional
	ARN string `json:"arn,omitempty"`

	// GroupID is the stable and unique identifier of the IAM group.
	// +optional
	GroupID string `json:"groupId,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.arn"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// IAMGroup is the Schema for managing AWS IAM Groups.
type IAMGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IAMGroupSpec   `json:"spec,omitempty"`
	Status IAMGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IAMGroupList contains a list of IAMGroup
type IAMGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IAMGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IAMGroup{}, &IAMGroupList{})
}
