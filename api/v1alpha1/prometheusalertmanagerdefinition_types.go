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

// PrometheusAlertManagerDefinitionSpec defines the desired state of the alert
// manager definition of an Amazon Managed Prometheus workspace. A workspace
// has at most one alert manager definition.
type PrometheusAlertManagerDefinitionSpec struct {
	// WorkspaceRef references the Prometheus workspace holding the definition.
	WorkspaceRef PrometheusWorkspaceRef `json:"workspaceRef"`

	// Definition is the alert manager definition, as a YAML string.
	// +kubebuilder:validation:MinLength=1
	Definition string `json:"definition"`
}

// PrometheusAlertManagerDefinitionStatus defines the observed state of
// PrometheusAlertManagerDefinition.
type PrometheusAlertManagerDefinitionStatus struct {
	// WorkspaceID is the resolved AWS workspace ID the definition was created in.
	// +optional
	WorkspaceID string `json:"workspaceId,omitempty"`

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
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PrometheusAlertManagerDefinition is the Schema for managing alert manager
// definitions in Amazon Managed Prometheus workspaces.
type PrometheusAlertManagerDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PrometheusAlertManagerDefinitionSpec   `json:"spec,omitempty"`
	Status PrometheusAlertManagerDefinitionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PrometheusAlertManagerDefinitionList contains a list of PrometheusAlertManagerDefinition
type PrometheusAlertManagerDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PrometheusAlertManagerDefinition `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PrometheusAlertManagerDefinition{}, &PrometheusAlertManagerDefinitionList{})
}
