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

// SecretSpec defines the desired state of a Secrets Manager Secret.
type SecretSpec struct {
	// SecretName is the name of the secret. Immutable.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="secretName is immutable"
	SecretName string `json:"secretName"`

	// Description is a human-readable description.
	// +optional
	Description string `json:"description,omitempty"`

	// KMSKeyARN is the KMS key ARN for encrypting the secret.
	// +optional
	KMSKeyARN string `json:"kmsKeyArn,omitempty"`

	// SecretStringRef is a reference to a Kubernetes Secret containing the secret value.
	// +optional
	SecretStringRef *SecretRef `json:"secretStringRef,omitempty"`

	// RecoveryWindowInDays is the number of days before permanent deletion (7-30).
	// Unset (0) uses the AWS default of 30 days.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=30
	// +optional
	RecoveryWindowInDays int32 `json:"recoveryWindowInDays,omitempty"`

	// ForceDelete permanently deletes the secret immediately with no recovery
	// window. This is unrecoverable; it must be set explicitly.
	// +optional
	ForceDelete bool `json:"forceDelete,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// SecretStatus defines the observed state of Secret.
type SecretStatus struct {
	// ARN is the ARN of the secret.
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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.arn"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Secret is the Schema for managing AWS Secrets Manager Secrets.
type Secret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SecretSpec   `json:"spec,omitempty"`
	Status SecretStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SecretList contains a list of Secret
type SecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Secret `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Secret{}, &SecretList{})
}
