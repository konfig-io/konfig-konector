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

// APIGatewayV2IntegrationSpec defines the desired state of an API Gateway v2 integration.
type APIGatewayV2IntegrationSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// APIRef references the APIGatewayV2API this integration belongs to.
	APIRef APIRef `json:"apiRef"`

	// IntegrationType is the integration type.
	// +kubebuilder:validation:Enum=AWS;AWS_PROXY;HTTP;HTTP_PROXY;MOCK
	IntegrationType string `json:"integrationType"`

	// IntegrationURI is the raw integration URI (e.g. a Lambda function ARN
	// for AWS_PROXY, or an HTTP endpoint URL for HTTP_PROXY). Prefer
	// FunctionRef for Lambda targets; if both are set, IntegrationURI wins.
	// +optional
	IntegrationURI string `json:"integrationUri,omitempty"`

	// FunctionRef references a LambdaFunction CR whose ARN becomes the
	// integration URI.
	// +optional
	FunctionRef *LambdaFunctionRef `json:"functionRef,omitempty"`

	// IntegrationMethod is the HTTP method for HTTP integrations (e.g. POST).
	// +optional
	IntegrationMethod string `json:"integrationMethod,omitempty"`

	// PayloadFormatVersion is 1.0 or 2.0 (Lambda proxy integrations).
	// +kubebuilder:validation:Enum="1.0";"2.0"
	// +optional
	PayloadFormatVersion string `json:"payloadFormatVersion,omitempty"`

	// Description of the integration.
	// +optional
	Description string `json:"description,omitempty"`

	// ConnectionType is INTERNET or VPC_LINK.
	// +kubebuilder:validation:Enum=INTERNET;VPC_LINK
	// +optional
	ConnectionType string `json:"connectionType,omitempty"`

	// VPCLinkRef references the APIGatewayV2VpcLink used when connectionType
	// is VPC_LINK. Resolved to the connection ID.
	// +optional
	VPCLinkRef *ResourceRef `json:"vpcLinkRef,omitempty"`

	// ConnectionID is a direct VPC link ID used when connectionType is
	// VPC_LINK, bypassing CR lookup.
	// +optional
	ConnectionID string `json:"connectionId,omitempty"`

	// CredentialsARN is the credentials for the integration.
	// +optional
	CredentialsARN string `json:"credentialsArn,omitempty"`

	// RequestParameters are parameter mappings.
	// +optional
	RequestParameters map[string]string `json:"requestParameters,omitempty"`

	// TimeoutInMillis is the custom timeout (50-30000 ms for HTTP APIs).
	// +kubebuilder:validation:Minimum=50
	// +optional
	TimeoutInMillis *int32 `json:"timeoutInMillis,omitempty"`
}

// APIGatewayV2IntegrationStatus defines the observed state of APIGatewayV2Integration.
type APIGatewayV2IntegrationStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// IntegrationID is the integration identifier.
	// +optional
	IntegrationID string `json:"integrationId,omitempty"`

	// APIID is the resolved API identifier the integration was created in.
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
// +kubebuilder:printcolumn:name="Integration-ID",type="string",JSONPath=".status.integrationId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// APIGatewayV2Integration is the Schema for managing API Gateway v2 integrations.
type APIGatewayV2Integration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   APIGatewayV2IntegrationSpec   `json:"spec,omitempty"`
	Status APIGatewayV2IntegrationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// APIGatewayV2IntegrationList contains a list of APIGatewayV2Integration
type APIGatewayV2IntegrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []APIGatewayV2Integration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&APIGatewayV2Integration{}, &APIGatewayV2IntegrationList{})
}
