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

// IAMGroupPolicyAttachmentSpec defines the desired state of IAMGroupPolicyAttachment.
type IAMGroupPolicyAttachmentSpec struct {
	// GroupRef references the IAMGroup CR or a direct AWS group name.
	GroupRef GroupRef `json:"groupRef"`

	// PolicyRef references the IAMPolicy CR or a direct AWS policy ARN.
	PolicyRef PolicyRef `json:"policyRef"`
}

// IAMGroupPolicyAttachmentStatus defines the observed state of IAMGroupPolicyAttachment.
type IAMGroupPolicyAttachmentStatus struct {
	// Attached indicates whether the policy is currently attached to the group.
	// +optional
	Attached bool `json:"attached,omitempty"`

	// GroupName is the resolved IAM group name.
	// +optional
	GroupName string `json:"groupName,omitempty"`

	// PolicyARN is the resolved IAM policy ARN.
	// +optional
	PolicyARN string `json:"policyArn,omitempty"`

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
// +kubebuilder:printcolumn:name="Attached",type="boolean",JSONPath=".status.attached"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// IAMGroupPolicyAttachment is the Schema for attaching AWS IAM policies to groups.
type IAMGroupPolicyAttachment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IAMGroupPolicyAttachmentSpec   `json:"spec,omitempty"`
	Status IAMGroupPolicyAttachmentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IAMGroupPolicyAttachmentList contains a list of IAMGroupPolicyAttachment
type IAMGroupPolicyAttachmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IAMGroupPolicyAttachment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IAMGroupPolicyAttachment{}, &IAMGroupPolicyAttachmentList{})
}
