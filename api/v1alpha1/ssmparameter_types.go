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

// SSMParameterSpec defines the desired state of an SSM Parameter.
type SSMParameterSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ParameterName is the full name of the parameter (e.g. /myapp/db/password). Immutable.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="parameterName is immutable"
	ParameterName string `json:"parameterName"`

	// Type is the parameter type.
	// +kubebuilder:validation:Enum=String;StringList;SecureString
	Type string `json:"type"`

	// Value is the parameter value. For SecureString, use valueFrom instead.
	// +optional
	Value string `json:"value,omitempty"`

	// ValueFrom references a Kubernetes Secret containing the parameter value.
	// +optional
	ValueFrom *SecretRef `json:"valueFrom,omitempty"`

	// Description is a human-readable description.
	// +optional
	Description string `json:"description,omitempty"`

	// KMSKeyID is the KMS key ID for SecureString parameters.
	// +optional
	KMSKeyID string `json:"kmsKeyId,omitempty"`

	// Tier is the parameter tier.
	// +kubebuilder:validation:Enum=Standard;Advanced;Intelligent-Tiering
	// +optional
	Tier string `json:"tier,omitempty"`

	// Overwrite allows updating an existing parameter.
	// +optional
	Overwrite bool `json:"overwrite,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// SSMParameterStatus defines the observed state of SSMParameter.
type SSMParameterStatus struct {
	// ARN is the ARN of the SSM parameter.
	// +optional
	ARN string `json:"arn,omitempty"`

	// Version is the current version of the parameter.
	// +optional
	Version int64 `json:"version,omitempty"`

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
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type"
// +kubebuilder:printcolumn:name="Version",type="integer",JSONPath=".status.version"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SSMParameter is the Schema for managing AWS SSM Parameters.
type SSMParameter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SSMParameterSpec   `json:"spec,omitempty"`
	Status SSMParameterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SSMParameterList contains a list of SSMParameter
type SSMParameterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SSMParameter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SSMParameter{}, &SSMParameterList{})
}
