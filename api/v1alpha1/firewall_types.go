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

// FirewallPolicyRef references a managed FirewallPolicy CR or a direct
// firewall policy ARN.
type FirewallPolicyRef struct {
	// Name of a FirewallPolicy CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ARN is a direct AWS firewall policy ARN. If set, Name is ignored.
	// +optional
	ARN string `json:"arn,omitempty"`
}

// FirewallSpec defines the desired state of an AWS Network Firewall firewall.
type FirewallSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the descriptive name of the firewall. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// FirewallPolicyRef references the firewall policy to use.
	FirewallPolicyRef FirewallPolicyRef `json:"firewallPolicyRef"`

	// VPCRef references the VPC where the firewall is created.
	VPCRef VPCResourceRef `json:"vpcRef"`

	// SubnetRefs are the subnets to place firewall endpoints in.
	// +kubebuilder:validation:MinItems=1
	SubnetRefs []SubnetRef `json:"subnetRefs"`

	// DeleteProtection prevents deletion of the firewall while enabled.
	// +optional
	DeleteProtection bool `json:"deleteProtection,omitempty"`

	// Description of the firewall.
	// +optional
	Description string `json:"description,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// FirewallStatus defines the observed state of Firewall.
type FirewallStatus struct {
	// ARN is the Amazon Resource Name of the firewall.
	// +optional
	ARN string `json:"arn,omitempty"`

	// ID is the unique identifier of the firewall.
	// +optional
	ID string `json:"id,omitempty"`

	// State is the readiness state reported by AWS (PROVISIONING, READY, DELETING).
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
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Firewall is the Schema for managing AWS Network Firewall firewalls.
type Firewall struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FirewallSpec   `json:"spec,omitempty"`
	Status FirewallStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// FirewallList contains a list of Firewall
type FirewallList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Firewall `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Firewall{}, &FirewallList{})
}
