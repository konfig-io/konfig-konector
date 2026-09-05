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

// VPNGatewaySpec defines the desired state of an EC2 virtual private gateway.
type VPNGatewaySpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Type of VPN connection the virtual private gateway supports.
	// +kubebuilder:validation:Enum=ipsec.1
	// +kubebuilder:default="ipsec.1"
	// +optional
	Type string `json:"type,omitempty"`

	// AmazonSideASN is the private Autonomous System Number for the Amazon
	// side of a BGP session.
	// +optional
	AmazonSideASN int64 `json:"amazonSideAsn,omitempty"`

	// VPCRef references the VPC to attach the gateway to. When unset the
	// gateway is left detached.
	// +optional
	VPCRef *VPCResourceRef `json:"vpcRef,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// VPNGatewayStatus defines the observed state of VPNGateway.
type VPNGatewayStatus struct {
	// VPNGatewayID is the AWS virtual private gateway identifier.
	// +optional
	VPNGatewayID string `json:"vpnGatewayId,omitempty"`

	// AttachedVPCID is the VPC the gateway is currently attached to.
	// +optional
	AttachedVPCID string `json:"attachedVpcId,omitempty"`

	// State is the lifecycle state reported by AWS.
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
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.vpnGatewayId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// VPNGateway is the Schema for managing EC2 virtual private gateways.
type VPNGateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VPNGatewaySpec   `json:"spec,omitempty"`
	Status VPNGatewayStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VPNGatewayList contains a list of VPNGateway
type VPNGatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VPNGateway `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VPNGateway{}, &VPNGatewayList{})
}
