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

// PlacementGroupSpec defines the desired state of a Placement Group.
type PlacementGroupSpec struct {
	// GroupName is the name of the placement group. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="groupName is immutable"
	GroupName string `json:"groupName"`

	// Strategy is the placement strategy.
	// +kubebuilder:validation:Enum=cluster;partition;spread
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="strategy is immutable"
	Strategy string `json:"strategy"`

	// PartitionCount is the number of partitions (for partition strategy only).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=7
	// +optional
	PartitionCount int32 `json:"partitionCount,omitempty"`

	// SpreadLevel is the spread level (for spread strategy).
	// +kubebuilder:validation:Enum=host;rack
	// +optional
	SpreadLevel string `json:"spreadLevel,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// PlacementGroupStatus defines the observed state of PlacementGroup.
type PlacementGroupStatus struct {
	// GroupID is the AWS placement group ID.
	// +optional
	GroupID string `json:"groupId,omitempty"`

	// State is the current state of the placement group.
	// +optional
	State string `json:"state,omitempty"`

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
// +kubebuilder:printcolumn:name="Group-ID",type="string",JSONPath=".status.groupId"
// +kubebuilder:printcolumn:name="Strategy",type="string",JSONPath=".spec.strategy"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PlacementGroup is the Schema for managing EC2 Placement Groups.
type PlacementGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PlacementGroupSpec   `json:"spec,omitempty"`
	Status PlacementGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PlacementGroupList contains a list of PlacementGroup
type PlacementGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PlacementGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PlacementGroup{}, &PlacementGroupList{})
}
