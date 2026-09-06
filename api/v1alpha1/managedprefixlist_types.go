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

// PrefixListEntry is one CIDR entry in a managed prefix list.
type PrefixListEntry struct {
	// CIDR block for the entry.
	// +kubebuilder:validation:MinLength=1
	CIDR string `json:"cidr"`
	// Description of the entry.
	// +optional
	Description string `json:"description,omitempty"`
}

// ManagedPrefixListSpec defines the desired state of an EC2 managed prefix list.
type ManagedPrefixListSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name of the prefix list.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// AddressFamily of the entries. Immutable after creation.
	// +kubebuilder:validation:Enum=IPv4;IPv6
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="addressFamily is immutable"
	AddressFamily string `json:"addressFamily"`

	// MaxEntries is the maximum number of entries the prefix list can hold.
	// +kubebuilder:validation:Minimum=1
	MaxEntries int32 `json:"maxEntries"`

	// Entries are the CIDR entries of the prefix list.
	// +optional
	Entries []PrefixListEntry `json:"entries,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ManagedPrefixListStatus defines the observed state of ManagedPrefixList.
type ManagedPrefixListStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// PrefixListID is the AWS prefix list identifier.
	// +optional
	PrefixListID string `json:"prefixListId,omitempty"`

	// ARN is the Amazon Resource Name of the prefix list.
	// +optional
	ARN string `json:"arn,omitempty"`

	// Version is the current version number of the prefix list.
	// +optional
	Version int64 `json:"version,omitempty"`

	// State is the lifecycle state reported by AWS.
	// +optional
	State string `json:"state,omitempty"`

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
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.prefixListId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ManagedPrefixList is the Schema for managing EC2 managed prefix lists.
type ManagedPrefixList struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ManagedPrefixListSpec   `json:"spec,omitempty"`
	Status ManagedPrefixListStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ManagedPrefixListList contains a list of ManagedPrefixList
type ManagedPrefixListList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ManagedPrefixList `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ManagedPrefixList{}, &ManagedPrefixListList{})
}
