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

// RestAPISpec defines the desired state of an API Gateway REST API.
type RestAPISpec struct {
	// Name of the REST API.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Description of the REST API.
	// +optional
	Description string `json:"description,omitempty"`

	// EndpointTypes is the endpoint configuration (REGIONAL, EDGE, PRIVATE).
	// +optional
	EndpointTypes []string `json:"endpointTypes,omitempty"`

	// Body is an OpenAPI (Swagger) definition in JSON or YAML. When set, the
	// whole API definition (resources, methods, integrations) is imported via
	// PutRestApi with mode=overwrite — the practical way to manage a deeply
	// nested REST API declaratively.
	// +optional
	Body string `json:"body,omitempty"`

	// DisableExecuteAPIEndpoint disables the default execute-api endpoint.
	// +optional
	DisableExecuteAPIEndpoint bool `json:"disableExecuteApiEndpoint,omitempty"`

	// MinimumCompressionSize enables payload compression above this size in
	// bytes (0-10485760).
	// +optional
	MinimumCompressionSize *int32 `json:"minimumCompressionSize,omitempty"`

	// Policy is a JSON resource policy for the API.
	// +optional
	Policy string `json:"policy,omitempty"`

	// Tags are AWS resource tags to apply on creation.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// RestAPIStatus defines the observed state of RestAPI.
type RestAPIStatus struct {
	// APIID is the REST API identifier.
	// +optional
	APIID string `json:"apiId,omitempty"`

	// RootResourceID is the ID of the API's root ("/") resource.
	// +optional
	RootResourceID string `json:"rootResourceId,omitempty"`

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

// RestAPI is the Schema for managing API Gateway REST APIs.
type RestAPI struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RestAPISpec   `json:"spec,omitempty"`
	Status RestAPIStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RestAPIList contains a list of RestAPI
type RestAPIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RestAPI `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RestAPI{}, &RestAPIList{})
}
