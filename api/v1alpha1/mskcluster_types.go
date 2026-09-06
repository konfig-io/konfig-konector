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

// MSKBrokerNodeGroupInfo defines broker node group configuration.
type MSKBrokerNodeGroupInfo struct {
	// InstanceType is the Amazon MSK broker instance type (e.g. kafka.m5.large).
	InstanceType string `json:"instanceType"`

	// ClientSubnets is the list of subnets for the broker nodes.
	ClientSubnets []string `json:"clientSubnets"`

	// SecurityGroups is the list of security group IDs for broker nodes.
	// +optional
	SecurityGroups []string `json:"securityGroups,omitempty"`

	// StorageVolumeSizeGiB is the EBS volume size in GiB per broker.
	// +optional
	StorageVolumeSizeGiB *int32 `json:"storageVolumeSizeGiB,omitempty"`
}

// MSKClusterSpec defines the desired state of an MSK provisioned cluster.
type MSKClusterSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ClusterName is the name of the cluster. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="clusterName is immutable"
	ClusterName string `json:"clusterName"`

	// KafkaVersion is the Kafka version to deploy (e.g. "3.5.1").
	KafkaVersion string `json:"kafkaVersion"`

	// NumberOfBrokerNodes is the total number of broker nodes across all AZs.
	NumberOfBrokerNodes int32 `json:"numberOfBrokerNodes"`

	// BrokerNodeGroupInfo defines the broker node configuration.
	BrokerNodeGroupInfo MSKBrokerNodeGroupInfo `json:"brokerNodeGroupInfo"`

	// Tags are metadata tags for the cluster.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// MSKClusterStatus defines the observed state of MSKCluster.
type MSKClusterStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ClusterARN is the ARN of the MSK cluster.
	// +optional
	ClusterARN string `json:"clusterARN,omitempty"`

	// ClusterState is the current state of the cluster (CREATING, ACTIVE, DELETING, etc.).
	// +optional
	ClusterState string `json:"clusterState,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.clusterARN"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.clusterState"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// MSKCluster is the Schema for managing MSK provisioned clusters.
type MSKCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MSKClusterSpec   `json:"spec,omitempty"`
	Status MSKClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MSKClusterList contains a list of MSKCluster.
type MSKClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MSKCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MSKCluster{}, &MSKClusterList{})
}
