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

// SGRule defines a single security group ingress or egress rule.
type SGRule struct {
	// Protocol is the IP protocol (tcp, udp, icmp, or -1 for all).
	Protocol string `json:"protocol"`

	// FromPort is the start of the port range. Use -1 for all protocols.
	// +optional
	FromPort int32 `json:"fromPort,omitempty"`

	// ToPort is the end of the port range. Use -1 for all protocols.
	// +optional
	ToPort int32 `json:"toPort,omitempty"`

	// CIDRIPv4 is the source/destination IPv4 CIDR range.
	// +optional
	CIDRIPv4 string `json:"cidrIpv4,omitempty"`

	// SourceGroupRef is the name of a SecurityGroup CR in the same namespace
	// to use as the source/destination (instead of a CIDR).
	// +optional
	SourceGroupRef string `json:"sourceGroupRef,omitempty"`

	// CIDRIPv6 is the source/destination IPv6 CIDR range.
	// +optional
	CIDRIPv6 string `json:"cidrIpv6,omitempty"`

	// PrefixListID is the ID of a managed prefix list to use as source/destination.
	// +optional
	PrefixListID string `json:"prefixListId,omitempty"`

	// Description is a description for the rule.
	// +optional
	Description string `json:"description,omitempty"`
}

// SecurityGroupSpec defines the desired state of an AWS Security Group.
type SecurityGroupSpec struct {
	// VPCRef references the VPC this security group belongs to.
	VPCRef VPCResourceRef `json:"vpcRef"`

	// GroupName is the name of the security group. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="groupName is immutable"
	GroupName string `json:"groupName"`

	// Description is the description for the security group.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="description is immutable"
	Description string `json:"description"`

	// IngressRules defines the allowed inbound traffic.
	// +optional
	IngressRules []SGRule `json:"ingressRules,omitempty"`

	// EgressRules defines the allowed outbound traffic.
	// +optional
	EgressRules []SGRule `json:"egressRules,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// SecurityGroupStatus defines the observed state of SecurityGroup.
type SecurityGroupStatus struct {
	// GroupID is the AWS Security Group identifier.
	// +optional
	GroupID string `json:"groupId,omitempty"`

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
// +kubebuilder:printcolumn:name="SG-ID",type="string",JSONPath=".status.groupId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SecurityGroup is the Schema for managing AWS Security Groups.
type SecurityGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SecurityGroupSpec   `json:"spec,omitempty"`
	Status SecurityGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SecurityGroupList contains a list of SecurityGroup
type SecurityGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SecurityGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SecurityGroup{}, &SecurityGroupList{})
}
