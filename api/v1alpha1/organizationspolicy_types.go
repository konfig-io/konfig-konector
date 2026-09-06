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

// OrganizationsPolicySpec defines the desired state of an AWS Organizations
// policy (SCP, tag policy, backup policy, or AI services opt-out policy).
// Policies only reconcile successfully from the organization's management
// (or delegated administrator) account.
type OrganizationsPolicySpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the friendly name of the policy.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	Name string `json:"name"`

	// Type is the policy type. Immutable after creation.
	// +kubebuilder:validation:Enum=SERVICE_CONTROL_POLICY;TAG_POLICY;BACKUP_POLICY;AISERVICES_OPT_OUT_POLICY
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="type is immutable"
	Type string `json:"type"`

	// Content is the policy document as JSON text. It must comply with the
	// syntax of the policy type.
	// +kubebuilder:validation:MinLength=1
	Content string `json:"content"`

	// Description is an optional description for the policy.
	// +optional
	Description string `json:"description,omitempty"`

	// Tags are AWS resource tags to apply to the policy.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// OrganizationsPolicyStatus defines the observed state of OrganizationsPolicy.
type OrganizationsPolicyStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// PolicyID is the unique identifier (p-...) of the policy.
	// +optional
	PolicyID string `json:"policyId,omitempty"`

	// ARN is the Amazon Resource Name of the policy.
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
// +kubebuilder:printcolumn:name="Policy-ID",type="string",JSONPath=".status.policyId"
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OrganizationsPolicy is the Schema for managing AWS Organizations policies.
type OrganizationsPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OrganizationsPolicySpec   `json:"spec,omitempty"`
	Status OrganizationsPolicyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OrganizationsPolicyList contains a list of OrganizationsPolicy
type OrganizationsPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OrganizationsPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OrganizationsPolicy{}, &OrganizationsPolicyList{})
}
