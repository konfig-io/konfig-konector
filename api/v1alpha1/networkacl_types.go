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

// NetworkACLEntry defines a single NACL rule.
type NetworkACLEntry struct {
	// RuleNumber is the rule priority (1-32766). Lower numbers are evaluated first.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=32766
	RuleNumber int32 `json:"ruleNumber"`

	// Protocol is the IP protocol number (-1 for all, 6 for TCP, 17 for UDP, 1 for ICMP).
	Protocol string `json:"protocol"`

	// RuleAction is "allow" or "deny".
	// +kubebuilder:validation:Enum=allow;deny
	RuleAction string `json:"ruleAction"`

	// Egress indicates whether this is an egress rule (true) or ingress rule (false).
	Egress bool `json:"egress"`

	// CIDRBlock is the IPv4 CIDR range.
	// +optional
	CIDRBlock string `json:"cidrBlock,omitempty"`

	// IPv6CIDRBlock is the IPv6 CIDR range.
	// +optional
	IPv6CIDRBlock string `json:"ipv6CidrBlock,omitempty"`

	// PortRange defines the allowed port range. Required for TCP/UDP.
	// +optional
	PortRange *NACLPortRange `json:"portRange,omitempty"`

	// ICMPTypeCode defines ICMP type and code. Required when protocol is 1.
	// +optional
	ICMPTypeCode *NACLICMPTypeCode `json:"icmpTypeCode,omitempty"`
}

// NACLPortRange defines a TCP/UDP port range.
type NACLPortRange struct {
	// From is the start of the port range.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=65535
	From int32 `json:"from"`
	// To is the end of the port range.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=65535
	To int32 `json:"to"`
}

// NACLICMPTypeCode defines ICMP type and code for a NACL rule.
type NACLICMPTypeCode struct {
	// Type is the ICMP type (-1 for all).
	Type int32 `json:"type"`
	// Code is the ICMP code (-1 for all).
	Code int32 `json:"code"`
}

// NetworkACLSpec defines the desired state of a Network ACL.
type NetworkACLSpec struct {
	// VPCRef references the VPC in which to create the NACL.
	VPCRef VPCResourceRef `json:"vpcRef"`

	// Entries are the NACL rules. Managed entries replace all non-default rules.
	// +optional
	Entries []NetworkACLEntry `json:"entries,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// NetworkACLStatus defines the observed state of NetworkACL.
type NetworkACLStatus struct {
	// NetworkACLID is the AWS NACL ID.
	// +optional
	NetworkACLID string `json:"networkAclId,omitempty"`

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
// +kubebuilder:printcolumn:name="NACL-ID",type="string",JSONPath=".status.networkAclId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// NetworkACL is the Schema for managing EC2 Network ACLs.
type NetworkACL struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkACLSpec   `json:"spec,omitempty"`
	Status NetworkACLStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NetworkACLList contains a list of NetworkACL
type NetworkACLList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkACL `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkACL{}, &NetworkACLList{})
}
