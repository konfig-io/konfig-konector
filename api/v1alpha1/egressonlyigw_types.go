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

// EgressOnlyIGWSpec defines the desired state of an Egress-Only Internet Gateway.
type EgressOnlyIGWSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// VPCRef is the VPC to attach the egress-only IGW to.
	VPCRef VPCResourceRef `json:"vpcRef"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// EgressOnlyIGWStatus defines the observed state of EgressOnlyIGW.
type EgressOnlyIGWStatus struct {
	// EgressOnlyIGWID is the ID of the egress-only internet gateway.
	// +optional
	EgressOnlyIGWID string `json:"egressOnlyIgwId,omitempty"`

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
// +kubebuilder:printcolumn:name="IGW-ID",type="string",JSONPath=".status.egressOnlyIgwId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EgressOnlyIGW is the Schema for managing Egress-Only Internet Gateways.
type EgressOnlyIGW struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EgressOnlyIGWSpec   `json:"spec,omitempty"`
	Status EgressOnlyIGWStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EgressOnlyIGWList contains a list of EgressOnlyIGW.
type EgressOnlyIGWList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EgressOnlyIGW `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EgressOnlyIGW{}, &EgressOnlyIGWList{})
}
