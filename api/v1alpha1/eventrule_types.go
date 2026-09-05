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

// EventBusRef references either a managed EventBus CR or a direct bus name/ARN.
type EventBusRef struct {
	// Name of an EventBus CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// EventBusName is a direct AWS event bus name or ARN.
	// +optional
	EventBusName string `json:"eventBusName,omitempty"`
}

// EventRuleSpec defines the desired state of an EventBridge Rule.
type EventRuleSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// EventBusRef references the event bus. Omit for the default bus.
	// +optional
	EventBusRef *EventBusRef `json:"eventBusRef,omitempty"`

	// RuleName is the name of the rule. Immutable.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="ruleName is immutable"
	RuleName string `json:"ruleName"`

	// Description is a human-readable description.
	// +optional
	Description string `json:"description,omitempty"`

	// EventPattern is the JSON event pattern.
	// +optional
	EventPattern string `json:"eventPattern,omitempty"`

	// ScheduleExpression is a cron or rate schedule expression.
	// +optional
	ScheduleExpression string `json:"scheduleExpression,omitempty"`

	// State enables or disables the rule.
	// +kubebuilder:validation:Enum=ENABLED;DISABLED
	// +optional
	State string `json:"state,omitempty"`

	// RoleARN is the IAM role ARN for the rule to invoke targets.
	// +optional
	RoleARN string `json:"roleArn,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// EventRuleStatus defines the observed state of EventRule.
type EventRuleStatus struct {
	// ARN is the ARN of the rule.
	// +optional
	ARN string `json:"arn,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.arn"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".spec.state"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EventRule is the Schema for managing EventBridge Rules.
type EventRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EventRuleSpec   `json:"spec,omitempty"`
	Status EventRuleStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EventRuleList contains a list of EventRule
type EventRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EventRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EventRule{}, &EventRuleList{})
}
