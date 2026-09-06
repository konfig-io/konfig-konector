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

// OrganizationsOUSpec defines the desired state of an AWS Organizations
// organizational unit. OUs only reconcile successfully from the
// organization's management (or delegated administrator) account.
type OrganizationsOUSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the friendly name of the organizational unit.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	Name string `json:"name"`

	// ParentID is the ID of the parent root (r-...) or OU (ou-...) to create
	// this OU under. Exactly one of parentId or parentRef must be set.
	// The parent cannot be changed after creation (Organizations has no
	// move-OU API).
	// +optional
	ParentID string `json:"parentId,omitempty"`

	// ParentRef references another OrganizationsOU CR to use as the parent.
	// +optional
	ParentRef *ResourceRef `json:"parentRef,omitempty"`

	// Tags are AWS resource tags to apply to the OU.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// OrganizationsOUStatus defines the observed state of OrganizationsOU.
type OrganizationsOUStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// OUID is the unique identifier (ou-...) of the organizational unit.
	// +optional
	OUID string `json:"ouId,omitempty"`

	// ARN is the Amazon Resource Name of the organizational unit.
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
// +kubebuilder:resource:path=organizationsous
// +kubebuilder:printcolumn:name="OU-ID",type="string",JSONPath=".status.ouId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OrganizationsOU is the Schema for managing AWS Organizations organizational units.
type OrganizationsOU struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OrganizationsOUSpec   `json:"spec,omitempty"`
	Status OrganizationsOUStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OrganizationsOUList contains a list of OrganizationsOU
type OrganizationsOUList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OrganizationsOU `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OrganizationsOU{}, &OrganizationsOUList{})
}
