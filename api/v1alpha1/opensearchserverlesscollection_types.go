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

// OpenSearchServerlessCollectionSpec defines the desired state of an OpenSearch Serverless collection.
type OpenSearchServerlessCollectionSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the name of the collection. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Type is the collection type (SEARCH, TIMESERIES, or VECTORSEARCH).
	// +kubebuilder:validation:Enum=SEARCH;TIMESERIES;VECTORSEARCH
	// +optional
	Type string `json:"type,omitempty"`

	// Description is the description of the collection.
	// +optional
	Description string `json:"description,omitempty"`

	// Tags are metadata tags for the collection.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// OpenSearchServerlessCollectionStatus defines the observed state of OpenSearchServerlessCollection.
type OpenSearchServerlessCollectionStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// CollectionID is the unique identifier of the collection.
	// +optional
	CollectionID string `json:"collectionID,omitempty"`

	// CollectionARN is the ARN of the collection.
	// +optional
	CollectionARN string `json:"collectionARN,omitempty"`

	// CollectionEndpoint is the endpoint URL for the collection.
	// +optional
	CollectionEndpoint string `json:"collectionEndpoint,omitempty"`

	// Status is the current status of the collection.
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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.collectionARN"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenSearchServerlessCollection is the Schema for managing OpenSearch Serverless collections.
type OpenSearchServerlessCollection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenSearchServerlessCollectionSpec   `json:"spec,omitempty"`
	Status OpenSearchServerlessCollectionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OpenSearchServerlessCollectionList contains a list of OpenSearchServerlessCollection.
type OpenSearchServerlessCollectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenSearchServerlessCollection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenSearchServerlessCollection{}, &OpenSearchServerlessCollectionList{})
}
