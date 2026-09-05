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

// MQConfigurationSpec defines the desired state of an Amazon MQ configuration.
type MQConfigurationSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name of the configuration. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=150
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// EngineType is the broker engine. Immutable after creation.
	// +kubebuilder:validation:Enum=ACTIVEMQ;RABBITMQ
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="engineType is immutable"
	EngineType string `json:"engineType"`

	// EngineVersion is the broker engine version.
	// +optional
	EngineVersion string `json:"engineVersion,omitempty"`

	// Data is the base64-encoded XML configuration (ActiveMQ) or
	// Cuttlefish configuration (RabbitMQ).
	// +optional
	Data string `json:"data,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// MQConfigurationStatus defines the observed state of MQConfiguration.
type MQConfigurationStatus struct {
	// ConfigurationID is the unique ID Amazon MQ generates for the configuration.
	// +optional
	ConfigurationID string `json:"configurationId,omitempty"`

	// ARN is the ARN of the configuration.
	// +optional
	ARN string `json:"arn,omitempty"`

	// LatestRevision is the latest revision number of the configuration.
	// +optional
	LatestRevision int32 `json:"latestRevision,omitempty"`

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
// +kubebuilder:printcolumn:name="Configuration-ID",type="string",JSONPath=".status.configurationId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// MQConfiguration is the Schema for managing Amazon MQ configurations.
type MQConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MQConfigurationSpec   `json:"spec,omitempty"`
	Status MQConfigurationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MQConfigurationList contains a list of MQConfiguration
type MQConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MQConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MQConfiguration{}, &MQConfigurationList{})
}
