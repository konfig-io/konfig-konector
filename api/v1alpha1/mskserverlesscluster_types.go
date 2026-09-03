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

// MSKServerlessVpcConfig defines a VPC config entry for serverless MSK.
type MSKServerlessVpcConfig struct {
	// SubnetIDs is the list of subnet IDs.
	SubnetIDs []string `json:"subnetIDs"`

	// SecurityGroupIDs is the list of security group IDs.
	// +optional
	SecurityGroupIDs []string `json:"securityGroupIDs,omitempty"`
}

// MSKServerlessClusterSpec defines the desired state of an MSK serverless cluster.
type MSKServerlessClusterSpec struct {
	// ClusterName is the name of the serverless cluster. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="clusterName is immutable"
	ClusterName string `json:"clusterName"`

	// VpcConfigs is the list of VPC configurations.
	VpcConfigs []MSKServerlessVpcConfig `json:"vpcConfigs"`

	// Tags are metadata tags for the cluster.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// MSKServerlessClusterStatus defines the observed state of MSKServerlessCluster.
type MSKServerlessClusterStatus struct {
	// ClusterARN is the ARN of the serverless MSK cluster.
	// +optional
	ClusterARN string `json:"clusterARN,omitempty"`

	// ClusterState is the current state of the cluster.
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

// MSKServerlessCluster is the Schema for managing MSK serverless clusters.
type MSKServerlessCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MSKServerlessClusterSpec   `json:"spec,omitempty"`
	Status MSKServerlessClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MSKServerlessClusterList contains a list of MSKServerlessCluster.
type MSKServerlessClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MSKServerlessCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MSKServerlessCluster{}, &MSKServerlessClusterList{})
}
