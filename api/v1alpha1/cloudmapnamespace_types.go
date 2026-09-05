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

// CloudMapNamespaceSpec defines the desired state of a Cloud Map namespace.
type CloudMapNamespaceSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name of the namespace (e.g. example.local). Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Type of the namespace. Immutable after creation.
	// +kubebuilder:validation:Enum=PRIVATE_DNS;PUBLIC_DNS;HTTP
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="type is immutable"
	Type string `json:"type"`

	// VPCRef references the VPC to associate with a PRIVATE_DNS namespace.
	// +optional
	VPCRef *VPCResourceRef `json:"vpcRef,omitempty"`

	// Description of the namespace.
	// +optional
	Description string `json:"description,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// CloudMapNamespaceStatus defines the observed state of CloudMapNamespace.
type CloudMapNamespaceStatus struct {
	// NamespaceID is the ID of the namespace.
	// +optional
	NamespaceID string `json:"namespaceId,omitempty"`

	// ARN of the namespace.
	// +optional
	ARN string `json:"arn,omitempty"`

	// OperationID tracks the async create operation until it completes.
	// +optional
	OperationID string `json:"operationId,omitempty"`

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
// +kubebuilder:printcolumn:name="Namespace-ID",type="string",JSONPath=".status.namespaceId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CloudMapNamespace is the Schema for managing AWS Cloud Map namespaces.
type CloudMapNamespace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudMapNamespaceSpec   `json:"spec,omitempty"`
	Status CloudMapNamespaceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CloudMapNamespaceList contains a list of CloudMapNamespace
type CloudMapNamespaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudMapNamespace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudMapNamespace{}, &CloudMapNamespaceList{})
}
