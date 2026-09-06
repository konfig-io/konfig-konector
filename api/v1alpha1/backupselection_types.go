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

// BackupSelectionTag is a simple StringEquals tag condition used to assign
// resources to a backup plan.
type BackupSelectionTag struct {
	// Key is the tag key.
	Key string `json:"key"`
	// Value is the tag value.
	Value string `json:"value"`
}

// BackupSelectionSpec defines the desired state of an AWS Backup selection.
type BackupSelectionSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// PlanRef is the name of a BackupPlan CR in the same namespace.
	// +kubebuilder:validation:MinLength=1
	PlanRef string `json:"planRef"`

	// SelectionName is the display name of the resource selection.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=50
	SelectionName string `json:"selectionName"`

	// IAMRoleARN is the IAM role ARN Backup uses to authenticate when backing
	// up resources. Either this or IAMRoleRef must be set.
	// +optional
	IAMRoleARN string `json:"iamRoleArn,omitempty"`

	// IAMRoleRef references an IAMRole CR whose ARN to use.
	// +optional
	IAMRoleRef *RoleRef `json:"iamRoleRef,omitempty"`

	// Resources are ARNs of resources to assign to the backup plan.
	// +optional
	Resources []string `json:"resources,omitempty"`

	// ListOfTags assigns resources matching any of these StringEquals tag
	// conditions (OR logic).
	// +optional
	ListOfTags []BackupSelectionTag `json:"listOfTags,omitempty"`
}

// BackupSelectionStatus defines the observed state of BackupSelection.
type BackupSelectionStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// SelectionID uniquely identifies the backup selection.
	// +optional
	SelectionID string `json:"selectionId,omitempty"`

	// PlanID is the backup plan the selection is attached to.
	// +optional
	PlanID string `json:"planId,omitempty"`

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
// +kubebuilder:printcolumn:name="Selection-ID",type="string",JSONPath=".status.selectionId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// BackupSelection is the Schema for managing AWS Backup selections.
type BackupSelection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupSelectionSpec   `json:"spec,omitempty"`
	Status BackupSelectionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BackupSelectionList contains a list of BackupSelection
type BackupSelectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupSelection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackupSelection{}, &BackupSelectionList{})
}
