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

// StatelessRuleGroupRef references a stateless FirewallRuleGroup CR or a
// direct rule group ARN, with the evaluation priority within the policy.
type StatelessRuleGroupRef struct {
	// Name of a FirewallRuleGroup CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ARN is a direct AWS rule group ARN. If set, Name is ignored.
	// +optional
	ARN string `json:"arn,omitempty"`
	// Priority is the order in which to run this rule group. Must be unique
	// within the policy.
	// +kubebuilder:validation:Minimum=1
	Priority int32 `json:"priority"`
}

// StatefulRuleGroupRef references a stateful FirewallRuleGroup CR or a
// direct rule group ARN.
type StatefulRuleGroupRef struct {
	// Name of a FirewallRuleGroup CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ARN is a direct AWS rule group ARN. If set, Name is ignored.
	// +optional
	ARN string `json:"arn,omitempty"`
}

// FirewallPolicySpec defines the desired state of an AWS Network Firewall policy.
type FirewallPolicySpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the descriptive name of the firewall policy. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// StatelessDefaultActions are the actions for packets that don't match any
	// stateless rule (e.g. aws:pass, aws:drop, aws:forward_to_sfe).
	// +kubebuilder:validation:MinItems=1
	StatelessDefaultActions []string `json:"statelessDefaultActions"`

	// StatelessFragmentDefaultActions are the actions for fragmented UDP packets
	// that don't match any stateless rule.
	// +kubebuilder:validation:MinItems=1
	StatelessFragmentDefaultActions []string `json:"statelessFragmentDefaultActions"`

	// StatelessRuleGroupRefs references the stateless rule groups in the policy.
	// +optional
	StatelessRuleGroupRefs []StatelessRuleGroupRef `json:"statelessRuleGroupRefs,omitempty"`

	// StatefulRuleGroupRefs references the stateful rule groups in the policy.
	// +optional
	StatefulRuleGroupRefs []StatefulRuleGroupRef `json:"statefulRuleGroupRefs,omitempty"`

	// Description of the firewall policy.
	// +optional
	Description string `json:"description,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// FirewallPolicyStatus defines the observed state of FirewallPolicy.
type FirewallPolicyStatus struct {
	// ARN is the Amazon Resource Name of the firewall policy.
	// +optional
	ARN string `json:"arn,omitempty"`

	// ID is the unique identifier of the firewall policy.
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

// FirewallPolicy is the Schema for managing AWS Network Firewall policies.
type FirewallPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FirewallPolicySpec   `json:"spec,omitempty"`
	Status FirewallPolicyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// FirewallPolicyList contains a list of FirewallPolicy
type FirewallPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FirewallPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FirewallPolicy{}, &FirewallPolicyList{})
}
