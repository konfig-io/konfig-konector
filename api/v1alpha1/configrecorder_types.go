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

// ConfigRecordingGroup selects which resource types AWS Config records.
type ConfigRecordingGroup struct {
	// AllSupported records configuration changes for all supported regional
	// resource types. Mutually exclusive with ResourceTypes.
	// +optional
	AllSupported bool `json:"allSupported,omitempty"`

	// IncludeGlobalResourceTypes also records global resource types (IAM).
	// Requires AllSupported.
	// +optional
	IncludeGlobalResourceTypes bool `json:"includeGlobalResourceTypes,omitempty"`

	// ResourceTypes lists specific resource types to record
	// (e.g. AWS::EC2::Instance). Only used when AllSupported is false.
	// +optional
	ResourceTypes []string `json:"resourceTypes,omitempty"`
}

// ConfigRecorderSpec defines the desired state of an AWS Config configuration recorder.
type ConfigRecorderSpec struct {
	// RecorderName is the name of the configuration recorder. AWS uses
	// "default" by convention. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="recorderName is immutable"
	RecorderName string `json:"recorderName"`

	// RoleARN is the IAM role ARN assumed by AWS Config. Either RoleARN or
	// RoleRef must be set.
	// +optional
	RoleARN string `json:"roleARN,omitempty"`

	// RoleRef references an IAMRole CR whose ARN to use.
	// +optional
	RoleRef *RoleRef `json:"roleRef,omitempty"`

	// RecordingGroup selects which resource types are recorded.
	// +optional
	RecordingGroup *ConfigRecordingGroup `json:"recordingGroup,omitempty"`

	// Enabled starts (true) or stops (false) the configuration recorder.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`
}

// ConfigRecorderStatus defines the observed state of ConfigRecorder.
type ConfigRecorderStatus struct {
	// RecorderName is the name of the recorder in AWS.
	// +optional
	RecorderName string `json:"recorderName,omitempty"`

	// Recording reports whether the recorder is currently recording.
	// +optional
	Recording bool `json:"recording,omitempty"`

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
// +kubebuilder:printcolumn:name="Recorder",type="string",JSONPath=".status.recorderName"
// +kubebuilder:printcolumn:name="Recording",type="boolean",JSONPath=".status.recording"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ConfigRecorder is the Schema for managing AWS Config configuration recorders.
type ConfigRecorder struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfigRecorderSpec   `json:"spec,omitempty"`
	Status ConfigRecorderStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ConfigRecorderList contains a list of ConfigRecorder
type ConfigRecorderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConfigRecorder `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConfigRecorder{}, &ConfigRecorderList{})
}
