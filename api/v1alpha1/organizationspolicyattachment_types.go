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

// OrganizationsPolicyRef references either a managed OrganizationsPolicy CR
// or a direct AWS Organizations policy ID.
type OrganizationsPolicyRef struct {
	// Name of an OrganizationsPolicy CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// PolicyID is a direct AWS Organizations policy ID (p-...).
	// If set, Name is ignored.
	// +optional
	PolicyID string `json:"policyId,omitempty"`
}

// OrganizationsPolicyAttachmentSpec attaches an Organizations policy to a
// root, organizational unit, or account. Attachments only reconcile
// successfully from the organization's management (or delegated
// administrator) account.
type OrganizationsPolicyAttachmentSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// PolicyRef references the policy to attach.
	PolicyRef OrganizationsPolicyRef `json:"policyRef"`

	// TargetID is the root ID (r-...), OU ID (ou-...), or 12-digit account ID
	// to attach the policy to. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="targetId is immutable"
	TargetID string `json:"targetId"`
}

// OrganizationsPolicyAttachmentStatus defines the observed state of
// OrganizationsPolicyAttachment.
type OrganizationsPolicyAttachmentStatus struct {
	// PolicyID is the resolved AWS policy ID that was attached.
	// +optional
	PolicyID string `json:"policyId,omitempty"`

	// TargetID is the target the policy was attached to.
	// +optional
	TargetID string `json:"targetId,omitempty"`

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
// +kubebuilder:printcolumn:name="Policy-ID",type="string",JSONPath=".status.policyId"
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".spec.targetId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OrganizationsPolicyAttachment is the Schema for managing AWS Organizations
// policy attachments.
type OrganizationsPolicyAttachment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OrganizationsPolicyAttachmentSpec   `json:"spec,omitempty"`
	Status OrganizationsPolicyAttachmentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OrganizationsPolicyAttachmentList contains a list of OrganizationsPolicyAttachment
type OrganizationsPolicyAttachmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OrganizationsPolicyAttachment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OrganizationsPolicyAttachment{}, &OrganizationsPolicyAttachmentList{})
}
