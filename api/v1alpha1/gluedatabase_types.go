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

// GlueDatabaseSpec defines the desired state of a Glue Data Catalog database.
type GlueDatabaseSpec struct {
	// Name is the name of the database. For Hive compatibility it is folded
	// to lowercase when stored. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Description is a description of the database.
	// +optional
	Description string `json:"description,omitempty"`

	// LocationURI is the location of the database (for example, an S3 path).
	// +optional
	LocationURI string `json:"locationUri,omitempty"`

	// Parameters are key-value pairs that define properties of the database.
	// +optional
	Parameters map[string]string `json:"parameters,omitempty"`

	// Tags are AWS resource tags to apply at creation.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// GlueDatabaseStatus defines the observed state of GlueDatabase.
type GlueDatabaseStatus struct {
	// DatabaseName is the name of the database in the Glue Data Catalog.
	// +optional
	DatabaseName string `json:"databaseName,omitempty"`

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
// +kubebuilder:printcolumn:name="Database",type="string",JSONPath=".status.databaseName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// GlueDatabase is the Schema for managing Glue Data Catalog databases.
type GlueDatabase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GlueDatabaseSpec   `json:"spec,omitempty"`
	Status GlueDatabaseStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GlueDatabaseList contains a list of GlueDatabase
type GlueDatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GlueDatabase `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GlueDatabase{}, &GlueDatabaseList{})
}
