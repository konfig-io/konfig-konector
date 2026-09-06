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

// EventBridgePipeSpec defines the desired state of an EventBridge Pipe.
type EventBridgePipeSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the name of the pipe.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// RoleARN is the ARN of the IAM role that allows the pipe to send data to the target.
	RoleARN string `json:"roleArn"`

	// Source is the ARN of the source resource.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="source is immutable"
	Source string `json:"source"`

	// Target is the ARN of the target resource.
	Target string `json:"target"`

	// Description is an optional description of the pipe.
	// +optional
	Description string `json:"description,omitempty"`

	// DesiredState is the state the pipe should be in.
	// +kubebuilder:validation:Enum=RUNNING;STOPPED
	// +optional
	DesiredState string `json:"desiredState,omitempty"`

	// Enrichment is the ARN of the enrichment resource.
	// +optional
	Enrichment string `json:"enrichment,omitempty"`

	// Tags are metadata tags for the pipe.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// EventBridgePipeStatus defines the observed state of EventBridgePipe.
type EventBridgePipeStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// PipeARN is the ARN of the pipe.
	// +optional
	PipeARN string `json:"pipeArn,omitempty"`

	// PipeState is the current state of the pipe.
	// +optional
	PipeState string `json:"pipeState,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.pipeArn"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.pipeState"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EventBridgePipe is the Schema for managing EventBridge Pipes.
type EventBridgePipe struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EventBridgePipeSpec   `json:"spec,omitempty"`
	Status EventBridgePipeStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EventBridgePipeList contains a list of EventBridgePipe.
type EventBridgePipeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EventBridgePipe `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EventBridgePipe{}, &EventBridgePipeList{})
}
