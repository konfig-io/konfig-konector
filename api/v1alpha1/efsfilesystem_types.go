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

// EFSFileSystemSpec defines the desired state of an EFS File System.
type EFSFileSystemSpec struct {
	// PerformanceMode sets the performance mode of the file system.
	// +kubebuilder:validation:Enum=generalPurpose;maxIO
	// +optional
	PerformanceMode string `json:"performanceMode,omitempty"`

	// ThroughputMode sets the throughput mode of the file system.
	// +kubebuilder:validation:Enum=bursting;provisioned;elastic
	// +optional
	ThroughputMode string `json:"throughputMode,omitempty"`

	// ProvisionedThroughputInMibps sets provisioned throughput (only for provisioned mode).
	// +optional
	ProvisionedThroughputInMibps *float64 `json:"provisionedThroughputInMibps,omitempty"`

	// Encrypted specifies whether the file system is encrypted at rest.
	// +optional
	Encrypted bool `json:"encrypted,omitempty"`

	// KMSKeyID is the ARN or ID of the KMS key for encryption.
	// +optional
	KMSKeyID string `json:"kmsKeyId,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// EFSFileSystemStatus defines the observed state of EFSFileSystem.
type EFSFileSystemStatus struct {
	// FileSystemID is the ID of the EFS file system.
	// +optional
	FileSystemID string `json:"fileSystemId,omitempty"`

	// FileSystemARN is the ARN of the EFS file system.
	// +optional
	FileSystemARN string `json:"fileSystemArn,omitempty"`

	// LifeCycleState is the current state of the file system.
	// +optional
	LifeCycleState string `json:"lifeCycleState,omitempty"`

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
// +kubebuilder:printcolumn:name="FileSystemID",type="string",JSONPath=".status.fileSystemId"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.lifeCycleState"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EFSFileSystem is the Schema for managing EFS file systems.
type EFSFileSystem struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EFSFileSystemSpec   `json:"spec,omitempty"`
	Status EFSFileSystemStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EFSFileSystemList contains a list of EFSFileSystem.
type EFSFileSystemList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EFSFileSystem `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EFSFileSystem{}, &EFSFileSystemList{})
}
