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

// DBSubnetGroupSpec defines the desired state of an RDS DB Subnet Group.
type DBSubnetGroupSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// DBSubnetGroupName is the name of the DB subnet group. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="dbSubnetGroupName is immutable"
	DBSubnetGroupName string `json:"dbSubnetGroupName"`

	// Description is the description for the DB subnet group.
	// +kubebuilder:validation:MinLength=1
	Description string `json:"description"`

	// SubnetRefs is the list of subnet references to include in the group.
	// +kubebuilder:validation:MinItems=2
	SubnetRefs []SubnetRef `json:"subnetRefs"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// DBSubnetGroupStatus defines the observed state of DBSubnetGroup.
type DBSubnetGroupStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ARN is the Amazon Resource Name of the DB subnet group.
	// +optional
	ARN string `json:"arn,omitempty"`

	// Status is the current status of the DB subnet group.
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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.arn"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DBSubnetGroup is the Schema for managing RDS DB Subnet Groups.
type DBSubnetGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DBSubnetGroupSpec   `json:"spec,omitempty"`
	Status DBSubnetGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DBSubnetGroupList contains a list of DBSubnetGroup
type DBSubnetGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DBSubnetGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DBSubnetGroup{}, &DBSubnetGroupList{})
}
