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

// VPCEndpointSpec defines the desired state of an AWS VPC Endpoint.
type VPCEndpointSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// VPCRef references the VPC for this endpoint.
	VPCRef VPCResourceRef `json:"vpcRef"`

	// ServiceName is the AWS service endpoint name (e.g. "com.amazonaws.us-east-1.s3").
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serviceName is immutable"
	ServiceName string `json:"serviceName"`

	// EndpointType is the type of endpoint: "Interface" or "Gateway".
	// +kubebuilder:validation:Enum=Interface;Gateway
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="endpointType is immutable"
	EndpointType string `json:"endpointType"`

	// RouteTableRefs is the list of RouteTable CR names (same namespace) for Gateway endpoints.
	// +optional
	RouteTableRefs []string `json:"routeTableRefs,omitempty"`

	// SubnetRefs is the list of subnet references for Interface endpoints.
	// +optional
	SubnetRefs []SubnetRef `json:"subnetRefs,omitempty"`

	// SecurityGroupRefs is the list of security group references for Interface endpoints.
	// +optional
	SecurityGroupRefs []SecurityGroupRef `json:"securityGroupRefs,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// VPCEndpointStatus defines the observed state of VPCEndpoint.
type VPCEndpointStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// EndpointID is the AWS VPC Endpoint identifier.
	// +optional
	EndpointID string `json:"endpointId,omitempty"`

	// State is the current endpoint state.
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
// +kubebuilder:printcolumn:name="Endpoint-ID",type="string",JSONPath=".status.endpointId"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// VPCEndpoint is the Schema for managing AWS VPC Endpoints.
type VPCEndpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VPCEndpointSpec   `json:"spec,omitempty"`
	Status VPCEndpointStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VPCEndpointList contains a list of VPCEndpoint
type VPCEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VPCEndpoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VPCEndpoint{}, &VPCEndpointList{})
}
