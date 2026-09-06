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

// ElastiCacheReplicationGroupSpec defines the desired state of an ElastiCache Replication Group.
type ElastiCacheReplicationGroupSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ReplicationGroupID is the identifier for the replication group. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=40
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="replicationGroupId is immutable"
	ReplicationGroupID string `json:"replicationGroupId"`

	// Description is a human-readable description of the replication group.
	// +kubebuilder:validation:MinLength=1
	Description string `json:"description"`

	// Engine is the cache engine: redis or valkey.
	// +kubebuilder:validation:Enum=redis;valkey
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="engine is immutable"
	Engine string `json:"engine"`

	// EngineVersion is the cache engine version (e.g. "7.0").
	// +optional
	EngineVersion string `json:"engineVersion,omitempty"`

	// CacheNodeType is the instance type (e.g. cache.t3.micro).
	// +kubebuilder:validation:MinLength=1
	CacheNodeType string `json:"cacheNodeType"`

	// NumCacheClusters is the number of cache clusters (nodes) in the replication group.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=6
	// +optional
	NumCacheClusters int32 `json:"numCacheClusters,omitempty"`

	// AutomaticFailover enables automatic failover to a read replica on primary failure.
	// +optional
	AutomaticFailover bool `json:"automaticFailover,omitempty"`

	// SubnetGroupRef is the name of an ElastiCacheSubnetGroup CR in the same namespace.
	// +optional
	SubnetGroupRef string `json:"subnetGroupRef,omitempty"`

	// SecurityGroupRefs is the list of security group references.
	// +optional
	SecurityGroupRefs []SecurityGroupRef `json:"securityGroupRefs,omitempty"`

	// AtRestEncryption enables encryption at rest.
	// +optional
	AtRestEncryption bool `json:"atRestEncryption,omitempty"`

	// TransitEncryption enables in-transit encryption (TLS).
	// +optional
	TransitEncryption bool `json:"transitEncryption,omitempty"`

	// AuthTokenRef references a Kubernetes Secret containing the auth token
	// for transit-encrypted clusters.
	// +optional
	AuthTokenRef *SecretRef `json:"authTokenRef,omitempty"`

	// AuthToken is the literal auth token value.
	// Deprecated: use authTokenRef instead — literal tokens are readable by
	// anyone with get access on the CR and are stored unencrypted in etcd.
	// +optional
	AuthToken string `json:"authToken,omitempty"`

	// SnapshotRetentionLimit is the number of days to retain automatic snapshots.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=35
	// +optional
	SnapshotRetentionLimit int32 `json:"snapshotRetentionLimit,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ElastiCacheReplicationGroupStatus defines the observed state of ElastiCacheReplicationGroup.
type ElastiCacheReplicationGroupStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ARN is the Amazon Resource Name of the replication group.
	// +optional
	ARN string `json:"arn,omitempty"`

	// PrimaryEndpoint is the primary node endpoint.
	// +optional
	PrimaryEndpoint string `json:"primaryEndpoint,omitempty"`

	// ReaderEndpoint is the reader endpoint for read replicas.
	// +optional
	ReaderEndpoint string `json:"readerEndpoint,omitempty"`

	// Status is the current replication group status.
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
// +kubebuilder:printcolumn:name="Primary-Endpoint",type="string",JSONPath=".status.primaryEndpoint"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ElastiCacheReplicationGroup is the Schema for managing ElastiCache Replication Groups.
type ElastiCacheReplicationGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ElastiCacheReplicationGroupSpec   `json:"spec,omitempty"`
	Status ElastiCacheReplicationGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ElastiCacheReplicationGroupList contains a list of ElastiCacheReplicationGroup
type ElastiCacheReplicationGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ElastiCacheReplicationGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ElastiCacheReplicationGroup{}, &ElastiCacheReplicationGroupList{})
}
