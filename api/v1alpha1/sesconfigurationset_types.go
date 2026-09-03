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

// SESConfigurationSetSpec defines the desired state of an SES Configuration Set.
type SESConfigurationSetSpec struct {
	// ConfigurationSetName is the name of the configuration set.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="configurationSetName is immutable"
	ConfigurationSetName string `json:"configurationSetName"`

	// SendingEnabled controls whether sending is enabled for this configuration set.
	// +optional
	SendingEnabled *bool `json:"sendingEnabled,omitempty"`

	// ReputationMetricsEnabled controls whether reputation metrics are enabled.
	// +optional
	ReputationMetricsEnabled *bool `json:"reputationMetricsEnabled,omitempty"`

	// Tags are metadata tags for the configuration set.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// SESConfigurationSetStatus defines the observed state of SESConfigurationSet.
type SESConfigurationSetStatus struct {
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
// +kubebuilder:printcolumn:name="Name",type="string",JSONPath=".spec.configurationSetName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SESConfigurationSet is the Schema for managing SES configuration sets.
type SESConfigurationSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SESConfigurationSetSpec   `json:"spec,omitempty"`
	Status SESConfigurationSetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SESConfigurationSetList contains a list of SESConfigurationSet.
type SESConfigurationSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SESConfigurationSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SESConfigurationSet{}, &SESConfigurationSetList{})
}
