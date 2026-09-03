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

// DynamoDBBackupSpec defines the desired state of a DynamoDB on-demand backup.
type DynamoDBBackupSpec struct {
	// TableName is the name of the DynamoDB table to back up.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="tableName is immutable"
	TableName string `json:"tableName"`

	// BackupName is the name of the backup.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="backupName is immutable"
	BackupName string `json:"backupName"`
}

// DynamoDBBackupStatus defines the observed state of DynamoDBBackup.
type DynamoDBBackupStatus struct {
	// BackupARN is the ARN of the backup.
	// +optional
	BackupARN string `json:"backupArn,omitempty"`

	// BackupStatus is the current status of the backup (CREATING, DELETED, AVAILABLE).
	// +optional
	BackupStatus string `json:"backupStatus,omitempty"`

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
// +kubebuilder:printcolumn:name="BackupARN",type="string",JSONPath=".status.backupArn"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.backupStatus"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DynamoDBBackup is the Schema for managing DynamoDB on-demand backups.
type DynamoDBBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DynamoDBBackupSpec   `json:"spec,omitempty"`
	Status DynamoDBBackupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DynamoDBBackupList contains a list of DynamoDBBackup.
type DynamoDBBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DynamoDBBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DynamoDBBackup{}, &DynamoDBBackupList{})
}
