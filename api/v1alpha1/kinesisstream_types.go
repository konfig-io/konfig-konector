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

// KinesisStreamSpec defines the desired state of a Kinesis Data Stream.
type KinesisStreamSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// StreamName is the name of the stream. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="streamName is immutable"
	StreamName string `json:"streamName"`

	// ShardCount is the number of shards for PROVISIONED mode.
	// +optional
	ShardCount *int32 `json:"shardCount,omitempty"`

	// StreamModeDetails specifies ON_DEMAND or PROVISIONED mode.
	// +kubebuilder:validation:Enum=ON_DEMAND;PROVISIONED
	// +optional
	StreamMode string `json:"streamMode,omitempty"`

	// RetentionPeriodHours is the data retention period in hours (24–8760).
	// +optional
	RetentionPeriodHours *int32 `json:"retentionPeriodHours,omitempty"`

	// Tags are metadata tags for the stream.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// KinesisStreamStatus defines the observed state of KinesisStream.
type KinesisStreamStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// StreamARN is the ARN of the stream.
	// +optional
	StreamARN string `json:"streamARN,omitempty"`

	// StreamStatus is the current status of the stream (CREATING, DELETING, ACTIVE, UPDATING).
	// +optional
	StreamStatus string `json:"streamStatus,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.streamARN"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.streamStatus"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// KinesisStream is the Schema for managing Kinesis Data Streams.
type KinesisStream struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KinesisStreamSpec   `json:"spec,omitempty"`
	Status KinesisStreamStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KinesisStreamList contains a list of KinesisStream.
type KinesisStreamList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KinesisStream `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KinesisStream{}, &KinesisStreamList{})
}
