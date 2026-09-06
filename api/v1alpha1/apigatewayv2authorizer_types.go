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

// JWTConfiguration configures a JWT authorizer.
type JWTConfiguration struct {
	// Issuer is the base domain of the identity provider that issues JWTs
	// (e.g. "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_abc123").
	Issuer string `json:"issuer"`

	// Audience is the list of intended recipients of the JWT.
	// +optional
	Audience []string `json:"audience,omitempty"`
}

// APIGatewayV2AuthorizerSpec defines the desired state of an API Gateway v2 authorizer.
type APIGatewayV2AuthorizerSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// APIRef references the APIGatewayV2API this authorizer belongs to.
	APIRef APIRef `json:"apiRef"`

	// Name of the authorizer.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// AuthorizerType is JWT (HTTP APIs) or REQUEST (Lambda authorizer).
	// +kubebuilder:validation:Enum=JWT;REQUEST
	AuthorizerType string `json:"authorizerType"`

	// IdentitySource is the list of identity sources
	// (e.g. "$request.header.Authorization").
	// +optional
	IdentitySource []string `json:"identitySource,omitempty"`

	// JWTConfiguration is required when authorizerType is JWT.
	// +optional
	JWTConfiguration *JWTConfiguration `json:"jwtConfiguration,omitempty"`

	// AuthorizerURI is the Lambda authorizer invocation URI. Prefer
	// FunctionRef; if both are set, AuthorizerURI wins. REQUEST only.
	// +optional
	AuthorizerURI string `json:"authorizerUri,omitempty"`

	// FunctionRef references a LambdaFunction CR whose ARN is turned into the
	// authorizer URI. REQUEST only.
	// +optional
	FunctionRef *LambdaFunctionRef `json:"functionRef,omitempty"`

	// AuthorizerPayloadFormatVersion is 1.0 or 2.0. REQUEST only.
	// +kubebuilder:validation:Enum="1.0";"2.0"
	// +optional
	AuthorizerPayloadFormatVersion string `json:"authorizerPayloadFormatVersion,omitempty"`

	// AuthorizerResultTTLInSeconds is how long authorizer results are cached.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=3600
	// +optional
	AuthorizerResultTTLInSeconds *int32 `json:"authorizerResultTtlInSeconds,omitempty"`

	// EnableSimpleResponses lets a REQUEST authorizer return a boolean response.
	// +optional
	EnableSimpleResponses bool `json:"enableSimpleResponses,omitempty"`

	// AuthorizerCredentialsARN is the IAM role for API Gateway to invoke the
	// authorizer. REQUEST only.
	// +optional
	AuthorizerCredentialsARN string `json:"authorizerCredentialsArn,omitempty"`
}

// APIGatewayV2AuthorizerStatus defines the observed state of APIGatewayV2Authorizer.
type APIGatewayV2AuthorizerStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// AuthorizerID is the authorizer identifier.
	// +optional
	AuthorizerID string `json:"authorizerId,omitempty"`

	// APIID is the resolved API identifier the authorizer was created in.
	// +optional
	APIID string `json:"apiId,omitempty"`

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
// +kubebuilder:printcolumn:name="Authorizer-ID",type="string",JSONPath=".status.authorizerId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// APIGatewayV2Authorizer is the Schema for managing API Gateway v2 authorizers.
type APIGatewayV2Authorizer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   APIGatewayV2AuthorizerSpec   `json:"spec,omitempty"`
	Status APIGatewayV2AuthorizerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// APIGatewayV2AuthorizerList contains a list of APIGatewayV2Authorizer
type APIGatewayV2AuthorizerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []APIGatewayV2Authorizer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&APIGatewayV2Authorizer{}, &APIGatewayV2AuthorizerList{})
}
