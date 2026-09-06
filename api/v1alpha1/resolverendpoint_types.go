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

// ResolverIPAddress defines an IP address for a resolver endpoint.
type ResolverIPAddress struct {
	// SubnetID is the subnet for this IP address.
	SubnetID string `json:"subnetID"`

	// IP is the specific IP address (optional; auto-assigned if omitted).
	// +optional
	IP string `json:"ip,omitempty"`
}

// ResolverEndpointSpec defines the desired state of a Route53 Resolver endpoint.
type ResolverEndpointSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the friendly name of the endpoint.
	Name string `json:"name"`

	// Direction is INBOUND or OUTBOUND.
	// +kubebuilder:validation:Enum=INBOUND;OUTBOUND
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="direction is immutable"
	Direction string `json:"direction"`

	// SecurityGroupIDs is the list of security group IDs.
	SecurityGroupIDs []string `json:"securityGroupIDs"`

	// IPAddresses is the list of IP address objects.
	IPAddresses []ResolverIPAddress `json:"ipAddresses"`

	// Tags are metadata tags for the endpoint.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ResolverEndpointStatus defines the observed state of ResolverEndpoint.
type ResolverEndpointStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// EndpointID is the unique identifier of the endpoint.
	// +optional
	EndpointID string `json:"endpointID,omitempty"`

	// HostVPCID is the VPC that hosts the endpoint.
	// +optional
	HostVPCID string `json:"hostVpcID,omitempty"`

	// Status is the current status (CREATING, OPERATIONAL, DELETING, etc.).
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
// +kubebuilder:printcolumn:name="EndpointID",type="string",JSONPath=".status.endpointID"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ResolverEndpoint is the Schema for managing Route53 Resolver endpoints.
type ResolverEndpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResolverEndpointSpec   `json:"spec,omitempty"`
	Status ResolverEndpointStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ResolverEndpointList contains a list of ResolverEndpoint.
type ResolverEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResolverEndpoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ResolverEndpoint{}, &ResolverEndpointList{})
}
