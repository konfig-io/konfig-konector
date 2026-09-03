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

// KMSGrantSpec defines the desired state of a KMS Grant.
type KMSGrantSpec struct {
	// KeyID is the KMS key ID or ARN to grant permissions on.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="keyId is immutable"
	KeyID string `json:"keyId"`

	// GranteePrincipalARN is the ARN of the principal that gets the grant permissions.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="granteePrincipalArn is immutable"
	GranteePrincipalARN string `json:"granteePrincipalArn"`

	// Operations are the KMS operations the grantee can perform.
	// +kubebuilder:validation:MinItems=1
	Operations []string `json:"operations"`

	// Name is an optional friendly name for the grant.
	// +optional
	Name string `json:"name,omitempty"`

	// RetiringPrincipalARN is the ARN of the principal that can retire the grant.
	// +optional
	RetiringPrincipalARN string `json:"retiringPrincipalArn,omitempty"`
}

// KMSGrantStatus defines the observed state of KMSGrant.
type KMSGrantStatus struct {
	// GrantID is the ID of the grant.
	// +optional
	GrantID string `json:"grantId,omitempty"`

	// GrantToken is the grant token.
	// +optional
	GrantToken string `json:"grantToken,omitempty"`

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
// +kubebuilder:printcolumn:name="GrantID",type="string",JSONPath=".status.grantId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// KMSGrant is the Schema for managing KMS grants.
type KMSGrant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KMSGrantSpec   `json:"spec,omitempty"`
	Status KMSGrantStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KMSGrantList contains a list of KMSGrant.
type KMSGrantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KMSGrant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KMSGrant{}, &KMSGrantList{})
}
