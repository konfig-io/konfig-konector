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

// CodeArtifactDomainSpec defines the desired state of a CodeArtifact domain.
type CodeArtifactDomainSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// DomainName is the name of the CodeArtifact domain. Immutable.
	// +kubebuilder:validation:MinLength=2
	// +kubebuilder:validation:MaxLength=50
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="domainName is immutable"
	DomainName string `json:"domainName"`

	// EncryptionKeyRef references the KMS key used to encrypt domain content,
	// either a managed KMSKey CR (name) or a direct key ID/ARN (keyId).
	// If unset, an AWS-managed key is used. Immutable in AWS after creation.
	// +optional
	EncryptionKeyRef *KMSKeyRef `json:"encryptionKeyRef,omitempty"`

	// Tags are AWS resource tags to apply at creation time.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// CodeArtifactDomainStatus defines the observed state of CodeArtifactDomain.
type CodeArtifactDomainStatus struct {
	// ARN is the ARN of the domain.
	// +optional
	ARN string `json:"arn,omitempty"`

	// Owner is the AWS account ID that owns the domain.
	// +optional
	Owner string `json:"owner,omitempty"`

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
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CodeArtifactDomain is the Schema for managing CodeArtifact domains.
type CodeArtifactDomain struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CodeArtifactDomainSpec   `json:"spec,omitempty"`
	Status CodeArtifactDomainStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CodeArtifactDomainList contains a list of CodeArtifactDomain
type CodeArtifactDomainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CodeArtifactDomain `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CodeArtifactDomain{}, &CodeArtifactDomainList{})
}
