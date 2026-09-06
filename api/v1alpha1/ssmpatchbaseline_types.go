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

// SSMPatchFilter selects patches by key/values (e.g. PRODUCT, CLASSIFICATION).
type SSMPatchFilter struct {
	// Key is the patch filter key, e.g. PRODUCT, CLASSIFICATION, SEVERITY.
	Key string `json:"key"`

	// Values for the filter key.
	// +kubebuilder:validation:MinItems=1
	Values []string `json:"values"`
}

// SSMPatchRule is one auto-approval rule of a patch baseline.
type SSMPatchRule struct {
	// ApproveAfterDays auto-approves patches this many days after release.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=360
	// +optional
	ApproveAfterDays *int32 `json:"approveAfterDays,omitempty"`

	// ComplianceLevel assigned to approved patches: CRITICAL, HIGH, MEDIUM,
	// LOW, INFORMATIONAL, or UNSPECIFIED.
	// +kubebuilder:validation:Enum=CRITICAL;HIGH;MEDIUM;LOW;INFORMATIONAL;UNSPECIFIED
	// +optional
	ComplianceLevel string `json:"complianceLevel,omitempty"`

	// PatchFilters select the patches the rule applies to.
	// +kubebuilder:validation:MinItems=1
	PatchFilters []SSMPatchFilter `json:"patchFilters"`
}

// SSMPatchBaselineSpec defines the desired state of an SSM patch baseline.
type SSMPatchBaselineSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the name of the patch baseline.
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=128
	Name string `json:"name"`

	// OperatingSystem the baseline applies to, e.g. AMAZON_LINUX_2, WINDOWS,
	// UBUNTU. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="operatingSystem is immutable"
	OperatingSystem string `json:"operatingSystem"`

	// ApprovalRules are the auto-approval rules.
	// +optional
	ApprovalRules []SSMPatchRule `json:"approvalRules,omitempty"`

	// ApprovedPatches is an explicit list of approved patch IDs.
	// +optional
	ApprovedPatches []string `json:"approvedPatches,omitempty"`

	// RejectedPatches is an explicit list of rejected patch IDs.
	// +optional
	RejectedPatches []string `json:"rejectedPatches,omitempty"`

	// Description of the patch baseline.
	// +optional
	Description string `json:"description,omitempty"`

	// Tags are AWS resource tags to apply on creation.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// SSMPatchBaselineStatus defines the observed state of SSMPatchBaseline.
type SSMPatchBaselineStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// BaselineID is the AWS patch baseline ID (pb-...).
	// +optional
	BaselineID string `json:"baselineId,omitempty"`

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
// +kubebuilder:printcolumn:name="Baseline-ID",type="string",JSONPath=".status.baselineId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SSMPatchBaseline is the Schema for managing SSM patch baselines.
type SSMPatchBaseline struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SSMPatchBaselineSpec   `json:"spec,omitempty"`
	Status SSMPatchBaselineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SSMPatchBaselineList contains a list of SSMPatchBaseline
type SSMPatchBaselineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SSMPatchBaseline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SSMPatchBaseline{}, &SSMPatchBaselineList{})
}
