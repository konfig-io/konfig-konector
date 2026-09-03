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

// IAMRoleSpec defines the desired state of an AWS IAM Role.
type IAMRoleSpec struct {
	// RoleName is the name of the IAM role. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="roleName is immutable"
	RoleName string `json:"roleName"`

	// Description is a description of the role.
	// +optional
	Description string `json:"description,omitempty"`

	// MaxSessionDuration is the maximum session duration in seconds (3600–43200).
	// +kubebuilder:validation:Minimum=3600
	// +kubebuilder:validation:Maximum=43200
	// +optional
	MaxSessionDuration int32 `json:"maxSessionDuration,omitempty"`

	// Path is the IAM path for the role. Defaults to "/".
	// +optional
	Path string `json:"path,omitempty"`

	// AssumeRolePolicyDocument is the trust policy JSON document.
	// +kubebuilder:validation:MinLength=1
	AssumeRolePolicyDocument string `json:"assumeRolePolicyDocument"`

	// PermissionsBoundary is the ARN of a managed policy to use as a permissions boundary.
	// +optional
	PermissionsBoundary string `json:"permissionsBoundary,omitempty"`

	// Tags are AWS resource tags to apply to the role.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// IAMRoleStatus defines the observed state of IAMRole.
type IAMRoleStatus struct {
	// ARN is the Amazon Resource Name of the IAM role.
	// +optional
	ARN string `json:"arn,omitempty"`

	// RoleID is the stable and unique identifier of the IAM role.
	// +optional
	RoleID string `json:"roleId,omitempty"`

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

// IAMRole is the Schema for managing AWS IAM Roles.
type IAMRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IAMRoleSpec   `json:"spec,omitempty"`
	Status IAMRoleStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IAMRoleList contains a list of IAMRole
type IAMRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IAMRole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IAMRole{}, &IAMRoleList{})
}
