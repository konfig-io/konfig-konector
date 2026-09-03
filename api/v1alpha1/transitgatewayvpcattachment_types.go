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

// TransitGatewayRef references either a managed TransitGateway CR or a direct TGW ID.
type TransitGatewayRef struct {
	// Name of a TransitGateway CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// TransitGatewayID is a direct AWS Transit Gateway ID.
	// +optional
	TransitGatewayID string `json:"transitGatewayId,omitempty"`
}

// TransitGatewayVpcAttachmentSpec defines the desired state of a TGW VPC attachment.
type TransitGatewayVpcAttachmentSpec struct {
	// TransitGatewayRef references the Transit Gateway to attach to.
	TransitGatewayRef TransitGatewayRef `json:"transitGatewayRef"`

	// VPCRef references the VPC to attach.
	VPCRef VPCResourceRef `json:"vpcRef"`

	// SubnetRefs are the subnets in each availability zone to attach.
	// +kubebuilder:validation:MinItems=1
	SubnetRefs []SubnetRef `json:"subnetRefs"`

	// DNSSupport enables DNS resolution for the attachment.
	// +optional
	DNSSupport bool `json:"dnsSupport,omitempty"`

	// IPv6Support enables IPv6 for the attachment.
	// +optional
	IPv6Support bool `json:"ipv6Support,omitempty"`

	// ApplianceModeSupport enables appliance mode for the attachment.
	// +optional
	ApplianceModeSupport bool `json:"applianceModeSupport,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// TransitGatewayVpcAttachmentStatus defines the observed state of TransitGatewayVpcAttachment.
type TransitGatewayVpcAttachmentStatus struct {
	// AttachmentID is the AWS TGW VPC attachment ID.
	// +optional
	AttachmentID string `json:"attachmentId,omitempty"`

	// State is the current state of the attachment.
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
// +kubebuilder:printcolumn:name="Attachment-ID",type="string",JSONPath=".status.attachmentId"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// TransitGatewayVpcAttachment is the Schema for managing TGW VPC attachments.
type TransitGatewayVpcAttachment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TransitGatewayVpcAttachmentSpec   `json:"spec,omitempty"`
	Status TransitGatewayVpcAttachmentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TransitGatewayVpcAttachmentList contains a list of TransitGatewayVpcAttachment
type TransitGatewayVpcAttachmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TransitGatewayVpcAttachment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TransitGatewayVpcAttachment{}, &TransitGatewayVpcAttachmentList{})
}
