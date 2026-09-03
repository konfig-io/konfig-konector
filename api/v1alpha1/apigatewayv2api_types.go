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

// APIGatewayV2CorsConfiguration is a CORS configuration for an HTTP API.
type APIGatewayV2CorsConfiguration struct {
	// AllowCredentials specifies whether credentials are included in the CORS request.
	// +optional
	AllowCredentials *bool `json:"allowCredentials,omitempty"`

	// AllowHeaders are the allowed headers.
	// +optional
	AllowHeaders []string `json:"allowHeaders,omitempty"`

	// AllowMethods are the allowed HTTP methods.
	// +optional
	AllowMethods []string `json:"allowMethods,omitempty"`

	// AllowOrigins are the allowed origins.
	// +optional
	AllowOrigins []string `json:"allowOrigins,omitempty"`

	// ExposeHeaders are the exposed headers.
	// +optional
	ExposeHeaders []string `json:"exposeHeaders,omitempty"`

	// MaxAge is the number of seconds that the browser should cache preflight results.
	// +optional
	MaxAge *int32 `json:"maxAge,omitempty"`
}

// APIGatewayV2APISpec defines the desired state of an API Gateway v2 API.
type APIGatewayV2APISpec struct {
	// Name is the name of the API.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// ProtocolType is the API protocol. Immutable after creation.
	// +kubebuilder:validation:Enum=HTTP;WEBSOCKET
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="protocolType is immutable"
	ProtocolType string `json:"protocolType"`

	// Description of the API.
	// +optional
	Description string `json:"description,omitempty"`

	// RouteSelectionExpression is required for WebSocket APIs
	// (e.g. "$request.body.action"). For HTTP APIs it must be
	// "${request.method} ${request.path}" and defaults automatically.
	// +optional
	RouteSelectionExpression string `json:"routeSelectionExpression,omitempty"`

	// APIKeySelectionExpression is supported only for WebSocket APIs.
	// +optional
	APIKeySelectionExpression string `json:"apiKeySelectionExpression,omitempty"`

	// CORSConfiguration configures cross-origin resource sharing. HTTP APIs only.
	// +optional
	CORSConfiguration *APIGatewayV2CorsConfiguration `json:"corsConfiguration,omitempty"`

	// DisableExecuteAPIEndpoint disables the default execute-api endpoint.
	// +optional
	DisableExecuteAPIEndpoint bool `json:"disableExecuteApiEndpoint,omitempty"`

	// Tags are AWS resource tags to apply on creation.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// APIGatewayV2APIStatus defines the observed state of APIGatewayV2API.
type APIGatewayV2APIStatus struct {
	// APIID is the API identifier.
	// +optional
	APIID string `json:"apiId,omitempty"`

	// APIEndpoint is the default invoke URL of the API.
	// +optional
	APIEndpoint string `json:"apiEndpoint,omitempty"`

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
// +kubebuilder:printcolumn:name="API-ID",type="string",JSONPath=".status.apiId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// APIGatewayV2API is the Schema for managing API Gateway v2 (HTTP/WebSocket) APIs.
type APIGatewayV2API struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   APIGatewayV2APISpec   `json:"spec,omitempty"`
	Status APIGatewayV2APIStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// APIGatewayV2APIList contains a list of APIGatewayV2API
type APIGatewayV2APIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []APIGatewayV2API `json:"items"`
}

func init() {
	SchemeBuilder.Register(&APIGatewayV2API{}, &APIGatewayV2APIList{})
}
