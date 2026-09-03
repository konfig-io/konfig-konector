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

// FirehoseS3Destination defines S3 destination configuration for a delivery stream.
type FirehoseS3Destination struct {
	// BucketARN is the ARN of the S3 bucket.
	BucketARN string `json:"bucketARN"`

	// RoleARN is the IAM role ARN that grants Firehose access to S3.
	RoleARN string `json:"roleARN"`

	// Prefix is the S3 prefix for delivered objects.
	// +optional
	Prefix string `json:"prefix,omitempty"`

	// BufferingIntervalSeconds is the buffering interval in seconds (60–900).
	// +optional
	BufferingIntervalSeconds *int32 `json:"bufferingIntervalSeconds,omitempty"`

	// BufferingSizeMBs is the buffering size in MBs (1–128).
	// +optional
	BufferingSizeMBs *int32 `json:"bufferingSizeMBs,omitempty"`
}

// FirehoseDeliveryStreamSpec defines the desired state of a Firehose delivery stream.
type FirehoseDeliveryStreamSpec struct {
	// DeliveryStreamName is the name of the delivery stream. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="deliveryStreamName is immutable"
	DeliveryStreamName string `json:"deliveryStreamName"`

	// DeliveryStreamType specifies the source type (DirectPut or KinesisStreamAsSource).
	// +kubebuilder:validation:Enum=DirectPut;KinesisStreamAsSource
	// +optional
	DeliveryStreamType string `json:"deliveryStreamType,omitempty"`

	// S3DestinationConfiguration defines an S3 destination.
	// +optional
	S3DestinationConfiguration *FirehoseS3Destination `json:"s3DestinationConfiguration,omitempty"`

	// Tags are metadata tags for the delivery stream.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// FirehoseDeliveryStreamStatus defines the observed state of FirehoseDeliveryStream.
type FirehoseDeliveryStreamStatus struct {
	// DeliveryStreamARN is the ARN of the delivery stream.
	// +optional
	DeliveryStreamARN string `json:"deliveryStreamARN,omitempty"`

	// DeliveryStreamStatus is the current status (CREATING, DELETING, ACTIVE).
	// +optional
	DeliveryStreamStatus string `json:"deliveryStreamStatus,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.deliveryStreamARN"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.deliveryStreamStatus"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// FirehoseDeliveryStream is the Schema for managing Kinesis Firehose delivery streams.
type FirehoseDeliveryStream struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FirehoseDeliveryStreamSpec   `json:"spec,omitempty"`
	Status FirehoseDeliveryStreamStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// FirehoseDeliveryStreamList contains a list of FirehoseDeliveryStream.
type FirehoseDeliveryStreamList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FirehoseDeliveryStream `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FirehoseDeliveryStream{}, &FirehoseDeliveryStreamList{})
}
