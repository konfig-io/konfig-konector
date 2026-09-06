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

// ElasticIPRef references either a managed ElasticIP CR or a direct allocation ID.
type ElasticIPRef struct {
	// Name of an ElasticIP CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// AllocationID is a direct AWS EIP allocation ID (e.g. eipalloc-0abc1234).
	// +optional
	AllocationID string `json:"allocationId,omitempty"`
}

// EIPAssociationSpec defines the desired state of an EIP association.
type EIPAssociationSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ElasticIPRef references the ElasticIP to associate.
	ElasticIPRef ElasticIPRef `json:"elasticIpRef"`

	// InstanceRef references the EC2 instance to associate with.
	// Either InstanceRef or NetworkInterfaceID must be set.
	// +optional
	InstanceRef *ResourceRef `json:"instanceRef,omitempty"`

	// InstanceID is a direct EC2 instance ID. Used when InstanceRef is not set.
	// +optional
	InstanceID string `json:"instanceId,omitempty"`

	// NetworkInterfaceID is the network interface ID to associate the EIP with.
	// +optional
	NetworkInterfaceID string `json:"networkInterfaceId,omitempty"`

	// PrivateIPAddress is the private IP address to associate the EIP with.
	// +optional
	PrivateIPAddress string `json:"privateIpAddress,omitempty"`

	// AllowReassociation allows the EIP to be reassociated if already associated.
	// +optional
	AllowReassociation bool `json:"allowReassociation,omitempty"`
}

// EIPAssociationStatus defines the observed state of EIPAssociation.
type EIPAssociationStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// AssociationID is the AWS association ID.
	// +optional
	AssociationID string `json:"associationId,omitempty"`

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
// +kubebuilder:printcolumn:name="Association-ID",type="string",JSONPath=".status.associationId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EIPAssociation is the Schema for managing EC2 Elastic IP associations.
type EIPAssociation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EIPAssociationSpec   `json:"spec,omitempty"`
	Status EIPAssociationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EIPAssociationList contains a list of EIPAssociation
type EIPAssociationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EIPAssociation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EIPAssociation{}, &EIPAssociationList{})
}
