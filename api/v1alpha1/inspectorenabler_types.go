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

// InspectorResourceType is an Amazon Inspector scan type.
// +kubebuilder:validation:Enum=EC2;ECR;LAMBDA;LAMBDA_CODE
type InspectorResourceType string

// InspectorEnablerSpec defines the desired state of Amazon Inspector
// activation in this account/region. Inspector activation is a singleton:
// create at most one InspectorEnabler CR per region.
type InspectorEnablerSpec struct {
	// ResourceTypes are the scan types to enable.
	// +kubebuilder:validation:MinItems=1
	ResourceTypes []InspectorResourceType `json:"resourceTypes"`
}

// InspectorResourceStatus reports the per-resource-type Inspector scan status.
type InspectorResourceStatus struct {
	// EC2 scan status.
	// +optional
	EC2 string `json:"ec2,omitempty"`
	// ECR scan status.
	// +optional
	ECR string `json:"ecr,omitempty"`
	// Lambda scan status.
	// +optional
	Lambda string `json:"lambda,omitempty"`
	// LambdaCode scan status.
	// +optional
	LambdaCode string `json:"lambdaCode,omitempty"`
}

// InspectorEnablerStatus defines the observed state of InspectorEnabler.
type InspectorEnablerStatus struct {
	// AccountID is the AWS account whose Inspector status is managed.
	// +optional
	AccountID string `json:"accountId,omitempty"`

	// AccountStatus is the overall Inspector status for the account.
	// +optional
	AccountStatus string `json:"accountStatus,omitempty"`

	// ResourceStatus reports the per-resource-type scan status.
	// +optional
	ResourceStatus *InspectorResourceStatus `json:"resourceStatus,omitempty"`

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
// +kubebuilder:printcolumn:name="Account",type="string",JSONPath=".status.accountId"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.accountStatus"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// InspectorEnabler is the Schema for managing Amazon Inspector activation.
type InspectorEnabler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InspectorEnablerSpec   `json:"spec,omitempty"`
	Status InspectorEnablerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// InspectorEnablerList contains a list of InspectorEnabler
type InspectorEnablerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InspectorEnabler `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InspectorEnabler{}, &InspectorEnablerList{})
}
