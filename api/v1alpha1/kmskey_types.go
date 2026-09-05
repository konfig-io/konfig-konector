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

// KMSKeySpec defines the desired state of a KMS Key.
type KMSKeySpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Description is a human-readable description for the key.
	// +optional
	Description string `json:"description,omitempty"`

	// KeyUsage is the cryptographic operations the key supports.
	// +kubebuilder:validation:Enum=ENCRYPT_DECRYPT;SIGN_VERIFY;GENERATE_VERIFY_MAC
	// +optional
	KeyUsage string `json:"keyUsage,omitempty"`

	// KeySpec is the type of KMS key to create.
	// +kubebuilder:validation:Enum=SYMMETRIC_DEFAULT;RSA_2048;RSA_3072;RSA_4096;ECC_NIST_P256;ECC_NIST_P384;ECC_NIST_P521;ECC_SECG_P256K1;HMAC_224;HMAC_256;HMAC_384;HMAC_512
	// +optional
	KeySpec string `json:"keySpec,omitempty"`

	// Policy is the JSON key policy document.
	// +optional
	Policy string `json:"policy,omitempty"`

	// EnableKeyRotation enables automatic annual key rotation.
	// +optional
	EnableKeyRotation bool `json:"enableKeyRotation,omitempty"`

	// PendingWindowInDays is the waiting period (7-30 days) before scheduled deletion.
	// +kubebuilder:validation:Minimum=7
	// +kubebuilder:validation:Maximum=30
	// +optional
	PendingWindowInDays int32 `json:"pendingWindowInDays,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// KMSKeyStatus defines the observed state of KMSKey.
type KMSKeyStatus struct {
	// KeyID is the AWS KMS key ID.
	// +optional
	KeyID string `json:"keyId,omitempty"`

	// ARN is the ARN of the KMS key.
	// +optional
	ARN string `json:"arn,omitempty"`

	// KeyState is the current state of the key.
	// +optional
	KeyState string `json:"keyState,omitempty"`

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
// +kubebuilder:printcolumn:name="Key-ID",type="string",JSONPath=".status.keyId"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.keyState"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// KMSKey is the Schema for managing AWS KMS Keys.
type KMSKey struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KMSKeySpec   `json:"spec,omitempty"`
	Status KMSKeyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KMSKeyList contains a list of KMSKey
type KMSKeyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KMSKey `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KMSKey{}, &KMSKeyList{})
}
