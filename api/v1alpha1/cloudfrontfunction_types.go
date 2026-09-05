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

// CloudFrontFunctionSpec defines the desired state of a CloudFront Function.
type CloudFrontFunctionSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the name of the function.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// FunctionCode is the function code (base64-encoded or plain text).
	FunctionCode string `json:"functionCode"`

	// Runtime is the function's runtime environment.
	// +kubebuilder:validation:Enum=cloudfront-js-1.0;cloudfront-js-2.0
	// +optional
	Runtime string `json:"runtime,omitempty"`

	// Comment is an optional description for the function.
	// +optional
	Comment string `json:"comment,omitempty"`
}

// CloudFrontFunctionStatus defines the observed state of CloudFrontFunction.
type CloudFrontFunctionStatus struct {
	// FunctionARN is the ARN of the function.
	// +optional
	FunctionARN string `json:"functionArn,omitempty"`

	// ETag is the ETag for the function.
	// +optional
	ETag string `json:"etag,omitempty"`

	// FunctionStatus is the current status of the function.
	// +optional
	FunctionStatus string `json:"functionStatus,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.functionArn"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CloudFrontFunction is the Schema for managing CloudFront functions.
type CloudFrontFunction struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudFrontFunctionSpec   `json:"spec,omitempty"`
	Status CloudFrontFunctionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CloudFrontFunctionList contains a list of CloudFrontFunction.
type CloudFrontFunctionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudFrontFunction `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudFrontFunction{}, &CloudFrontFunctionList{})
}
