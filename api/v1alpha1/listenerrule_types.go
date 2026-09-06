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

// ListenerRef references either a managed Listener CR or a direct ARN.
type ListenerRef struct {
	// Name of a Listener CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ARN is a direct AWS listener ARN.
	// +optional
	ARN string `json:"arn,omitempty"`
}

// RuleCondition defines a condition for a listener rule.
type RuleCondition struct {
	// Field is the condition field (e.g. "host-header", "path-pattern", "http-header").
	Field string `json:"field"`
	// Values are the values to match.
	// +optional
	Values []string `json:"values,omitempty"`
}

// ListenerRuleSpec defines the desired state of a Listener Rule.
type ListenerRuleSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ListenerRef references the Listener.
	ListenerRef ListenerRef `json:"listenerRef"`

	// Priority is the rule priority (1-50000).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=50000
	Priority int32 `json:"priority"`

	// Conditions are the rule conditions.
	// +kubebuilder:validation:MinItems=1
	Conditions []RuleCondition `json:"conditions"`

	// Actions are the rule actions (same structure as listener default actions).
	// +kubebuilder:validation:MinItems=1
	Actions []ListenerDefaultAction `json:"actions"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ListenerRuleStatus defines the observed state of ListenerRule.
type ListenerRuleStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ARN is the ARN of the listener rule.
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
// +kubebuilder:printcolumn:name="Priority",type="integer",JSONPath=".spec.priority"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ListenerRule is the Schema for managing ELBv2 Listener Rules.
type ListenerRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ListenerRuleSpec   `json:"spec,omitempty"`
	Status ListenerRuleStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ListenerRuleList contains a list of ListenerRule
type ListenerRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ListenerRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ListenerRule{}, &ListenerRuleList{})
}
