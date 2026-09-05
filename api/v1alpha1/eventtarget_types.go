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

// EventRuleRef references either a managed EventRule CR or a direct rule name.
type EventRuleRef struct {
	// Name of an EventRule CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// RuleName is a direct AWS EventBridge rule name.
	// +optional
	RuleName string `json:"ruleName,omitempty"`
}

// EventTargetEntry defines a single target for an EventBridge rule.
type EventTargetEntry struct {
	// ID is the unique identifier for the target.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64
	ID string `json:"id"`
	// ARN is the ARN of the target resource.
	ARN string `json:"arn"`
	// RoleARN is the IAM role ARN to use for invoking this target.
	// +optional
	RoleARN string `json:"roleArn,omitempty"`
	// Input is a literal JSON string to pass to the target.
	// +optional
	Input string `json:"input,omitempty"`
	// InputPath is a JSONPath expression applied to the event.
	// +optional
	InputPath string `json:"inputPath,omitempty"`
}

// EventTargetSpec defines the desired state of an EventBridge Target set.
type EventTargetSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// EventRuleRef references the event rule.
	EventRuleRef EventRuleRef `json:"eventRuleRef"`

	// EventBusRef identifies the event bus of the rule. Omit for the default bus.
	// +optional
	EventBusRef *EventBusRef `json:"eventBusRef,omitempty"`

	// Targets are the targets to add to the rule.
	// +kubebuilder:validation:MinItems=1
	Targets []EventTargetEntry `json:"targets"`
}

// EventTargetStatus defines the observed state of EventTarget.
type EventTargetStatus struct {
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

// EventTarget is the Schema for managing EventBridge Targets.
type EventTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EventTargetSpec   `json:"spec,omitempty"`
	Status EventTargetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EventTargetList contains a list of EventTarget
type EventTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EventTarget `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EventTarget{}, &EventTargetList{})
}
