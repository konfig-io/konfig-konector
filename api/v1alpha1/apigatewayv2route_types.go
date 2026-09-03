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

// IntegrationRef references a managed APIGatewayV2Integration CR or a direct
// integration ID.
type IntegrationRef struct {
	// Name of an APIGatewayV2Integration CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// IntegrationID is a direct AWS integration ID, bypassing CR lookup.
	// +optional
	IntegrationID string `json:"integrationId,omitempty"`
}

// AuthorizerRef references a managed APIGatewayV2Authorizer CR or a direct
// authorizer ID.
type AuthorizerRef struct {
	// Name of an APIGatewayV2Authorizer CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// AuthorizerID is a direct AWS authorizer ID, bypassing CR lookup.
	// +optional
	AuthorizerID string `json:"authorizerId,omitempty"`
}

// APIGatewayV2RouteSpec defines the desired state of an API Gateway v2 route.
type APIGatewayV2RouteSpec struct {
	// APIRef references the APIGatewayV2API this route belongs to.
	APIRef APIRef `json:"apiRef"`

	// RouteKey is the route key (e.g. "GET /pets" or "$default").
	// +kubebuilder:validation:MinLength=1
	RouteKey string `json:"routeKey"`

	// Target is a raw route target (e.g. "integrations/abc123"). Prefer
	// IntegrationRef; if both are set, Target wins.
	// +optional
	Target string `json:"target,omitempty"`

	// IntegrationRef references the APIGatewayV2Integration this route targets.
	// Resolved into a "integrations/<id>" target.
	// +optional
	IntegrationRef *IntegrationRef `json:"integrationRef,omitempty"`

	// AuthorizationType is the authorization type for the route.
	// +kubebuilder:validation:Enum=NONE;AWS_IAM;CUSTOM;JWT
	// +optional
	AuthorizationType string `json:"authorizationType,omitempty"`

	// AuthorizerRef references the APIGatewayV2Authorizer for the route when
	// authorizationType is CUSTOM or JWT.
	// +optional
	AuthorizerRef *AuthorizerRef `json:"authorizerRef,omitempty"`

	// AuthorizationScopes are the OAuth scopes for JWT authorization.
	// +optional
	AuthorizationScopes []string `json:"authorizationScopes,omitempty"`

	// APIKeyRequired specifies whether an API key is required (WebSocket only).
	// +optional
	APIKeyRequired bool `json:"apiKeyRequired,omitempty"`

	// OperationName is the operation name for the route.
	// +optional
	OperationName string `json:"operationName,omitempty"`
}

// APIGatewayV2RouteStatus defines the observed state of APIGatewayV2Route.
type APIGatewayV2RouteStatus struct {
	// RouteID is the route identifier.
	// +optional
	RouteID string `json:"routeId,omitempty"`

	// APIID is the resolved API identifier the route was created in.
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
// +kubebuilder:printcolumn:name="Route-ID",type="string",JSONPath=".status.routeId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// APIGatewayV2Route is the Schema for managing API Gateway v2 routes.
type APIGatewayV2Route struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   APIGatewayV2RouteSpec   `json:"spec,omitempty"`
	Status APIGatewayV2RouteStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// APIGatewayV2RouteList contains a list of APIGatewayV2Route
type APIGatewayV2RouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []APIGatewayV2Route `json:"items"`
}

func init() {
	SchemeBuilder.Register(&APIGatewayV2Route{}, &APIGatewayV2RouteList{})
}
