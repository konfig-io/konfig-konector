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

// RedshiftSubnetGroupSpec defines the desired state of a Redshift cluster
// subnet group.
type RedshiftSubnetGroupSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the name of the subnet group. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Description is a description of the subnet group.
	Description string `json:"description"`

	// SubnetRefs is the list of subnets in the group.
	// +kubebuilder:validation:MinItems=1
	SubnetRefs []SubnetRef `json:"subnetRefs"`

	// Tags are AWS resource tags to apply at creation.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// RedshiftSubnetGroupStatus defines the observed state of RedshiftSubnetGroup.
type RedshiftSubnetGroupStatus struct {
	// SubnetGroupName is the name of the subnet group in AWS.
	// +optional
	SubnetGroupName string `json:"subnetGroupName,omitempty"`

	// VPCID is the VPC the subnet group belongs to.
	// +optional
	VPCID string `json:"vpcId,omitempty"`

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
// +kubebuilder:printcolumn:name="Subnet-Group",type="string",JSONPath=".status.subnetGroupName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// RedshiftSubnetGroup is the Schema for managing Redshift cluster subnet groups.
type RedshiftSubnetGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedshiftSubnetGroupSpec   `json:"spec,omitempty"`
	Status RedshiftSubnetGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RedshiftSubnetGroupList contains a list of RedshiftSubnetGroup
type RedshiftSubnetGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedshiftSubnetGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RedshiftSubnetGroup{}, &RedshiftSubnetGroupList{})
}
