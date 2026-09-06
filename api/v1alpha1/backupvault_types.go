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

// BackupVaultSpec defines the desired state of an AWS Backup vault.
type BackupVaultSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// VaultName is the name of the backup vault. Immutable after creation.
	// +kubebuilder:validation:MinLength=2
	// +kubebuilder:validation:MaxLength=50
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="vaultName is immutable"
	VaultName string `json:"vaultName"`

	// KMSKeyARN is the server-side encryption key used to protect backups.
	// +optional
	KMSKeyARN string `json:"kmsKeyArn,omitempty"`

	// Tags are AWS resource tags to apply to the vault.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// BackupVaultStatus defines the observed state of BackupVault.
type BackupVaultStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// VaultARN is the ARN of the backup vault.
	// +optional
	VaultARN string `json:"vaultArn,omitempty"`

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
// +kubebuilder:printcolumn:name="Vault-ARN",type="string",JSONPath=".status.vaultArn"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// BackupVault is the Schema for managing AWS Backup vaults.
type BackupVault struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupVaultSpec   `json:"spec,omitempty"`
	Status BackupVaultStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BackupVaultList contains a list of BackupVault
type BackupVaultList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupVault `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackupVault{}, &BackupVaultList{})
}
