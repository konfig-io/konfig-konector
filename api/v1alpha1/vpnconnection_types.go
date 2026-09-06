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

// CustomerGatewayRef references a managed CustomerGateway CR or a direct
// customer gateway ID.
type CustomerGatewayRef struct {
	// Name of a CustomerGateway CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ID is a direct AWS customer gateway ID (cgw-...). If set, Name is ignored.
	// +optional
	ID string `json:"id,omitempty"`
}

// VPNGatewayRef references a managed VPNGateway CR or a direct virtual
// private gateway ID.
type VPNGatewayRef struct {
	// Name of a VPNGateway CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ID is a direct AWS virtual private gateway ID (vgw-...). If set, Name is ignored.
	// +optional
	ID string `json:"id,omitempty"`
}

// VPNTunnelOptions configures one of the two VPN tunnels.
type VPNTunnelOptions struct {
	// InsideCIDR is the range of inside IP addresses for the tunnel
	// (a /30 CIDR block from the 169.254.0.0/16 range).
	// +optional
	InsideCIDR string `json:"insideCidr,omitempty"`

	// PreSharedKeyRef references a Kubernetes Secret holding the tunnel's
	// pre-shared key. The key material is never written to status or logs.
	// +optional
	PreSharedKeyRef *SecretRef `json:"presharedKeyRef,omitempty"`
}

// VPNConnectionSpec defines the desired state of an EC2 VPN connection.
type VPNConnectionSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// CustomerGatewayRef references the customer gateway.
	CustomerGatewayRef CustomerGatewayRef `json:"customerGatewayRef"`

	// VPNGatewayRef references the virtual private gateway. Exactly one of
	// VPNGatewayRef or TransitGatewayRef must be set.
	// +optional
	VPNGatewayRef *VPNGatewayRef `json:"vpnGatewayRef,omitempty"`

	// TransitGatewayRef references the transit gateway. Exactly one of
	// VPNGatewayRef or TransitGatewayRef must be set.
	// +optional
	TransitGatewayRef *TransitGatewayRef `json:"transitGatewayRef,omitempty"`

	// Type of VPN connection.
	// +kubebuilder:validation:Enum=ipsec.1
	// +kubebuilder:default="ipsec.1"
	// +optional
	Type string `json:"type,omitempty"`

	// StaticRoutesOnly indicates the VPN connection uses static routes only
	// (required when the customer gateway device does not support BGP).
	// +optional
	StaticRoutesOnly bool `json:"staticRoutesOnly,omitempty"`

	// TunnelOptions configures the VPN tunnels (up to 2).
	// +kubebuilder:validation:MaxItems=2
	// +optional
	TunnelOptions []VPNTunnelOptions `json:"tunnelOptions,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// VPNConnectionStatus defines the observed state of VPNConnection.
type VPNConnectionStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// VPNConnectionID is the AWS VPN connection identifier.
	// +optional
	VPNConnectionID string `json:"vpnConnectionId,omitempty"`

	// State is the lifecycle state reported by AWS (pending, available,
	// deleting, deleted).
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
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.vpnConnectionId"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// VPNConnection is the Schema for managing EC2 VPN connections.
type VPNConnection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VPNConnectionSpec   `json:"spec,omitempty"`
	Status VPNConnectionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VPNConnectionList contains a list of VPNConnection
type VPNConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VPNConnection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VPNConnection{}, &VPNConnectionList{})
}
