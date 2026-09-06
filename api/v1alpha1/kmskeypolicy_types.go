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

// KMSKeyPolicySpec defines the desired state of a KMS Key Policy.
type KMSKeyPolicySpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// KeyRef references the KMS key.
	KeyRef KMSKeyRef `json:"keyRef"`

	// PolicyDocument is the JSON key policy document.
	PolicyDocument string `json:"policyDocument"`

	// PolicyName is the name of the policy (default is "default").
	// +optional
	PolicyName string `json:"policyName,omitempty"`
}

// KMSKeyPolicyStatus defines the observed state of KMSKeyPolicy.
type KMSKeyPolicyStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
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

// KMSKeyPolicy is the Schema for managing KMS Key Policies.
type KMSKeyPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KMSKeyPolicySpec   `json:"spec,omitempty"`
	Status KMSKeyPolicyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KMSKeyPolicyList contains a list of KMSKeyPolicy
type KMSKeyPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KMSKeyPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KMSKeyPolicy{}, &KMSKeyPolicyList{})
}
