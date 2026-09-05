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

// StateMachineSpec defines the desired state of a Step Functions state machine.
type StateMachineSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the name of the state machine. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Definition is the Amazon States Language definition (JSON string).
	Definition string `json:"definition"`

	// RoleARN is the IAM role ARN for executing the state machine.
	RoleARN string `json:"roleARN"`

	// Type is STANDARD or EXPRESS.
	// +kubebuilder:validation:Enum=STANDARD;EXPRESS
	// +optional
	Type string `json:"type,omitempty"`

	// Tags are metadata tags for the state machine.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// StateMachineStatus defines the observed state of StateMachine.
type StateMachineStatus struct {
	// StateMachineARN is the ARN of the state machine.
	// +optional
	StateMachineARN string `json:"stateMachineARN,omitempty"`

	// Status is the current status of the state machine (ACTIVE, DELETING).
	// +optional
	Status string `json:"status,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.stateMachineARN"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// StateMachine is the Schema for managing Step Functions state machines.
type StateMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StateMachineSpec   `json:"spec,omitempty"`
	Status StateMachineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// StateMachineList contains a list of StateMachine.
type StateMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StateMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StateMachine{}, &StateMachineList{})
}
