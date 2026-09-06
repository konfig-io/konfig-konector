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

// CodeDeployApplicationSpec defines the desired state of a CodeDeploy application.
type CodeDeployApplicationSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ApplicationName is the name of the application. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="applicationName is immutable"
	ApplicationName string `json:"applicationName"`

	// ComputePlatform is the destination platform (Server, Lambda, or ECS).
	// +kubebuilder:validation:Enum=Server;Lambda;ECS
	// +optional
	ComputePlatform string `json:"computePlatform,omitempty"`

	// Tags are metadata tags for the application.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// CodeDeployApplicationStatus defines the observed state of CodeDeployApplication.
type CodeDeployApplicationStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ApplicationID is the CodeDeploy application ID.
	// +optional
	ApplicationID string `json:"applicationID,omitempty"`

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
// +kubebuilder:printcolumn:name="AppID",type="string",JSONPath=".status.applicationID"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CodeDeployApplication is the Schema for managing AWS CodeDeploy applications.
type CodeDeployApplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CodeDeployApplicationSpec   `json:"spec,omitempty"`
	Status CodeDeployApplicationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CodeDeployApplicationList contains a list of CodeDeployApplication.
type CodeDeployApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CodeDeployApplication `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CodeDeployApplication{}, &CodeDeployApplicationList{})
}
