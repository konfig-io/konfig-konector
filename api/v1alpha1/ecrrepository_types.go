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

// ECRRepositorySpec defines the desired state of an ECR Repository.
type ECRRepositorySpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// RepositoryName is the name of the repository. Immutable.
	// +kubebuilder:validation:MinLength=2
	// +kubebuilder:validation:MaxLength=256
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="repositoryName is immutable"
	RepositoryName string `json:"repositoryName"`

	// ImageTagMutability controls whether image tags can be overwritten.
	// +kubebuilder:validation:Enum=MUTABLE;IMMUTABLE
	// +optional
	ImageTagMutability string `json:"imageTagMutability,omitempty"`

	// ScanOnPush enables automatic image scanning on push.
	// +optional
	ScanOnPush bool `json:"scanOnPush,omitempty"`

	// EncryptionType is the encryption type for the repository.
	// +kubebuilder:validation:Enum=AES256;KMS
	// +optional
	EncryptionType string `json:"encryptionType,omitempty"`

	// KMSKeyARN is the KMS key ARN for KMS encryption.
	// +optional
	KMSKeyARN string `json:"kmsKeyArn,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ECRRepositoryStatus defines the observed state of ECRRepository.
type ECRRepositoryStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ARN is the ARN of the repository.
	// +optional
	ARN string `json:"arn,omitempty"`

	// RepositoryURI is the URI of the repository.
	// +optional
	RepositoryURI string `json:"repositoryUri,omitempty"`

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
// +kubebuilder:printcolumn:name="URI",type="string",JSONPath=".status.repositoryUri"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ECRRepository is the Schema for managing ECR Repositories.
type ECRRepository struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ECRRepositorySpec   `json:"spec,omitempty"`
	Status ECRRepositoryStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ECRRepositoryList contains a list of ECRRepository
type ECRRepositoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ECRRepository `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ECRRepository{}, &ECRRepositoryList{})
}
