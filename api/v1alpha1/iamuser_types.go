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

// IAMUserSpec defines the desired state of an AWS IAM User.
type IAMUserSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// UserName is the name of the IAM user. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="userName is immutable"
	UserName string `json:"userName"`

	// Path is the IAM path for the user. Defaults to "/".
	// +optional
	Path string `json:"path,omitempty"`

	// PermissionsBoundary is the ARN of a managed policy to use as a permissions boundary.
	// +optional
	PermissionsBoundary string `json:"permissionsBoundary,omitempty"`

	// Tags are AWS resource tags to apply to the user.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// IAMUserStatus defines the observed state of IAMUser.
type IAMUserStatus struct {
	// ARN is the Amazon Resource Name of the IAM user.
	// +optional
	ARN string `json:"arn,omitempty"`

	// UserID is the stable and unique identifier of the IAM user.
	// +optional
	UserID string `json:"userId,omitempty"`

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

// IAMUser is the Schema for managing AWS IAM Users.
type IAMUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IAMUserSpec   `json:"spec,omitempty"`
	Status IAMUserStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IAMUserList contains a list of IAMUser
type IAMUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IAMUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IAMUser{}, &IAMUserList{})
}
