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

// NatGatewaySpec defines the desired state of an AWS NAT Gateway.
type NatGatewaySpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// SubnetRef references the subnet in which to create the NAT gateway.
	// Must be a public subnet for connectivity-type public.
	SubnetRef SubnetRef `json:"subnetRef"`

	// ConnectivityType is "public" or "private". Default is "public".
	// +kubebuilder:validation:Enum=public;private
	// +optional
	ConnectivityType string `json:"connectivityType,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// NatGatewayStatus defines the observed state of NatGateway.
type NatGatewayStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// NatGatewayID is the AWS NAT Gateway identifier.
	// +optional
	NatGatewayID string `json:"natGatewayId,omitempty"`

	// ElasticIPAllocationID is the allocation ID of the Elastic IP (public NAT only).
	// +optional
	ElasticIPAllocationID string `json:"elasticIpAllocationId,omitempty"`

	// State is the current NAT Gateway state (pending | available | deleting | deleted).
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
// +kubebuilder:printcolumn:name="NAT-ID",type="string",JSONPath=".status.natGatewayId"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// NatGateway is the Schema for managing AWS NAT Gateways.
type NatGateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NatGatewaySpec   `json:"spec,omitempty"`
	Status NatGatewayStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NatGatewayList contains a list of NatGateway
type NatGatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NatGateway `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NatGateway{}, &NatGatewayList{})
}
