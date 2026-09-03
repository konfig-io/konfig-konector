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

// SubscriptionFilterSpec defines the desired state of a Subscription Filter.
type SubscriptionFilterSpec struct {
	// LogGroupRef references the log group.
	LogGroupRef LogGroupRef `json:"logGroupRef"`

	// FilterName is the name of the subscription filter. Immutable.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="filterName is immutable"
	FilterName string `json:"filterName"`

	// FilterPattern is the CloudWatch Logs filter pattern.
	FilterPattern string `json:"filterPattern"`

	// DestinationARN is the ARN of the destination (Kinesis, Lambda, or Firehose).
	DestinationARN string `json:"destinationArn"`

	// RoleARN is the IAM role ARN for delivering logs to the destination (Kinesis only).
	// +optional
	RoleARN string `json:"roleArn,omitempty"`
}

// SubscriptionFilterStatus defines the observed state of SubscriptionFilter.
type SubscriptionFilterStatus struct {
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

// SubscriptionFilter is the Schema for managing CloudWatch Logs Subscription Filters.
type SubscriptionFilter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SubscriptionFilterSpec   `json:"spec,omitempty"`
	Status SubscriptionFilterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SubscriptionFilterList contains a list of SubscriptionFilter
type SubscriptionFilterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SubscriptionFilter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SubscriptionFilter{}, &SubscriptionFilterList{})
}
