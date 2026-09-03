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

// IAMSAMLProviderSpec defines the desired state of an AWS IAM SAML Provider.
type IAMSAMLProviderSpec struct {
	// Name is the name of the SAML provider. Immutable after creation — forms part of the ARN.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// SAMLMetadataDocument is the XML metadata document from the IdP.
	// +kubebuilder:validation:MinLength=1
	SAMLMetadataDocument string `json:"samlMetadataDocument"`

	// Tags are AWS resource tags to apply to the SAML provider.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// IAMSAMLProviderStatus defines the observed state of IAMSAMLProvider.
type IAMSAMLProviderStatus struct {
	// ARN is the Amazon Resource Name of the SAML provider.
	// +optional
	ARN string `json:"arn,omitempty"`

	// ValidUntil is the expiry date/time of the SAML metadata document.
	// +optional
	ValidUntil string `json:"validUntil,omitempty"`

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

// IAMSAMLProvider is the Schema for managing AWS IAM SAML Providers.
type IAMSAMLProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IAMSAMLProviderSpec   `json:"spec,omitempty"`
	Status IAMSAMLProviderStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IAMSAMLProviderList contains a list of IAMSAMLProvider
type IAMSAMLProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IAMSAMLProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IAMSAMLProvider{}, &IAMSAMLProviderList{})
}
