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

// AthenaNamedQuerySpec defines the desired state of an Athena named query.
// Athena has no UpdateNamedQuery API for name/query changes managed here, so
// every field is effectively create-only; changes after creation are
// surfaced as UpdateNotSupported.
type AthenaNamedQuerySpec struct {
	// Name is the query name.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	Name string `json:"name"`

	// Database is the database to which the query belongs.
	// +kubebuilder:validation:MinLength=1
	Database string `json:"database"`

	// QueryString is the SQL statements that make up the query.
	// +kubebuilder:validation:MinLength=1
	QueryString string `json:"queryString"`

	// WorkGroup is the workgroup in which the named query is saved.
	// +optional
	WorkGroup string `json:"workGroup,omitempty"`

	// Description is a description of the query.
	// +optional
	Description string `json:"description,omitempty"`
}

// AthenaNamedQueryStatus defines the observed state of AthenaNamedQuery.
type AthenaNamedQueryStatus struct {
	// NamedQueryID is the unique ID of the query in AWS.
	// +optional
	NamedQueryID string `json:"namedQueryId,omitempty"`

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
// +kubebuilder:printcolumn:name="Query-ID",type="string",JSONPath=".status.namedQueryId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AthenaNamedQuery is the Schema for managing Athena named queries.
type AthenaNamedQuery struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AthenaNamedQuerySpec   `json:"spec,omitempty"`
	Status AthenaNamedQueryStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AthenaNamedQueryList contains a list of AthenaNamedQuery
type AthenaNamedQueryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AthenaNamedQuery `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AthenaNamedQuery{}, &AthenaNamedQueryList{})
}
