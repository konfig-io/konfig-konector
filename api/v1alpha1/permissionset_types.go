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

// PermissionSetSpec defines the desired state of an IAM Identity Center
// permission set. Permission sets only reconcile successfully from the
// account where the Identity Center instance lives (management or delegated
// administrator account).
type PermissionSetSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// InstanceArn is the ARN of the IAM Identity Center instance under which
	// the permission set is created. Immutable after creation.
	// +kubebuilder:validation:MinLength=10
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="instanceArn is immutable"
	InstanceArn string `json:"instanceArn"`

	// Name is the name of the permission set. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=32
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Description of the permission set.
	// +optional
	Description string `json:"description,omitempty"`

	// SessionDuration is the length of time that application user sessions
	// are valid, in ISO-8601 duration format (e.g. PT8H).
	// +optional
	SessionDuration string `json:"sessionDuration,omitempty"`

	// RelayState redirects users within the application during federation
	// authentication.
	// +optional
	RelayState string `json:"relayState,omitempty"`

	// ManagedPolicies is a list of AWS managed policy ARNs to attach to the
	// permission set.
	// +optional
	ManagedPolicies []string `json:"managedPolicies,omitempty"`

	// InlinePolicy is an IAM policy document (JSON) stored inline in the
	// permission set.
	// +optional
	InlinePolicy string `json:"inlinePolicy,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// PermissionSetStatus defines the observed state of PermissionSet.
type PermissionSetStatus struct {
	// PermissionSetArn is the ARN of the permission set.
	// +optional
	PermissionSetArn string `json:"permissionSetArn,omitempty"`

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
// +kubebuilder:printcolumn:name="PermissionSet-ARN",type="string",JSONPath=".status.permissionSetArn"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PermissionSet is the Schema for managing IAM Identity Center permission sets.
type PermissionSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PermissionSetSpec   `json:"spec,omitempty"`
	Status PermissionSetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PermissionSetList contains a list of PermissionSet
type PermissionSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PermissionSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PermissionSet{}, &PermissionSetList{})
}
