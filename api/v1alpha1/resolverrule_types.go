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

// ResolverTargetIP defines a target IP for DNS forwarding.
type ResolverTargetIP struct {
	// IP is the IP address.
	IP string `json:"ip"`

	// Port is the port (default 53).
	// +optional
	Port *int32 `json:"port,omitempty"`
}

// ResolverRuleSpec defines the desired state of a Route53 Resolver rule.
type ResolverRuleSpec struct {
	// Name is the friendly name of the rule.
	Name string `json:"name"`

	// DomainName is the DNS domain that this rule applies to.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="domainName is immutable"
	DomainName string `json:"domainName"`

	// RuleType is FORWARD, SYSTEM, or RECURSIVE.
	// +kubebuilder:validation:Enum=FORWARD;SYSTEM;RECURSIVE
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="ruleType is immutable"
	RuleType string `json:"ruleType"`

	// ResolverEndpointID is the ID of the outbound endpoint for forwarding.
	// +optional
	ResolverEndpointID string `json:"resolverEndpointID,omitempty"`

	// TargetIPs are the DNS servers to forward queries to.
	// +optional
	TargetIPs []ResolverTargetIP `json:"targetIPs,omitempty"`

	// Tags are metadata tags for the rule.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ResolverRuleStatus defines the observed state of ResolverRule.
type ResolverRuleStatus struct {
	// RuleID is the unique identifier of the rule.
	// +optional
	RuleID string `json:"ruleID,omitempty"`

	// RuleARN is the ARN of the rule.
	// +optional
	RuleARN string `json:"ruleARN,omitempty"`

	// Status is the current status of the rule.
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
// +kubebuilder:printcolumn:name="RuleID",type="string",JSONPath=".status.ruleID"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ResolverRule is the Schema for managing Route53 Resolver forwarding rules.
type ResolverRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResolverRuleSpec   `json:"spec,omitempty"`
	Status ResolverRuleStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ResolverRuleList contains a list of ResolverRule.
type ResolverRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResolverRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ResolverRule{}, &ResolverRuleList{})
}
