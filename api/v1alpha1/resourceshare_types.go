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

// ResourceShareSpec defines the desired state of an AWS RAM resource share.
type ResourceShareSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the name of the resource share.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// ResourceArns are the ARNs of the resources to share.
	// +optional
	ResourceArns []string `json:"resourceArns,omitempty"`

	// Principals to share with: 12-digit account IDs, OU ARNs, organization
	// ARNs, or IAM role/user ARNs.
	// +optional
	Principals []string `json:"principals,omitempty"`

	// AllowExternalPrincipals permits sharing with accounts outside the
	// organization.
	// +optional
	AllowExternalPrincipals bool `json:"allowExternalPrincipals,omitempty"`

	// PermissionArns are RAM permission ARNs to associate with the share.
	// If empty, RAM attaches the default permission per resource type.
	// +optional
	PermissionArns []string `json:"permissionArns,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ResourceShareStatus defines the observed state of ResourceShare.
type ResourceShareStatus struct {
	// ARN is the Amazon Resource Name of the resource share.
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
// +kubebuilder:printcolumn:name="Share-ARN",type="string",JSONPath=".status.arn"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ResourceShare is the Schema for managing AWS RAM resource shares.
type ResourceShare struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceShareSpec   `json:"spec,omitempty"`
	Status ResourceShareStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ResourceShareList contains a list of ResourceShare
type ResourceShareList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceShare `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ResourceShare{}, &ResourceShareList{})
}
