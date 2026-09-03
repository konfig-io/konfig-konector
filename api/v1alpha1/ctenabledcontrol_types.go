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

// CTEnabledControlSpec defines the desired state of an AWS Control Tower
// enabled control. Enable/disable operations are asynchronous; the controller
// polls GetControlOperation. Only reconciles successfully from the Control
// Tower management account.
type CTEnabledControlSpec struct {
	// ControlIdentifier is the ARN of the control to enable. Immutable after
	// creation.
	// +kubebuilder:validation:MinLength=10
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="controlIdentifier is immutable"
	ControlIdentifier string `json:"controlIdentifier"`

	// TargetIdentifier is the ARN of the organizational unit the control is
	// enabled on. Immutable after creation.
	// +kubebuilder:validation:MinLength=10
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="targetIdentifier is immutable"
	TargetIdentifier string `json:"targetIdentifier"`

	// Tags are AWS resource tags to apply to the EnabledControl resource.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// CTEnabledControlStatus defines the observed state of CTEnabledControl.
type CTEnabledControlStatus struct {
	// ARN is the Amazon Resource Name of the EnabledControl resource.
	// +optional
	ARN string `json:"arn,omitempty"`

	// OperationIdentifier tracks the in-flight asynchronous enable operation.
	// +optional
	OperationIdentifier string `json:"operationIdentifier,omitempty"`

	// State is the last observed control operation status
	// (IN_PROGRESS, SUCCEEDED, or FAILED).
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
// +kubebuilder:printcolumn:name="Control-ARN",type="string",JSONPath=".status.arn"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CTEnabledControl is the Schema for managing AWS Control Tower enabled controls.
type CTEnabledControl struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CTEnabledControlSpec   `json:"spec,omitempty"`
	Status CTEnabledControlStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CTEnabledControlList contains a list of CTEnabledControl
type CTEnabledControlList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CTEnabledControl `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CTEnabledControl{}, &CTEnabledControlList{})
}
