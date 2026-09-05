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

// RDSGlobalClusterSpec defines the desired state of an Aurora global cluster.
type RDSGlobalClusterSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// GlobalClusterIdentifier is the identifier of the global cluster.
	// Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="globalClusterIdentifier is immutable"
	GlobalClusterIdentifier string `json:"globalClusterIdentifier"`

	// Engine is the database engine, e.g. aurora-mysql or aurora-postgresql.
	// +optional
	Engine string `json:"engine,omitempty"`

	// EngineVersion is the engine version.
	// +optional
	EngineVersion string `json:"engineVersion,omitempty"`

	// DeletionProtection prevents the global cluster from being deleted.
	// +optional
	DeletionProtection bool `json:"deletionProtection,omitempty"`

	// StorageEncrypted enables encrypted storage.
	// +optional
	StorageEncrypted bool `json:"storageEncrypted,omitempty"`

	// SourceDBClusterIdentifier is the ARN of an existing DB cluster to use
	// as the primary. When set, engine settings come from the source cluster.
	// +optional
	SourceDBClusterIdentifier string `json:"sourceDBClusterIdentifier,omitempty"`
}

// RDSGlobalClusterStatus defines the observed state of RDSGlobalCluster.
type RDSGlobalClusterStatus struct {
	// ARN is the ARN of the global cluster.
	// +optional
	ARN string `json:"arn,omitempty"`

	// Status is the current status of the global cluster.
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
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// RDSGlobalCluster is the Schema for managing Aurora global clusters.
type RDSGlobalCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RDSGlobalClusterSpec   `json:"spec,omitempty"`
	Status RDSGlobalClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RDSGlobalClusterList contains a list of RDSGlobalCluster
type RDSGlobalClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RDSGlobalCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RDSGlobalCluster{}, &RDSGlobalClusterList{})
}
