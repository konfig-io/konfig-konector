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

// CustomerGatewaySpec defines the desired state of an EC2 customer gateway.
type CustomerGatewaySpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// BGPASN is the customer gateway device's Border Gateway Protocol
	// Autonomous System Number. Immutable after creation.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="bgpAsn is immutable"
	BGPASN int32 `json:"bgpAsn"`

	// IPAddress is the internet-routable IP address of the customer gateway
	// device's outside interface. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="ipAddress is immutable"
	IPAddress string `json:"ipAddress"`

	// Type of VPN connection this customer gateway supports.
	// +kubebuilder:validation:Enum=ipsec.1
	// +kubebuilder:default="ipsec.1"
	// +optional
	Type string `json:"type,omitempty"`

	// DeviceName is a name for the customer gateway device.
	// +optional
	DeviceName string `json:"deviceName,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// CustomerGatewayStatus defines the observed state of CustomerGateway.
type CustomerGatewayStatus struct {
	// CustomerGatewayID is the AWS customer gateway identifier.
	// +optional
	CustomerGatewayID string `json:"customerGatewayId,omitempty"`

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
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.customerGatewayId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CustomerGateway is the Schema for managing EC2 customer gateways.
type CustomerGateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CustomerGatewaySpec   `json:"spec,omitempty"`
	Status CustomerGatewayStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CustomerGatewayList contains a list of CustomerGateway
type CustomerGatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CustomerGateway `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CustomerGateway{}, &CustomerGatewayList{})
}
