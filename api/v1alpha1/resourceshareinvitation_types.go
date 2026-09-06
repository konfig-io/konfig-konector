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

// ResourceShareRef references a ResourceShare CR (possibly reconciled under a
// different AWSProvider) or a direct RAM resource share ARN.
type ResourceShareRef struct {
	// Name of a ResourceShare CR.
	// +optional
	Name string `json:"name,omitempty"`
	// Namespace of the ResourceShare CR. Defaults to the referring object's namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// ARN is a direct resource share ARN.
	// +optional
	ARN string `json:"arn,omitempty"`
}

// ResourceShareInvitationSpec accepts a RAM resource share in the receiving
// account. Pair it with a ResourceShare in the owning account (under another
// AWSProvider) to complete cross-account sharing from a single cluster.
type ResourceShareInvitationSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ResourceShareRef identifies the share to accept.
	ResourceShareRef ResourceShareRef `json:"resourceShareRef"`
}

// ResourceShareInvitationStatus defines the observed state of ResourceShareInvitation.
type ResourceShareInvitationStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// InvitationARN is the accepted invitation, when one existed.
	// +optional
	InvitationARN string `json:"invitationArn,omitempty"`
	// ResourceShareARN is the share that was accepted.
	// +optional
	ResourceShareARN string `json:"resourceShareArn,omitempty"`
	// SenderAccountID owns the share.
	// +optional
	SenderAccountID string `json:"senderAccountId,omitempty"`
	// Status is the RAM invitation status (PENDING, ACCEPTED, ...).
	// +optional
	Status string `json:"status,omitempty"`
	// ResourceARNs shared with this account through the share.
	// +optional
	ResourceARNs []string `json:"resourceArns,omitempty"`
	// Conditions describe the current state of the resource.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration is the generation last reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// LastSyncTime is the last successful sync.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ResourceShareInvitation accepts an AWS RAM resource share invitation in the
// receiving account.
type ResourceShareInvitation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceShareInvitationSpec   `json:"spec,omitempty"`
	Status ResourceShareInvitationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ResourceShareInvitationList contains a list of ResourceShareInvitation.
type ResourceShareInvitationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceShareInvitation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ResourceShareInvitation{}, &ResourceShareInvitationList{})
}
