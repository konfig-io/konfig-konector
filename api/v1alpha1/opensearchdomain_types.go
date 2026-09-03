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

// OpenSearchClusterConfig defines the cluster configuration for an OpenSearch domain.
type OpenSearchClusterConfig struct {
	// InstanceType is the instance type for data nodes (e.g. r6g.large.search).
	// +optional
	InstanceType string `json:"instanceType,omitempty"`

	// InstanceCount is the number of data nodes.
	// +optional
	InstanceCount *int32 `json:"instanceCount,omitempty"`

	// DedicatedMasterEnabled enables dedicated master nodes.
	// +optional
	DedicatedMasterEnabled *bool `json:"dedicatedMasterEnabled,omitempty"`
}

// OpenSearchDomainSpec defines the desired state of an OpenSearch Service domain.
type OpenSearchDomainSpec struct {
	// DomainName is the name of the domain. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="domainName is immutable"
	DomainName string `json:"domainName"`

	// EngineVersion specifies the OpenSearch or Elasticsearch version (e.g. "OpenSearch_2.11").
	// +optional
	EngineVersion string `json:"engineVersion,omitempty"`

	// ClusterConfig defines the cluster configuration.
	// +optional
	ClusterConfig *OpenSearchClusterConfig `json:"clusterConfig,omitempty"`

	// Tags are metadata tags for the domain.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// OpenSearchDomainStatus defines the observed state of OpenSearchDomain.
type OpenSearchDomainStatus struct {
	// DomainARN is the ARN of the OpenSearch domain.
	// +optional
	DomainARN string `json:"domainARN,omitempty"`

	// DomainID is the unique identifier of the domain.
	// +optional
	DomainID string `json:"domainID,omitempty"`

	// Endpoint is the domain endpoint URL.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// Processing is true when the domain is being created or updated.
	// +optional
	Processing bool `json:"processing,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.domainARN"
// +kubebuilder:printcolumn:name="Endpoint",type="string",JSONPath=".status.endpoint"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenSearchDomain is the Schema for managing AWS OpenSearch Service domains.
type OpenSearchDomain struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenSearchDomainSpec   `json:"spec,omitempty"`
	Status OpenSearchDomainStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OpenSearchDomainList contains a list of OpenSearchDomain.
type OpenSearchDomainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenSearchDomain `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenSearchDomain{}, &OpenSearchDomainList{})
}
