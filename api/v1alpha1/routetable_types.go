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

// RouteEntry defines a single route in a route table.
type RouteEntry struct {
	// DestinationCIDR is the destination CIDR block (e.g. "0.0.0.0/0").
	DestinationCIDR string `json:"destinationCidr"`

	// GatewayID is a direct gateway ID (e.g. an IGW ID or "local").
	// +optional
	GatewayID string `json:"gatewayId,omitempty"`

	// NatGatewayRef is the name of a NatGateway CR in the same namespace.
	// +optional
	NatGatewayRef string `json:"natGatewayRef,omitempty"`
}

// RouteTableSpec defines the desired state of an AWS Route Table.
type RouteTableSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// VPCRef references the VPC this route table belongs to.
	VPCRef VPCResourceRef `json:"vpcRef"`

	// Routes is the list of routes to manage in this table.
	// +optional
	Routes []RouteEntry `json:"routes,omitempty"`

	// SubnetAssociations is a list of Subnet CR names (same namespace) to associate.
	// +optional
	SubnetAssociations []string `json:"subnetAssociations,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// RouteTableStatus defines the observed state of RouteTable.
type RouteTableStatus struct {
	// RouteTableID is the AWS Route Table identifier.
	// +optional
	RouteTableID string `json:"routeTableId,omitempty"`

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
// +kubebuilder:printcolumn:name="RTB-ID",type="string",JSONPath=".status.routeTableId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// RouteTable is the Schema for managing AWS Route Tables.
type RouteTable struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RouteTableSpec   `json:"spec,omitempty"`
	Status RouteTableStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RouteTableList contains a list of RouteTable
type RouteTableList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RouteTable `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RouteTable{}, &RouteTableList{})
}
