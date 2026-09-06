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

// UserPoolRef references either a managed UserPool CR or a direct user pool ID.
type UserPoolRef struct {
	// Name of a UserPool CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// UserPoolID is a direct AWS Cognito user pool ID.
	// +optional
	UserPoolID string `json:"userPoolId,omitempty"`
}

// UserPoolClientSpec defines the desired state of a Cognito User Pool Client.
type UserPoolClientSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// UserPoolRef references the user pool.
	UserPoolRef UserPoolRef `json:"userPoolRef"`

	// ClientName is the name of the app client.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	ClientName string `json:"clientName"`

	// GenerateSecret generates a client secret.
	// +optional
	GenerateSecret bool `json:"generateSecret,omitempty"`

	// ExplicitAuthFlows are the authentication flows to allow.
	// +optional
	ExplicitAuthFlows []string `json:"explicitAuthFlows,omitempty"`

	// AllowedOAuthFlows are the allowed OAuth flows.
	// +optional
	AllowedOAuthFlows []string `json:"allowedOAuthFlows,omitempty"`

	// AllowedOAuthScopes are the allowed OAuth scopes.
	// +optional
	AllowedOAuthScopes []string `json:"allowedOAuthScopes,omitempty"`

	// CallbackURLs are the allowed callback URLs.
	// +optional
	CallbackURLs []string `json:"callbackUrls,omitempty"`

	// LogoutURLs are the allowed sign-out URLs.
	// +optional
	LogoutURLs []string `json:"logoutUrls,omitempty"`

	// SupportedIdentityProviders are the supported identity providers.
	// +optional
	SupportedIdentityProviders []string `json:"supportedIdentityProviders,omitempty"`

	// AccessTokenValidity is the access token validity in minutes (1-86400).
	// +optional
	AccessTokenValidity int32 `json:"accessTokenValidity,omitempty"`

	// IdTokenValidity is the ID token validity in minutes (1-86400).
	// +optional
	IdTokenValidity int32 `json:"idTokenValidity,omitempty"`

	// RefreshTokenValidity is the refresh token validity in days (1-3650).
	// +optional
	RefreshTokenValidity int32 `json:"refreshTokenValidity,omitempty"`
}

// UserPoolClientStatus defines the observed state of UserPoolClient.
type UserPoolClientStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ClientID is the AWS Cognito app client ID.
	// +optional
	ClientID string `json:"clientId,omitempty"`

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
// +kubebuilder:printcolumn:name="Client-ID",type="string",JSONPath=".status.clientId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// UserPoolClient is the Schema for managing Cognito User Pool App Clients.
type UserPoolClient struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UserPoolClientSpec   `json:"spec,omitempty"`
	Status UserPoolClientStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// UserPoolClientList contains a list of UserPoolClient
type UserPoolClientList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UserPoolClient `json:"items"`
}

func init() {
	SchemeBuilder.Register(&UserPoolClient{}, &UserPoolClientList{})
}
