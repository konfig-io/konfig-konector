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

// ConfigDeliveryChannelSpec defines the desired state of an AWS Config delivery channel.
type ConfigDeliveryChannelSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ChannelName is the name of the delivery channel. AWS uses "default" by
	// convention. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="channelName is immutable"
	ChannelName string `json:"channelName"`

	// S3BucketName is the S3 bucket to which Config delivers configuration
	// snapshots and history files.
	// +kubebuilder:validation:MinLength=1
	S3BucketName string `json:"s3BucketName"`

	// S3KeyPrefix is the prefix for the S3 bucket.
	// +optional
	S3KeyPrefix string `json:"s3KeyPrefix,omitempty"`

	// SNSTopicARN is the SNS topic to which Config sends notifications.
	// +optional
	SNSTopicARN string `json:"snsTopicARN,omitempty"`

	// DeliveryFrequency is how often Config delivers configuration snapshots.
	// +kubebuilder:validation:Enum=One_Hour;Three_Hours;Six_Hours;Twelve_Hours;TwentyFour_Hours
	// +optional
	DeliveryFrequency string `json:"deliveryFrequency,omitempty"`
}

// ConfigDeliveryChannelStatus defines the observed state of ConfigDeliveryChannel.
type ConfigDeliveryChannelStatus struct {
	// ChannelName is the name of the delivery channel in AWS.
	// +optional
	ChannelName string `json:"channelName,omitempty"`

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
// +kubebuilder:printcolumn:name="Channel",type="string",JSONPath=".status.channelName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ConfigDeliveryChannel is the Schema for managing AWS Config delivery channels.
type ConfigDeliveryChannel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfigDeliveryChannelSpec   `json:"spec,omitempty"`
	Status ConfigDeliveryChannelStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ConfigDeliveryChannelList contains a list of ConfigDeliveryChannel
type ConfigDeliveryChannelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConfigDeliveryChannel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConfigDeliveryChannel{}, &ConfigDeliveryChannelList{})
}
