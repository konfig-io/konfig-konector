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

// DBParameter defines a single DB parameter override.
type DBParameter struct {
	// ParameterName is the name of the DB parameter.
	ParameterName string `json:"parameterName"`

	// ParameterValue is the value of the DB parameter.
	ParameterValue string `json:"parameterValue"`

	// ApplyMethod is "immediate" or "pending-reboot".
	// +kubebuilder:validation:Enum=immediate;pending-reboot
	// +optional
	ApplyMethod string `json:"applyMethod,omitempty"`
}

// DBParameterGroupSpec defines the desired state of an RDS DB Parameter Group.
type DBParameterGroupSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// DBParameterGroupName is the name of the parameter group. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="dbParameterGroupName is immutable"
	DBParameterGroupName string `json:"dbParameterGroupName"`

	// DBParameterGroupFamily is the DB engine family (e.g. mysql8.0, postgres15).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="dbParameterGroupFamily is immutable"
	DBParameterGroupFamily string `json:"dbParameterGroupFamily"`

	// Description is the description for the parameter group.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="description is immutable"
	Description string `json:"description"`

	// Parameters is the list of parameter overrides.
	// +optional
	Parameters []DBParameter `json:"parameters,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// DBParameterGroupStatus defines the observed state of DBParameterGroup.
type DBParameterGroupStatus struct {
	// ARN is the Amazon Resource Name of the DB parameter group.
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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.arn"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DBParameterGroup is the Schema for managing RDS DB Parameter Groups.
type DBParameterGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DBParameterGroupSpec   `json:"spec,omitempty"`
	Status DBParameterGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DBParameterGroupList contains a list of DBParameterGroup
type DBParameterGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DBParameterGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DBParameterGroup{}, &DBParameterGroupList{})
}
