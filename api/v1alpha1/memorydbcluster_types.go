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

// MemoryDBClusterSpec defines the desired state of a MemoryDB Cluster.
type MemoryDBClusterSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ClusterName is the name of the cluster.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="clusterName is immutable"
	ClusterName string `json:"clusterName"`

	// NodeType is the compute and memory capacity of the nodes.
	// +kubebuilder:validation:MinLength=1
	NodeType string `json:"nodeType"`

	// ACLName is the name of the access control list to associate.
	// +kubebuilder:validation:MinLength=1
	ACLName string `json:"aclName"`

	// NumShards is the number of shards in the cluster.
	// +optional
	NumShards *int32 `json:"numShards,omitempty"`

	// NumReplicasPerShard is the number of replicas per shard.
	// +optional
	NumReplicasPerShard *int32 `json:"numReplicasPerShard,omitempty"`

	// SubnetGroupName is the subnet group to use for the cluster.
	// +optional
	SubnetGroupName string `json:"subnetGroupName,omitempty"`

	// SecurityGroupIDs are the VPC security group IDs.
	// +optional
	SecurityGroupIDs []string `json:"securityGroupIds,omitempty"`

	// EngineVersion is the Redis OSS version.
	// +optional
	EngineVersion string `json:"engineVersion,omitempty"`

	// SnapshotRetentionLimit is the number of days to retain snapshots.
	// +optional
	SnapshotRetentionLimit *int32 `json:"snapshotRetentionLimit,omitempty"`

	// TLSEnabled enables in-transit encryption.
	// +optional
	TLSEnabled *bool `json:"tlsEnabled,omitempty"`

	// KMSKeyID is the ARN of the KMS key for encryption at rest.
	// +optional
	KMSKeyID string `json:"kmsKeyId,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// MemoryDBClusterStatus defines the observed state of MemoryDBCluster.
type MemoryDBClusterStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ARN is the ARN of the cluster.
	// +optional
	ARN string `json:"arn,omitempty"`

	// Status is the current state of the cluster.
	// +optional
	Status string `json:"status,omitempty"`

	// Endpoint is the cluster configuration endpoint.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

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

// MemoryDBCluster is the Schema for managing MemoryDB clusters.
type MemoryDBCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MemoryDBClusterSpec   `json:"spec,omitempty"`
	Status MemoryDBClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MemoryDBClusterList contains a list of MemoryDBCluster.
type MemoryDBClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MemoryDBCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MemoryDBCluster{}, &MemoryDBClusterList{})
}
