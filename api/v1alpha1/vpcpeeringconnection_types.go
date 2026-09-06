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

// VPCPeeringConnectionSpec defines the desired state of a VPC Peering Connection.
type VPCPeeringConnectionSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// VPCRef is the requester VPC (local).
	VPCRef VPCResourceRef `json:"vpcRef"`

	// PeerVPCID is the accepter VPC ID.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="peerVpcId is immutable"
	PeerVPCID string `json:"peerVpcId"`

	// PeerOwnerID is the AWS account ID of the accepter VPC owner.
	// Leave empty for same-account peering.
	// +optional
	PeerOwnerID string `json:"peerOwnerId,omitempty"`

	// PeerRegion is the region of the accepter VPC.
	// Leave empty for same-region peering.
	// +optional
	PeerRegion string `json:"peerRegion,omitempty"`

	// AutoAccept makes the controller accept the peering request itself. For
	// same-account peers this uses the resource's own provider (in PeerRegion
	// when set). For cross-account peers set AccepterProviderRef as well so the
	// acceptance runs with the accepter account's credentials.
	// +optional
	AutoAccept bool `json:"autoAccept,omitempty"`

	// AccepterProviderRef names the AWSProvider for the accepter VPC's account.
	// When set the controller accepts the request on the peer's behalf and
	// Ready only becomes True once the connection is active on both sides.
	// +optional
	AccepterProviderRef *ProviderRef `json:"accepterProviderRef,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// VPCPeeringConnectionStatus defines the observed state of VPCPeeringConnection.
type VPCPeeringConnectionStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// PeeringID is the VPC peering connection ID.
	// +optional
	PeeringID string `json:"peeringId,omitempty"`

	// RequesterVPCID and AccepterVPCID are the two VPCs as reported by AWS.
	// +optional
	RequesterVPCID string `json:"requesterVpcId,omitempty"`
	// +optional
	AccepterVPCID string `json:"accepterVpcId,omitempty"`
	// AccepterAccountID is the account that owns the accepter VPC.
	// +optional
	AccepterAccountID string `json:"accepterAccountId,omitempty"`

	// Status is the current state of the peering connection.
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
// +kubebuilder:printcolumn:name="PeeringID",type="string",JSONPath=".status.peeringId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// STATUS: WIP. The cross-account path of this kind (acting under another
// account's AWSProvider) is implemented and unit-tested but has not yet been
// verified against a second live AWS account. Same-account use is verified.
// VPCPeeringConnection is the Schema for managing VPC Peering Connections.
type VPCPeeringConnection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VPCPeeringConnectionSpec   `json:"spec,omitempty"`
	Status VPCPeeringConnectionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VPCPeeringConnectionList contains a list of VPCPeeringConnection.
type VPCPeeringConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VPCPeeringConnection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VPCPeeringConnection{}, &VPCPeeringConnectionList{})
}
