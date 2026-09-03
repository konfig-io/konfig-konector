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

// TransitGatewaySpec defines the desired state of a Transit Gateway.
type TransitGatewaySpec struct {
	// Description is a human-readable description for the transit gateway.
	// +optional
	Description string `json:"description,omitempty"`

	// AmazonSideASN is the private ASN for the Amazon side of a BGP session.
	// +kubebuilder:validation:Minimum=64512
	// +kubebuilder:validation:Maximum=65534
	// +optional
	AmazonSideASN int64 `json:"amazonSideAsn,omitempty"`

	// AutoAcceptSharedAttachments enables automatic acceptance of cross-account attachments.
	// +optional
	AutoAcceptSharedAttachments bool `json:"autoAcceptSharedAttachments,omitempty"`

	// DefaultRouteTableAssociation enables automatic association with the default route table.
	// +optional
	DefaultRouteTableAssociation bool `json:"defaultRouteTableAssociation,omitempty"`

	// DefaultRouteTablePropagation enables automatic propagation to the default route table.
	// +optional
	DefaultRouteTablePropagation bool `json:"defaultRouteTablePropagation,omitempty"`

	// DNSSupport enables DNS resolution for the transit gateway.
	// +optional
	DNSSupport bool `json:"dnsSupport,omitempty"`

	// VPNECMPSupport enables equal-cost multipath routing for VPN attachments.
	// +optional
	VPNECMPSupport bool `json:"vpnEcmpSupport,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// TransitGatewayStatus defines the observed state of TransitGateway.
type TransitGatewayStatus struct {
	// TransitGatewayID is the AWS Transit Gateway ID.
	// +optional
	TransitGatewayID string `json:"transitGatewayId,omitempty"`

	// ARN is the ARN of the transit gateway.
	// +optional
	ARN string `json:"arn,omitempty"`

	// State is the current state of the transit gateway.
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
// +kubebuilder:printcolumn:name="TGW-ID",type="string",JSONPath=".status.transitGatewayId"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// TransitGateway is the Schema for managing AWS Transit Gateways.
type TransitGateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TransitGatewaySpec   `json:"spec,omitempty"`
	Status TransitGatewayStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TransitGatewayList contains a list of TransitGateway
type TransitGatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TransitGateway `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TransitGateway{}, &TransitGatewayList{})
}
