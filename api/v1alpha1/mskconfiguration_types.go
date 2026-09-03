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

// MSKConfigurationSpec defines the desired state of an MSK cluster configuration.
type MSKConfigurationSpec struct {
	// Name is the name of the configuration. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// ServerProperties contains the kafka server.properties content.
	ServerProperties string `json:"serverProperties"`

	// KafkaVersions is the list of Kafka version strings this configuration applies to.
	// +optional
	KafkaVersions []string `json:"kafkaVersions,omitempty"`

	// Description describes the configuration.
	// +optional
	Description string `json:"description,omitempty"`
}

// MSKConfigurationStatus defines the observed state of MSKConfiguration.
type MSKConfigurationStatus struct {
	// ConfigurationARN is the ARN of the MSK configuration.
	// +optional
	ConfigurationARN string `json:"configurationARN,omitempty"`

	// LatestRevision is the current revision number.
	// +optional
	LatestRevision int64 `json:"latestRevision,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.configurationARN"
// +kubebuilder:printcolumn:name="Revision",type="integer",JSONPath=".status.latestRevision"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// MSKConfiguration is the Schema for managing MSK cluster configurations.
type MSKConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MSKConfigurationSpec   `json:"spec,omitempty"`
	Status MSKConfigurationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MSKConfigurationList contains a list of MSKConfiguration.
type MSKConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MSKConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MSKConfiguration{}, &MSKConfigurationList{})
}
