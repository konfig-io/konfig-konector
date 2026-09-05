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

// CloudFormationParameter defines a CloudFormation parameter key/value pair.
type CloudFormationParameter struct {
	// ParameterKey is the parameter key.
	ParameterKey string `json:"parameterKey"`

	// ParameterValue is the parameter value.
	ParameterValue string `json:"parameterValue"`
}

// CloudFormationStackSpec defines the desired state of a CloudFormation stack.
type CloudFormationStackSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// StackName is the name of the stack. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="stackName is immutable"
	StackName string `json:"stackName"`

	// TemplateBody is the CloudFormation template body (JSON or YAML string).
	// +optional
	TemplateBody string `json:"templateBody,omitempty"`

	// TemplateURL is an S3 URL for the template.
	// +optional
	TemplateURL string `json:"templateURL,omitempty"`

	// Parameters is the list of template parameters.
	// +optional
	Parameters []CloudFormationParameter `json:"parameters,omitempty"`

	// Capabilities is the list of CloudFormation capabilities (e.g. CAPABILITY_IAM).
	// +optional
	Capabilities []string `json:"capabilities,omitempty"`

	// Tags are metadata tags for the stack.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// CloudFormationStackStatus defines the observed state of CloudFormationStack.
type CloudFormationStackStatus struct {
	// StackID is the unique identifier (ARN) of the stack.
	// +optional
	StackID string `json:"stackID,omitempty"`

	// StackStatus is the current status of the stack (CREATE_COMPLETE, UPDATE_COMPLETE, etc.).
	// +optional
	StackStatus string `json:"stackStatus,omitempty"`

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
// +kubebuilder:printcolumn:name="StackID",type="string",JSONPath=".status.stackID"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.stackStatus"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CloudFormationStack is the Schema for managing CloudFormation stacks.
type CloudFormationStack struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudFormationStackSpec   `json:"spec,omitempty"`
	Status CloudFormationStackStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CloudFormationStackList contains a list of CloudFormationStack.
type CloudFormationStackList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudFormationStack `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudFormationStack{}, &CloudFormationStackList{})
}
