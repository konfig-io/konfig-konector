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

// IdentityProviderSpec defines the desired state of a Cognito Identity Provider.
type IdentityProviderSpec struct {
	// UserPoolRef references the user pool.
	UserPoolRef UserPoolRef `json:"userPoolRef"`

	// ProviderName is the identity provider name.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=32
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="providerName is immutable"
	ProviderName string `json:"providerName"`

	// ProviderType is the identity provider type.
	// +kubebuilder:validation:Enum=SAML;Facebook;Google;LoginWithAmazon;SignInWithApple;OIDC
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="providerType is immutable"
	ProviderType string `json:"providerType"`

	// ProviderDetails are the provider-specific settings.
	ProviderDetails map[string]string `json:"providerDetails"`

	// AttributeMapping maps provider attributes to user pool attributes.
	// +optional
	AttributeMapping map[string]string `json:"attributeMapping,omitempty"`

	// IdpIdentifiers are alternative identifiers for the IdP.
	// +optional
	IdpIdentifiers []string `json:"idpIdentifiers,omitempty"`
}

// IdentityProviderStatus defines the observed state of IdentityProvider.
type IdentityProviderStatus struct {
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
// +kubebuilder:printcolumn:name="Provider-Type",type="string",JSONPath=".spec.providerType"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// IdentityProvider is the Schema for managing Cognito Identity Providers.
type IdentityProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IdentityProviderSpec   `json:"spec,omitempty"`
	Status IdentityProviderStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IdentityProviderList contains a list of IdentityProvider
type IdentityProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IdentityProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IdentityProvider{}, &IdentityProviderList{})
}
