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

// FirewallRuleGroupSpec defines the desired state of an AWS Network Firewall rule group.
type FirewallRuleGroupSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the descriptive name of the rule group. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Type indicates whether the rule group is stateless or stateful.
	// Immutable after creation.
	// +kubebuilder:validation:Enum=STATELESS;STATEFUL
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="type is immutable"
	Type string `json:"type"`

	// Capacity is the maximum operating resources this rule group can use.
	// Immutable after creation.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="capacity is immutable"
	Capacity int32 `json:"capacity"`

	// RulesString contains the rule definitions in Suricata-compatible format
	// (stateful) or as a rules source string (stateless).
	// +optional
	RulesString string `json:"rulesString,omitempty"`

	// Description of the rule group.
	// +optional
	Description string `json:"description,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// FirewallRuleGroupStatus defines the observed state of FirewallRuleGroup.
type FirewallRuleGroupStatus struct {
	// ARN is the Amazon Resource Name of the rule group.
	// +optional
	ARN string `json:"arn,omitempty"`

	// ID is the unique identifier of the rule group.
	// +optional
	ID string `json:"id,omitempty"`

	// UpdateToken is the optimistic-locking token from the last read.
	// +optional
	UpdateToken string `json:"updateToken,omitempty"`

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
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// FirewallRuleGroup is the Schema for managing AWS Network Firewall rule groups.
type FirewallRuleGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FirewallRuleGroupSpec   `json:"spec,omitempty"`
	Status FirewallRuleGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// FirewallRuleGroupList contains a list of FirewallRuleGroup
type FirewallRuleGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FirewallRuleGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FirewallRuleGroup{}, &FirewallRuleGroupList{})
}
