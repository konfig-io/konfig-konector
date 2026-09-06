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

// ShieldProtectionSpec defines the desired state of a Shield Advanced protection.
type ShieldProtectionSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the friendly name of the protection.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// ResourceARN is the ARN of the resource to protect.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="resourceARN is immutable"
	ResourceARN string `json:"resourceARN"`

	// Tags are metadata tags for the protection.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ShieldProtectionStatus defines the observed state of ShieldProtection.
type ShieldProtectionStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ProtectionID is the unique identifier of the Shield protection.
	// +optional
	ProtectionID string `json:"protectionID,omitempty"`

	// ProtectionARN is the ARN of the protection.
	// +optional
	ProtectionARN string `json:"protectionArn,omitempty"`

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
// +kubebuilder:printcolumn:name="ProtectionID",type="string",JSONPath=".status.protectionID"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ShieldProtection is the Schema for managing AWS Shield Advanced protections.
type ShieldProtection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ShieldProtectionSpec   `json:"spec,omitempty"`
	Status ShieldProtectionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ShieldProtectionList contains a list of ShieldProtection.
type ShieldProtectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ShieldProtection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ShieldProtection{}, &ShieldProtectionList{})
}
