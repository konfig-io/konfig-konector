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

// ECSClusterSpec defines the desired state of an ECS Cluster.
type ECSClusterSpec struct {
	// ClusterName is the name of the ECS cluster. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="clusterName is immutable"
	ClusterName string `json:"clusterName"`

	// CapacityProviders is the list of capacity providers to associate with the cluster.
	// Defaults to ["FARGATE", "FARGATE_SPOT"].
	// +optional
	CapacityProviders []string `json:"capacityProviders,omitempty"`

	// ContainerInsights enables CloudWatch Container Insights for the cluster.
	// +optional
	ContainerInsights *bool `json:"containerInsights,omitempty"`

	// Tags are AWS resource tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ECSClusterStatus defines the observed state of ECSCluster.
type ECSClusterStatus struct {
	// ClusterARN is the ARN of the ECS cluster.
	// +optional
	ClusterARN string `json:"clusterArn,omitempty"`

	// Status is the cluster status: ACTIVE, PROVISIONING, DEPROVISIONING, FAILED, INACTIVE.
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
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.clusterName"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ECSCluster is the Schema for managing ECS Clusters.
type ECSCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ECSClusterSpec   `json:"spec,omitempty"`
	Status ECSClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ECSClusterList contains a list of ECSCluster.
type ECSClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ECSCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ECSCluster{}, &ECSClusterList{})
}
