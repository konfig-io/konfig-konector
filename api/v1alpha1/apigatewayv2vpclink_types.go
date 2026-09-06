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

// APIGatewayV2VpcLinkSpec defines the desired state of an API Gateway v2 VPC link.
type APIGatewayV2VpcLinkSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name of the VPC link.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// SubnetRefs are the subnets for the VPC link.
	// +kubebuilder:validation:MinItems=1
	SubnetRefs []SubnetRef `json:"subnetRefs"`

	// SecurityGroupRefs are the security groups for the VPC link.
	// +optional
	SecurityGroupRefs []SecurityGroupRef `json:"securityGroupRefs,omitempty"`

	// Tags are AWS resource tags to apply on creation.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// APIGatewayV2VpcLinkStatus defines the observed state of APIGatewayV2VpcLink.
type APIGatewayV2VpcLinkStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// VPCLinkID is the VPC link identifier.
	// +optional
	VPCLinkID string `json:"vpcLinkId,omitempty"`

	// VPCLinkStatus is the AWS status of the VPC link (PENDING, AVAILABLE, ...).
	// +optional
	VPCLinkStatus string `json:"vpcLinkStatus,omitempty"`

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
// +kubebuilder:printcolumn:name="VpcLink-ID",type="string",JSONPath=".status.vpcLinkId"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.vpcLinkStatus"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// APIGatewayV2VpcLink is the Schema for managing API Gateway v2 VPC links.
type APIGatewayV2VpcLink struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   APIGatewayV2VpcLinkSpec   `json:"spec,omitempty"`
	Status APIGatewayV2VpcLinkStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// APIGatewayV2VpcLinkList contains a list of APIGatewayV2VpcLink
type APIGatewayV2VpcLinkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []APIGatewayV2VpcLink `json:"items"`
}

func init() {
	SchemeBuilder.Register(&APIGatewayV2VpcLink{}, &APIGatewayV2VpcLinkList{})
}
