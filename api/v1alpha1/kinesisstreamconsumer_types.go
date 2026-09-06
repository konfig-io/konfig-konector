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

// KinesisStreamConsumerSpec defines the desired state of a Kinesis enhanced fan-out consumer.
type KinesisStreamConsumerSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ConsumerName is the name of the consumer. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="consumerName is immutable"
	ConsumerName string `json:"consumerName"`

	// StreamARN is the ARN of the Kinesis stream to register the consumer on.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="streamARN is immutable"
	StreamARN string `json:"streamARN"`
}

// KinesisStreamConsumerStatus defines the observed state of KinesisStreamConsumer.
type KinesisStreamConsumerStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ConsumerARN is the ARN of the registered consumer.
	// +optional
	ConsumerARN string `json:"consumerARN,omitempty"`

	// ConsumerStatus is the current status (CREATING, DELETING, ACTIVE).
	// +optional
	ConsumerStatus string `json:"consumerStatus,omitempty"`

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
// +kubebuilder:printcolumn:name="ConsumerARN",type="string",JSONPath=".status.consumerARN"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.consumerStatus"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// KinesisStreamConsumer is the Schema for managing Kinesis enhanced fan-out consumers.
type KinesisStreamConsumer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KinesisStreamConsumerSpec   `json:"spec,omitempty"`
	Status KinesisStreamConsumerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KinesisStreamConsumerList contains a list of KinesisStreamConsumer.
type KinesisStreamConsumerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KinesisStreamConsumer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KinesisStreamConsumer{}, &KinesisStreamConsumerList{})
}
