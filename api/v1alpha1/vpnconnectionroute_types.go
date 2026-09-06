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

// VPNConnectionRef references a managed VPNConnection CR or a direct VPN
// connection ID.
type VPNConnectionRef struct {
	// Name of a VPNConnection CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ID is a direct AWS VPN connection ID (vpn-...). If set, Name is ignored.
	// +optional
	ID string `json:"id,omitempty"`
}

// VPNConnectionRouteSpec defines the desired state of a static route on a
// VPN connection.
type VPNConnectionRouteSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// VPNConnectionRef references the VPN connection to add the route to.
	VPNConnectionRef VPNConnectionRef `json:"vpnConnectionRef"`

	// DestinationCIDRBlock is the CIDR block associated with the local subnet
	// of the customer network. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="destinationCidrBlock is immutable"
	DestinationCIDRBlock string `json:"destinationCidrBlock"`
}

// VPNConnectionRouteStatus defines the observed state of VPNConnectionRoute.
type VPNConnectionRouteStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// VPNConnectionID is the resolved VPN connection the route was created on.
	// +optional
	VPNConnectionID string `json:"vpnConnectionId,omitempty"`

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
// +kubebuilder:printcolumn:name="VPN",type="string",JSONPath=".status.vpnConnectionId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// VPNConnectionRoute is the Schema for managing static routes on EC2 VPN connections.
type VPNConnectionRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VPNConnectionRouteSpec   `json:"spec,omitempty"`
	Status VPNConnectionRouteStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VPNConnectionRouteList contains a list of VPNConnectionRoute
type VPNConnectionRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VPNConnectionRoute `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VPNConnectionRoute{}, &VPNConnectionRouteList{})
}
