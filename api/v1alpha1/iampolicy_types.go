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

// IAMPolicySpec defines the desired state of an AWS IAM managed policy.
type IAMPolicySpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// PolicyName is the name of the IAM policy. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="policyName is immutable"
	PolicyName string `json:"policyName"`

	// Description is a description of the policy.
	// +optional
	Description string `json:"description,omitempty"`

	// Path is the IAM path for the policy. Defaults to "/".
	// +optional
	Path string `json:"path,omitempty"`

	// PolicyDocument is the JSON IAM policy document.
	// +kubebuilder:validation:MinLength=1
	PolicyDocument string `json:"policyDocument"`

	// Tags are AWS resource tags to apply to the policy.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// IAMPolicyStatus defines the observed state of IAMPolicy.
type IAMPolicyStatus struct {
	// ARN is the Amazon Resource Name of the IAM policy.
	// +optional
	ARN string `json:"arn,omitempty"`

	// PolicyID is the stable and unique identifier of the IAM policy.
	// +optional
	PolicyID string `json:"policyId,omitempty"`

	// DefaultVersionID is the current default version of the policy document.
	// +optional
	DefaultVersionID string `json:"defaultVersionId,omitempty"`

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

// IAMPolicy is the Schema for managing AWS IAM managed policies.
type IAMPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IAMPolicySpec   `json:"spec,omitempty"`
	Status IAMPolicyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IAMPolicyList contains a list of IAMPolicy
type IAMPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IAMPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IAMPolicy{}, &IAMPolicyList{})
}
