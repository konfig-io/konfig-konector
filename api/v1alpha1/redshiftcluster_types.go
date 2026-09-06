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

// RedshiftClusterSpec defines the desired state of a Redshift cluster.
type RedshiftClusterSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ClusterIdentifier is the unique identifier of the cluster. Immutable
	// after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="clusterIdentifier is immutable"
	ClusterIdentifier string `json:"clusterIdentifier"`

	// NodeType is the node type to provision (e.g. ra3.xlplus, dc2.large).
	// +kubebuilder:validation:MinLength=1
	NodeType string `json:"nodeType"`

	// NumberOfNodes is the number of compute nodes. 1 creates a single-node
	// cluster; >1 creates a multi-node cluster.
	// +kubebuilder:validation:Minimum=1
	// +optional
	NumberOfNodes int32 `json:"numberOfNodes,omitempty"`

	// MasterUsername is the master user name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="masterUsername is immutable"
	MasterUsername string `json:"masterUsername"`

	// MasterUserPasswordRef references a Kubernetes Secret containing the
	// master user password. The value is never written to status or logs.
	MasterUserPasswordRef SecretRef `json:"masterUserPasswordRef"`

	// DBName is the name of the first database created in the cluster.
	// +optional
	DBName string `json:"dbName,omitempty"`

	// ClusterSubnetGroupName is the name of an existing Redshift subnet
	// group. Takes precedence over ClusterSubnetGroupRef.
	// +optional
	ClusterSubnetGroupName string `json:"clusterSubnetGroupName,omitempty"`

	// ClusterSubnetGroupRef is the name of a RedshiftSubnetGroup CR in the
	// same namespace.
	// +optional
	ClusterSubnetGroupRef string `json:"clusterSubnetGroupRef,omitempty"`

	// VPCSecurityGroupRefs is the list of VPC security group references.
	// +optional
	VPCSecurityGroupRefs []SecurityGroupRef `json:"vpcSecurityGroupRefs,omitempty"`

	// Encrypted enables encryption at rest.
	// +optional
	Encrypted bool `json:"encrypted,omitempty"`

	// KMSKeyID is the KMS key ID/ARN used for encryption at rest.
	// +optional
	KMSKeyID string `json:"kmsKeyId,omitempty"`

	// PubliclyAccessible controls whether the cluster is reachable from a
	// public network.
	// +optional
	PubliclyAccessible bool `json:"publiclyAccessible,omitempty"`

	// SkipFinalClusterSnapshot skips the final snapshot when the cluster is
	// deleted.
	// +optional
	SkipFinalClusterSnapshot bool `json:"skipFinalClusterSnapshot,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// RedshiftClusterStatus defines the observed state of RedshiftCluster.
type RedshiftClusterStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ClusterIdentifier is the identifier of the cluster in AWS.
	// +optional
	ClusterIdentifier string `json:"clusterIdentifier,omitempty"`

	// ClusterStatus is the current cluster status (available, creating,
	// modifying, deleting, ...).
	// +optional
	ClusterStatus string `json:"clusterStatus,omitempty"`

	// Endpoint is the connection endpoint hostname.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// Port is the connection port.
	// +optional
	Port int32 `json:"port,omitempty"`

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
// +kubebuilder:printcolumn:name="Endpoint",type="string",JSONPath=".status.endpoint"
// +kubebuilder:printcolumn:name="Cluster-Status",type="string",JSONPath=".status.clusterStatus"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// RedshiftCluster is the Schema for managing Redshift clusters.
type RedshiftCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedshiftClusterSpec   `json:"spec,omitempty"`
	Status RedshiftClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RedshiftClusterList contains a list of RedshiftCluster
type RedshiftClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedshiftCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RedshiftCluster{}, &RedshiftClusterList{})
}
