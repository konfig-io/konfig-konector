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

// AthenaDataCatalogSpec defines the desired state of an Athena data catalog.
type AthenaDataCatalogSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the data catalog name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=127
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Type is the catalog type: LAMBDA, GLUE, or HIVE.
	// +kubebuilder:validation:Enum=LAMBDA;GLUE;HIVE
	Type string `json:"type"`

	// Parameters are type-specific key-value pairs (e.g. function for
	// LAMBDA, catalog-id for GLUE, metadata-function for HIVE).
	// +optional
	Parameters map[string]string `json:"parameters,omitempty"`

	// Description is a description of the data catalog.
	// +optional
	Description string `json:"description,omitempty"`

	// Tags are AWS resource tags to apply at creation.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// AthenaDataCatalogStatus defines the observed state of AthenaDataCatalog.
type AthenaDataCatalogStatus struct {
	// CatalogName is the name of the data catalog in AWS.
	// +optional
	CatalogName string `json:"catalogName,omitempty"`

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
// +kubebuilder:printcolumn:name="Catalog",type="string",JSONPath=".status.catalogName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AthenaDataCatalog is the Schema for managing Athena data catalogs.
type AthenaDataCatalog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AthenaDataCatalogSpec   `json:"spec,omitempty"`
	Status AthenaDataCatalogStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AthenaDataCatalogList contains a list of AthenaDataCatalog
type AthenaDataCatalogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AthenaDataCatalog `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AthenaDataCatalog{}, &AthenaDataCatalogList{})
}
