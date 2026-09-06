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

// SSMDocumentRef references either a managed SSMDocument CR or a direct
// document name.
type SSMDocumentRef struct {
	// Name of an SSMDocument CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// DocumentName is a direct SSM document name (e.g. AWS-RunPatchBaseline),
	// bypassing CR lookup.
	// +optional
	DocumentName string `json:"documentName,omitempty"`
}

// SSMAssociationTarget selects the managed nodes the association applies to.
type SSMAssociationTarget struct {
	// Key is the target key, e.g. InstanceIds, tag:Environment.
	Key string `json:"key"`

	// Values for the target key.
	// +kubebuilder:validation:MinItems=1
	Values []string `json:"values"`
}

// SSMAssociationSpec defines the desired state of an SSM State Manager association.
type SSMAssociationSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is a direct SSM document name to associate. Either name or
	// documentRef must be set.
	// +optional
	Name string `json:"name,omitempty"`

	// DocumentRef references the SSM document to associate.
	// +optional
	DocumentRef *SSMDocumentRef `json:"documentRef,omitempty"`

	// AssociationName is a human-readable name for the association.
	// +optional
	AssociationName string `json:"associationName,omitempty"`

	// Targets select the managed nodes.
	// +optional
	Targets []SSMAssociationTarget `json:"targets,omitempty"`

	// ScheduleExpression is a cron or rate expression for re-application.
	// +optional
	ScheduleExpression string `json:"scheduleExpression,omitempty"`

	// Parameters passed to the document.
	// +optional
	Parameters map[string][]string `json:"parameters,omitempty"`
}

// SSMAssociationStatus defines the observed state of SSMAssociation.
type SSMAssociationStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// AssociationID is the AWS association ID.
	// +optional
	AssociationID string `json:"associationId,omitempty"`

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
// +kubebuilder:printcolumn:name="Association-ID",type="string",JSONPath=".status.associationId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SSMAssociation is the Schema for managing SSM State Manager associations.
type SSMAssociation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SSMAssociationSpec   `json:"spec,omitempty"`
	Status SSMAssociationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SSMAssociationList contains a list of SSMAssociation
type SSMAssociationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SSMAssociation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SSMAssociation{}, &SSMAssociationList{})
}
