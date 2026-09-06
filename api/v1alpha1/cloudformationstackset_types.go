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

// CloudFormationStackSetSpec defines the desired state of a CloudFormation StackSet.
type CloudFormationStackSetSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// StackSetName is the name of the StackSet. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="stackSetName is immutable"
	StackSetName string `json:"stackSetName"`

	// TemplateBody is the CloudFormation template body (JSON or YAML string).
	// +optional
	TemplateBody string `json:"templateBody,omitempty"`

	// TemplateURL is an S3 URL for the template.
	// +optional
	TemplateURL string `json:"templateURL,omitempty"`

	// Parameters is the list of template parameters.
	// +optional
	Parameters []CloudFormationParameter `json:"parameters,omitempty"`

	// Capabilities is the list of CloudFormation capabilities.
	// +optional
	Capabilities []string `json:"capabilities,omitempty"`

	// Description is a description of the StackSet.
	// +optional
	Description string `json:"description,omitempty"`

	// Tags are metadata tags for the StackSet.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// CloudFormationStackSetStatus defines the observed state of CloudFormationStackSet.
type CloudFormationStackSetStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// StackSetID is the unique identifier of the StackSet.
	// +optional
	StackSetID string `json:"stackSetID,omitempty"`

	// StackSetStatus is the current status of the StackSet.
	// +optional
	StackSetStatus string `json:"stackSetStatus,omitempty"`

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
// +kubebuilder:printcolumn:name="StackSetID",type="string",JSONPath=".status.stackSetID"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.stackSetStatus"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CloudFormationStackSet is the Schema for managing CloudFormation StackSets.
type CloudFormationStackSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudFormationStackSetSpec   `json:"spec,omitempty"`
	Status CloudFormationStackSetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CloudFormationStackSetList contains a list of CloudFormationStackSet.
type CloudFormationStackSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudFormationStackSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudFormationStackSet{}, &CloudFormationStackSetList{})
}
