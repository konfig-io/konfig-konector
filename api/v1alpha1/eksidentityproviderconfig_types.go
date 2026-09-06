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

// EKSOIDCConfig defines the OIDC configuration for an EKS identity provider.
type EKSOIDCConfig struct {
	// ClientID is the OIDC client application ID.
	// +kubebuilder:validation:MinLength=1
	ClientID string `json:"clientId"`

	// IssuerURL is the OIDC provider URL.
	// +kubebuilder:validation:MinLength=1
	IssuerURL string `json:"issuerUrl"`

	// UsernameClaim is the JWT claim to use as the username.
	// +optional
	UsernameClaim string `json:"usernameClaim,omitempty"`

	// UsernamePrefix is a prefix to prepend to usernames.
	// +optional
	UsernamePrefix string `json:"usernamePrefix,omitempty"`

	// GroupsClaim is the JWT claim to use for group membership.
	// +optional
	GroupsClaim string `json:"groupsClaim,omitempty"`

	// GroupsPrefix is a prefix to prepend to group names.
	// +optional
	GroupsPrefix string `json:"groupsPrefix,omitempty"`

	// RequiredClaims are key-value pairs that must be present in the token.
	// +optional
	RequiredClaims map[string]string `json:"requiredClaims,omitempty"`
}

// EKSIdentityProviderConfigSpec defines the desired state of an EKS Identity Provider Config.
type EKSIdentityProviderConfigSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ClusterName is the name of the EKS cluster.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="clusterName is immutable"
	ClusterName string `json:"clusterName"`

	// IdentityProviderConfigName is the name of the identity provider configuration.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="identityProviderConfigName is immutable"
	IdentityProviderConfigName string `json:"identityProviderConfigName"`

	// OIDC is the OIDC identity provider configuration.
	OIDC EKSOIDCConfig `json:"oidc"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// EKSIdentityProviderConfigStatus defines the observed state of EKSIdentityProviderConfig.
type EKSIdentityProviderConfigStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ARN is the ARN of the identity provider configuration.
	// +optional
	ARN string `json:"arn,omitempty"`

	// Status is the current state of the configuration.
	// +optional
	Status string `json:"status,omitempty"`

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
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.clusterName"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EKSIdentityProviderConfig is the Schema for managing EKS OIDC identity provider configurations.
type EKSIdentityProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EKSIdentityProviderConfigSpec   `json:"spec,omitempty"`
	Status EKSIdentityProviderConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EKSIdentityProviderConfigList contains a list of EKSIdentityProviderConfig.
type EKSIdentityProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EKSIdentityProviderConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EKSIdentityProviderConfig{}, &EKSIdentityProviderConfigList{})
}
