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

// SESEmailIdentitySpec defines the desired state of an SES Email Identity.
type SESEmailIdentitySpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// EmailIdentity is the email address or domain to verify.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="emailIdentity is immutable"
	EmailIdentity string `json:"emailIdentity"`

	// ConfigurationSetName is the configuration set to use by default.
	// +optional
	ConfigurationSetName string `json:"configurationSetName,omitempty"`

	// Tags are metadata tags for the email identity.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// SESEmailIdentityStatus defines the observed state of SESEmailIdentity.
type SESEmailIdentityStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// IdentityType is the type of email identity (EMAIL_ADDRESS or DOMAIN).
	// +optional
	IdentityType string `json:"identityType,omitempty"`

	// VerifiedForSendingStatus indicates whether the identity is verified for sending.
	// +optional
	VerifiedForSendingStatus bool `json:"verifiedForSendingStatus,omitempty"`

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
// +kubebuilder:printcolumn:name="Identity",type="string",JSONPath=".spec.emailIdentity"
// +kubebuilder:printcolumn:name="Verified",type="boolean",JSONPath=".status.verifiedForSendingStatus"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SESEmailIdentity is the Schema for managing SES email identities.
type SESEmailIdentity struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SESEmailIdentitySpec   `json:"spec,omitempty"`
	Status SESEmailIdentityStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SESEmailIdentityList contains a list of SESEmailIdentity.
type SESEmailIdentityList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SESEmailIdentity `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SESEmailIdentity{}, &SESEmailIdentityList{})
}
