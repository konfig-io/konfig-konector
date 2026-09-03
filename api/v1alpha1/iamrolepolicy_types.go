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

// IAMRolePolicySpec defines the desired state of an inline IAM policy on a role.
type IAMRolePolicySpec struct {
	// RoleRef references the IAMRole CR that owns this inline policy.
	RoleRef RoleRef `json:"roleRef"`

	// PolicyName is the name of the inline policy within the role.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="policyName is immutable"
	PolicyName string `json:"policyName"`

	// PolicyDocument is the JSON IAM policy document.
	// +kubebuilder:validation:MinLength=1
	PolicyDocument string `json:"policyDocument"`
}

// IAMRolePolicyStatus defines the observed state of IAMRolePolicy.
type IAMRolePolicyStatus struct {
	// RoleARN is the resolved IAM role ARN.
	// +optional
	RoleARN string `json:"roleArn,omitempty"`

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

// IAMRolePolicy is the Schema for managing inline AWS IAM policies on roles.
type IAMRolePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IAMRolePolicySpec   `json:"spec,omitempty"`
	Status IAMRolePolicyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IAMRolePolicyList contains a list of IAMRolePolicy
type IAMRolePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IAMRolePolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IAMRolePolicy{}, &IAMRolePolicyList{})
}
