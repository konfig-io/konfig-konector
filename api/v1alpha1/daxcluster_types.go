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

// DAXClusterSpec defines the desired state of a DAX Cluster.
type DAXClusterSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ClusterName is the cluster identifier.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="clusterName is immutable"
	ClusterName string `json:"clusterName"`

	// NodeType is the compute and memory capacity node type.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="nodeType is immutable"
	NodeType string `json:"nodeType"`

	// ReplicationFactor is the number of nodes in the cluster.
	// +kubebuilder:validation:Minimum=1
	ReplicationFactor int32 `json:"replicationFactor"`

	// IAMRoleARN is the ARN of the IAM role for DAX to access DynamoDB.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="iamRoleArn is immutable"
	IAMRoleARN string `json:"iamRoleArn"`

	// SubnetGroupName is the subnet group for the cluster.
	// +optional
	SubnetGroupName string `json:"subnetGroupName,omitempty"`

	// SecurityGroupIDs are the VPC security group IDs.
	// +optional
	SecurityGroupIDs []string `json:"securityGroupIds,omitempty"`

	// ParameterGroupName is the parameter group to associate.
	// +optional
	ParameterGroupName string `json:"parameterGroupName,omitempty"`

	// AvailabilityZones are the AZs for the nodes.
	// +optional
	AvailabilityZones []string `json:"availabilityZones,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// DAXClusterStatus defines the observed state of DAXCluster.
type DAXClusterStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ClusterARN is the ARN of the cluster.
	// +optional
	ClusterARN string `json:"clusterArn,omitempty"`

	// Status is the current state of the cluster.
	// +optional
	Status string `json:"status,omitempty"`

	// ClusterDiscoveryEndpoint is the endpoint for cluster discovery.
	// +optional
	ClusterDiscoveryEndpoint string `json:"clusterDiscoveryEndpoint,omitempty"`

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

// DAXCluster is the Schema for managing DAX clusters.
type DAXCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DAXClusterSpec   `json:"spec,omitempty"`
	Status DAXClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DAXClusterList contains a list of DAXCluster.
type DAXClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DAXCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DAXCluster{}, &DAXClusterList{})
}
