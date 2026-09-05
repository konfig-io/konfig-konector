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

// DynamoDBReplicaSpec defines a replica region for a global table.
type DynamoDBReplicaSpec struct {
	// RegionName is the AWS region for this replica.
	RegionName string `json:"regionName"`
}

// DynamoDBGlobalTableSpec defines the desired state of a DynamoDB Global Table.
type DynamoDBGlobalTableSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// TableName is the name of the global table. Immutable.
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="tableName is immutable"
	TableName string `json:"tableName"`

	// ReplicationGroup defines the regions for the global table.
	// +kubebuilder:validation:MinItems=1
	ReplicationGroup []DynamoDBReplicaSpec `json:"replicationGroup"`
}

// DynamoDBGlobalTableStatus defines the observed state of DynamoDBGlobalTable.
type DynamoDBGlobalTableStatus struct {
	// ARN is the ARN of the global table.
	// +optional
	ARN string `json:"arn,omitempty"`

	// GlobalTableStatus is the current status.
	// +optional
	GlobalTableStatus string `json:"globalTableStatus,omitempty"`

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
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.globalTableStatus"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DynamoDBGlobalTable is the Schema for managing DynamoDB Global Tables.
type DynamoDBGlobalTable struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DynamoDBGlobalTableSpec   `json:"spec,omitempty"`
	Status DynamoDBGlobalTableStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DynamoDBGlobalTableList contains a list of DynamoDBGlobalTable
type DynamoDBGlobalTableList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DynamoDBGlobalTable `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DynamoDBGlobalTable{}, &DynamoDBGlobalTableList{})
}
