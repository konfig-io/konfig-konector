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

// LambdaFilterCriteria defines a filter pattern for event source mappings.
type LambdaFilterCriteria struct {
	// Filters are the filter patterns (JSON pattern strings).
	// +optional
	Filters []string `json:"filters,omitempty"`
}

// LambdaEventSourceMappingSpec defines the desired state of a Lambda Event Source Mapping.
type LambdaEventSourceMappingSpec struct {
	// FunctionRef references the Lambda function to invoke.
	// Either functionRef or functionArn must be set.
	// +optional
	FunctionRef *LambdaFunctionRef `json:"functionRef,omitempty"`

	// FunctionArn is a direct Lambda function ARN or name.
	// +optional
	FunctionArn string `json:"functionArn,omitempty"`

	// EventSourceArn is the ARN of the event source (SQS queue, Kinesis stream, DynamoDB stream, etc.).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="eventSourceArn is immutable"
	EventSourceArn string `json:"eventSourceArn"`

	// BatchSize is the maximum number of records in each batch. Default varies by source.
	// +optional
	BatchSize *int32 `json:"batchSize,omitempty"`

	// Enabled controls whether the event source mapping is active. Default: true.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// StartingPosition is where to start reading from a stream.
	// +kubebuilder:validation:Enum=TRIM_HORIZON;LATEST;AT_TIMESTAMP
	// +optional
	StartingPosition string `json:"startingPosition,omitempty"`

	// MaximumBatchingWindowInSeconds is the maximum time to gather records before invoking the function.
	// +optional
	MaximumBatchingWindowInSeconds *int32 `json:"maximumBatchingWindowInSeconds,omitempty"`

	// FilterCriteria filters which events invoke the function.
	// +optional
	FilterCriteria *LambdaFilterCriteria `json:"filterCriteria,omitempty"`
}

// LambdaEventSourceMappingStatus defines the observed state of LambdaEventSourceMapping.
type LambdaEventSourceMappingStatus struct {
	// UUID is the identifier of the event source mapping.
	// +optional
	UUID string `json:"uuid,omitempty"`

	// State is the event source mapping state: Creating, Enabled, Disabled, Disabling, Enabling, Updating, Deleting.
	// +optional
	State string `json:"state,omitempty"`

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
// +kubebuilder:printcolumn:name="Function",type="string",JSONPath=".spec.functionArn"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// LambdaEventSourceMapping is the Schema for managing Lambda Event Source Mappings.
type LambdaEventSourceMapping struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LambdaEventSourceMappingSpec   `json:"spec,omitempty"`
	Status LambdaEventSourceMappingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LambdaEventSourceMappingList contains a list of LambdaEventSourceMapping.
type LambdaEventSourceMappingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LambdaEventSourceMapping `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LambdaEventSourceMapping{}, &LambdaEventSourceMappingList{})
}
