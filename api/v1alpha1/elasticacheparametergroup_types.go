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

// ElastiCacheParameter defines a parameter name/value pair.
type ElastiCacheParameter struct {
	// Name is the parameter name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Value is the parameter value.
	Value string `json:"value"`
}

// ElastiCacheParameterGroupSpec defines the desired state of an ElastiCache Parameter Group.
type ElastiCacheParameterGroupSpec struct {
	// CacheParameterGroupName is the name of the parameter group.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="cacheParameterGroupName is immutable"
	CacheParameterGroupName string `json:"cacheParameterGroupName"`

	// CacheParameterGroupFamily is the engine family (e.g., redis7, memcached1.6).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="cacheParameterGroupFamily is immutable"
	CacheParameterGroupFamily string `json:"cacheParameterGroupFamily"`

	// Description is a description for the parameter group.
	// +optional
	Description string `json:"description,omitempty"`

	// Parameters are the parameter name/value pairs to set.
	// +optional
	Parameters []ElastiCacheParameter `json:"parameters,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ElastiCacheParameterGroupStatus defines the observed state of ElastiCacheParameterGroup.
type ElastiCacheParameterGroupStatus struct {
	// ARN is the ARN of the parameter group.
	// +optional
	ARN string `json:"arn,omitempty"`

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
// +kubebuilder:printcolumn:name="Family",type="string",JSONPath=".spec.cacheParameterGroupFamily"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ElastiCacheParameterGroup is the Schema for managing ElastiCache parameter groups.
type ElastiCacheParameterGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ElastiCacheParameterGroupSpec   `json:"spec,omitempty"`
	Status ElastiCacheParameterGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ElastiCacheParameterGroupList contains a list of ElastiCacheParameterGroup.
type ElastiCacheParameterGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ElastiCacheParameterGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ElastiCacheParameterGroup{}, &ElastiCacheParameterGroupList{})
}
