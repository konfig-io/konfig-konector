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

// LatticeServiceRef references a managed LatticeService CR or a direct
// service ID/ARN.
type LatticeServiceRef struct {
	// Name of a LatticeService CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ID is a direct AWS service ID or ARN. If set, Name is ignored.
	// +optional
	ID string `json:"id,omitempty"`
}

// LatticeServiceNetworkServiceAssociationSpec defines the desired state of a
// VPC Lattice service network to service association.
type LatticeServiceNetworkServiceAssociationSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ServiceNetworkRef references the service network to associate.
	ServiceNetworkRef LatticeServiceNetworkRef `json:"serviceNetworkRef"`

	// ServiceRef references the service to associate.
	ServiceRef LatticeServiceRef `json:"serviceRef"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// LatticeServiceNetworkServiceAssociationStatus defines the observed state of
// LatticeServiceNetworkServiceAssociation.
type LatticeServiceNetworkServiceAssociationStatus struct {
	// ARN is the Amazon Resource Name of the association.
	// +optional
	ARN string `json:"arn,omitempty"`

	// ID is the unique identifier of the association.
	// +optional
	ID string `json:"id,omitempty"`

	// Status is the lifecycle status reported by AWS.
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
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.id"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// LatticeServiceNetworkServiceAssociation is the Schema for managing VPC
// Lattice service network to service associations.
type LatticeServiceNetworkServiceAssociation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LatticeServiceNetworkServiceAssociationSpec   `json:"spec,omitempty"`
	Status LatticeServiceNetworkServiceAssociationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// LatticeServiceNetworkServiceAssociationList contains a list of LatticeServiceNetworkServiceAssociation
type LatticeServiceNetworkServiceAssociationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LatticeServiceNetworkServiceAssociation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LatticeServiceNetworkServiceAssociation{}, &LatticeServiceNetworkServiceAssociationList{})
}
