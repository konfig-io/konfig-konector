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

// KMSKeyRef references either a managed KMSKey CR or a direct key ID/ARN.
type KMSKeyRef struct {
	// Name of a KMSKey CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// KeyID is a direct AWS KMS key ID or ARN.
	// +optional
	KeyID string `json:"keyId,omitempty"`
}

// KMSAliasSpec defines the desired state of a KMS Alias.
type KMSAliasSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// AliasName is the alias name. Must start with "alias/". Immutable.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^alias/[a-zA-Z0-9/_-]+$`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="aliasName is immutable"
	AliasName string `json:"aliasName"`

	// TargetKeyRef references the KMS key to alias.
	TargetKeyRef KMSKeyRef `json:"targetKeyRef"`
}

// KMSAliasStatus defines the observed state of KMSAlias.
type KMSAliasStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ARN is the ARN of the alias.
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
// +kubebuilder:printcolumn:name="Alias",type="string",JSONPath=".spec.aliasName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// KMSAlias is the Schema for managing KMS Aliases.
type KMSAlias struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KMSAliasSpec   `json:"spec,omitempty"`
	Status KMSAliasStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KMSAliasList contains a list of KMSAlias
type KMSAliasList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KMSAlias `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KMSAlias{}, &KMSAliasList{})
}
