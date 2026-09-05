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

// IAMInstanceProfileSpec defines the desired state of an IAM instance profile.
type IAMInstanceProfileSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// InstanceProfileName is the name of the instance profile. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="instanceProfileName is immutable"
	InstanceProfileName string `json:"instanceProfileName"`

	// Path for the instance profile. Defaults to /.
	// +optional
	Path string `json:"path,omitempty"`

	// RoleRef references the IAM role to add to the profile (by IAMRole CR
	// name or direct ARN). An instance profile holds at most one role.
	// +optional
	RoleRef *RoleRef `json:"roleRef,omitempty"`

	// Tags are AWS resource tags to apply on creation.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// IAMInstanceProfileStatus defines the observed state of IAMInstanceProfile.
type IAMInstanceProfileStatus struct {
	// ARN is the ARN of the instance profile.
	// +optional
	ARN string `json:"arn,omitempty"`

	// RoleName is the name of the role currently attached to the profile.
	// +optional
	RoleName string `json:"roleName,omitempty"`

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

// IAMInstanceProfile is the Schema for managing IAM instance profiles.
type IAMInstanceProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IAMInstanceProfileSpec   `json:"spec,omitempty"`
	Status IAMInstanceProfileStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IAMInstanceProfileList contains a list of IAMInstanceProfile
type IAMInstanceProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IAMInstanceProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IAMInstanceProfile{}, &IAMInstanceProfileList{})
}
