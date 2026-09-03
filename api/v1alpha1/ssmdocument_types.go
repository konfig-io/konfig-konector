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

// SSMDocumentSpec defines the desired state of an SSM Document.
type SSMDocumentSpec struct {
	// Name is the name of the SSM document.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Content is the document content in JSON or YAML format.
	Content string `json:"content"`

	// DocumentType is the type of the document.
	// +kubebuilder:validation:Enum=Command;Policy;Automation;Session;Package;ApplicationConfiguration;ApplicationConfigurationSchema;DeploymentStrategy;ChangeCalendar;Runbook
	// +optional
	DocumentType string `json:"documentType,omitempty"`

	// DocumentFormat is the format of the document (JSON or YAML).
	// +kubebuilder:validation:Enum=JSON;YAML;TEXT
	// +optional
	DocumentFormat string `json:"documentFormat,omitempty"`

	// VersionName is an optional version identifier.
	// +optional
	VersionName string `json:"versionName,omitempty"`

	// Tags are metadata tags for the document.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// SSMDocumentStatus defines the observed state of SSMDocument.
type SSMDocumentStatus struct {
	// DocumentVersion is the current version of the document.
	// +optional
	DocumentVersion string `json:"documentVersion,omitempty"`

	// Status is the current status of the document.
	// +optional
	Status string `json:"status,omitempty"`

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
// +kubebuilder:printcolumn:name="Name",type="string",JSONPath=".spec.name"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".status.documentVersion"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SSMDocument is the Schema for managing SSM documents.
type SSMDocument struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SSMDocumentSpec   `json:"spec,omitempty"`
	Status SSMDocumentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SSMDocumentList contains a list of SSMDocument.
type SSMDocumentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SSMDocument `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SSMDocument{}, &SSMDocumentList{})
}
