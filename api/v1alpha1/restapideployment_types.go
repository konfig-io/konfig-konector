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

// RestAPIDeploymentSpec defines the desired state of a REST API deployment.
// A deployment is an immutable snapshot of the API: spec changes after
// creation are not applied and surface as an UpdateNotSupported condition.
type RestAPIDeploymentSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// RestAPIRef references the RestAPI to deploy.
	RestAPIRef APIRef `json:"restApiRef"`

	// StageName optionally creates/points a stage at the deployment.
	// +optional
	StageName string `json:"stageName,omitempty"`

	// Description of the deployment.
	// +optional
	Description string `json:"description,omitempty"`
}

// RestAPIDeploymentStatus defines the observed state of RestAPIDeployment.
type RestAPIDeploymentStatus struct {
	// DeploymentID is the deployment identifier.
	// +optional
	DeploymentID string `json:"deploymentId,omitempty"`

	// APIID is the resolved REST API identifier the deployment was created in.
	// +optional
	APIID string `json:"apiId,omitempty"`

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
// +kubebuilder:printcolumn:name="Deployment-ID",type="string",JSONPath=".status.deploymentId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// RestAPIDeployment is the Schema for managing API Gateway REST API deployments.
type RestAPIDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RestAPIDeploymentSpec   `json:"spec,omitempty"`
	Status RestAPIDeploymentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RestAPIDeploymentList contains a list of RestAPIDeployment
type RestAPIDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RestAPIDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RestAPIDeployment{}, &RestAPIDeploymentList{})
}
