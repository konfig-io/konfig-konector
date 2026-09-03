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

// DomainNameRef references a managed APIGatewayV2DomainName CR or a direct
// AWS domain name.
type DomainNameRef struct {
	// Name of an APIGatewayV2DomainName CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// DomainName is a direct AWS custom domain name, bypassing CR lookup.
	// +optional
	DomainName string `json:"domainName,omitempty"`
}

// APIGatewayV2ApiMappingSpec defines the desired state of an API mapping.
type APIGatewayV2ApiMappingSpec struct {
	// APIRef references the APIGatewayV2API to map.
	APIRef APIRef `json:"apiRef"`

	// DomainNameRef references the APIGatewayV2DomainName to map onto.
	DomainNameRef DomainNameRef `json:"domainNameRef"`

	// Stage is the API stage to map (e.g. "$default").
	// +kubebuilder:validation:MinLength=1
	Stage string `json:"stage"`

	// APIMappingKey is the path key under the domain (empty for root).
	// +optional
	APIMappingKey string `json:"apiMappingKey,omitempty"`
}

// APIGatewayV2ApiMappingStatus defines the observed state of APIGatewayV2ApiMapping.
type APIGatewayV2ApiMappingStatus struct {
	// APIMappingID is the mapping identifier.
	// +optional
	APIMappingID string `json:"apiMappingId,omitempty"`

	// DomainName is the resolved AWS domain name the mapping was created on.
	// +optional
	DomainName string `json:"domainName,omitempty"`

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
// +kubebuilder:printcolumn:name="Mapping-ID",type="string",JSONPath=".status.apiMappingId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// APIGatewayV2ApiMapping is the Schema for managing API Gateway v2 API mappings.
type APIGatewayV2ApiMapping struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   APIGatewayV2ApiMappingSpec   `json:"spec,omitempty"`
	Status APIGatewayV2ApiMappingStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// APIGatewayV2ApiMappingList contains a list of APIGatewayV2ApiMapping
type APIGatewayV2ApiMappingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []APIGatewayV2ApiMapping `json:"items"`
}

func init() {
	SchemeBuilder.Register(&APIGatewayV2ApiMapping{}, &APIGatewayV2ApiMappingList{})
}
