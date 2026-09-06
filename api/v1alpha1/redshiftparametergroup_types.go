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

// RedshiftParameter is a single name/value parameter setting.
type RedshiftParameter struct {
	// Name is the parameter name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Value is the parameter value.
	Value string `json:"value"`
}

// RedshiftParameterGroupSpec defines the desired state of a Redshift cluster
// parameter group.
type RedshiftParameterGroupSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the name of the parameter group. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Family is the parameter group family (e.g. redshift-1.0). Immutable
	// after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="family is immutable"
	Family string `json:"family"`

	// Description is a description of the parameter group.
	Description string `json:"description"`

	// Parameters is the list of parameter overrides to apply.
	// +optional
	Parameters []RedshiftParameter `json:"parameters,omitempty"`

	// Tags are AWS resource tags to apply at creation.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// RedshiftParameterGroupStatus defines the observed state of
// RedshiftParameterGroup.
type RedshiftParameterGroupStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ParameterGroupName is the name of the parameter group in AWS.
	// +optional
	ParameterGroupName string `json:"parameterGroupName,omitempty"`

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
// +kubebuilder:printcolumn:name="Parameter-Group",type="string",JSONPath=".status.parameterGroupName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// RedshiftParameterGroup is the Schema for managing Redshift cluster
// parameter groups.
type RedshiftParameterGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedshiftParameterGroupSpec   `json:"spec,omitempty"`
	Status RedshiftParameterGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RedshiftParameterGroupList contains a list of RedshiftParameterGroup
type RedshiftParameterGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedshiftParameterGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RedshiftParameterGroup{}, &RedshiftParameterGroupList{})
}
