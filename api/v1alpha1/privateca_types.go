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

// PrivateCASubject contains X.500 distinguished name information for the CA.
type PrivateCASubject struct {
	// CommonName (CN) of the CA.
	// +optional
	CommonName string `json:"commonName,omitempty"`

	// Organization (O) the CA belongs to.
	// +optional
	Organization string `json:"organization,omitempty"`

	// OrganizationalUnit (OU) within the organization.
	// +optional
	OrganizationalUnit string `json:"organizationalUnit,omitempty"`

	// Country is the two-letter country code (C).
	// +optional
	Country string `json:"country,omitempty"`

	// State or province (ST).
	// +optional
	State string `json:"state,omitempty"`

	// Locality such as a city or town (L).
	// +optional
	Locality string `json:"locality,omitempty"`
}

// PrivateCACrlConfiguration configures CRL publication for the CA.
type PrivateCACrlConfiguration struct {
	// Enabled turns on certificate revocation list publication.
	Enabled bool `json:"enabled"`

	// S3BucketName is the bucket CRLs are published to.
	// +optional
	S3BucketName string `json:"s3BucketName,omitempty"`

	// ExpirationInDays is the CRL validity period.
	// +kubebuilder:validation:Minimum=1
	// +optional
	ExpirationInDays int32 `json:"expirationInDays,omitempty"`
}

// PrivateCARevocationConfiguration configures revocation for the CA.
type PrivateCARevocationConfiguration struct {
	// Crl configures certificate revocation list publication.
	// +optional
	Crl *PrivateCACrlConfiguration `json:"crl,omitempty"`
}

// PrivateCASpec defines the desired state of a Private Certificate Authority.
type PrivateCASpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Type of the certificate authority. Immutable after creation.
	// +kubebuilder:validation:Enum=ROOT;SUBORDINATE
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="type is immutable"
	Type string `json:"type"`

	// KeyAlgorithm for the CA key pair. Immutable after creation.
	// +kubebuilder:validation:Enum=RSA_2048;RSA_4096;EC_prime256v1;EC_secp384r1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="keyAlgorithm is immutable"
	KeyAlgorithm string `json:"keyAlgorithm"`

	// SigningAlgorithm the CA uses to sign certificate requests.
	// Immutable after creation.
	// +kubebuilder:validation:Enum=SHA256WITHRSA;SHA384WITHRSA;SHA512WITHRSA;SHA256WITHECDSA;SHA384WITHECDSA;SHA512WITHECDSA
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="signingAlgorithm is immutable"
	SigningAlgorithm string `json:"signingAlgorithm"`

	// Subject is the X.500 distinguished name of the CA.
	Subject PrivateCASubject `json:"subject"`

	// RevocationConfiguration configures CRL support.
	// +optional
	RevocationConfiguration *PrivateCARevocationConfiguration `json:"revocationConfiguration,omitempty"`

	// UsageMode of the CA.
	// +kubebuilder:validation:Enum=GENERAL_PURPOSE;SHORT_LIVED_CERTIFICATE
	// +optional
	UsageMode string `json:"usageMode,omitempty"`

	// PermanentDeletionTimeInDays is the restore window applied when the CA
	// is deleted (7-30 days).
	// +kubebuilder:validation:Minimum=7
	// +kubebuilder:validation:Maximum=30
	// +kubebuilder:default=30
	// +optional
	PermanentDeletionTimeInDays int32 `json:"permanentDeletionTimeInDays,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// PrivateCAStatus defines the observed state of PrivateCA.
type PrivateCAStatus struct {
	// ARN of the certificate authority.
	// +optional
	ARN string `json:"arn,omitempty"`

	// Status is the current CA status (e.g. PENDING_CERTIFICATE, ACTIVE).
	// +optional
	Status string `json:"status,omitempty"`

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
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PrivateCA is the Schema for managing AWS Private Certificate Authorities.
type PrivateCA struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PrivateCASpec   `json:"spec,omitempty"`
	Status PrivateCAStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PrivateCAList contains a list of PrivateCA
type PrivateCAList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PrivateCA `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PrivateCA{}, &PrivateCAList{})
}
