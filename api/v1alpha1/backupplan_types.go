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

// BackupLifecycle defines when a recovery point transitions to cold storage
// and when it expires.
type BackupLifecycle struct {
	// DeleteAfterDays is the number of days after creation that a recovery
	// point is deleted. Must be at least 90 days after
	// MoveToColdStorageAfterDays.
	// +optional
	DeleteAfterDays *int64 `json:"deleteAfterDays,omitempty"`

	// MoveToColdStorageAfterDays is the number of days after creation that a
	// recovery point is moved to cold storage.
	// +optional
	MoveToColdStorageAfterDays *int64 `json:"moveToColdStorageAfterDays,omitempty"`
}

// BackupPlanRule specifies a scheduled backup task.
type BackupPlanRule struct {
	// RuleName is a display name for the rule.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=50
	RuleName string `json:"ruleName"`

	// TargetBackupVaultName is the AWS name of the vault where backups are
	// stored. Either this or TargetBackupVaultRef must be set.
	// +optional
	TargetBackupVaultName string `json:"targetBackupVaultName,omitempty"`

	// TargetBackupVaultRef references a BackupVault CR in the same namespace.
	// +optional
	TargetBackupVaultRef string `json:"targetBackupVaultRef,omitempty"`

	// ScheduleExpression is a CRON expression in UTC specifying when Backup
	// initiates a backup job.
	// +optional
	ScheduleExpression string `json:"scheduleExpression,omitempty"`

	// StartWindowMinutes is the window after a backup is scheduled before a
	// job is canceled if it doesn't start. Minimum 60.
	// +optional
	StartWindowMinutes *int64 `json:"startWindowMinutes,omitempty"`

	// CompletionWindowMinutes is the window after a backup job starts before
	// it must complete or be canceled.
	// +optional
	CompletionWindowMinutes *int64 `json:"completionWindowMinutes,omitempty"`

	// Lifecycle defines cold storage transition and expiry for recovery points.
	// +optional
	Lifecycle *BackupLifecycle `json:"lifecycle,omitempty"`
}

// BackupPlanSpec defines the desired state of an AWS Backup plan.
type BackupPlanSpec struct {
	// PlanName is the display name of the backup plan.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=50
	PlanName string `json:"planName"`

	// Rules are the scheduled backup tasks in this plan.
	// +kubebuilder:validation:MinItems=1
	Rules []BackupPlanRule `json:"rules"`

	// Tags are AWS resource tags to apply to the plan.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// BackupPlanStatus defines the observed state of BackupPlan.
type BackupPlanStatus struct {
	// PlanID uniquely identifies the backup plan.
	// +optional
	PlanID string `json:"planId,omitempty"`

	// PlanARN is the ARN of the backup plan.
	// +optional
	PlanARN string `json:"planArn,omitempty"`

	// VersionID is the current version of the backup plan.
	// +optional
	VersionID string `json:"versionId,omitempty"`

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
// +kubebuilder:printcolumn:name="Plan-ID",type="string",JSONPath=".status.planId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// BackupPlan is the Schema for managing AWS Backup plans.
type BackupPlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupPlanSpec   `json:"spec,omitempty"`
	Status BackupPlanStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BackupPlanList contains a list of BackupPlan
type BackupPlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupPlan `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackupPlan{}, &BackupPlanList{})
}
