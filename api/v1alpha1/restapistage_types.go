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

// RestAPIDeploymentRef references a managed RestAPIDeployment CR or a direct
// deployment ID.
type RestAPIDeploymentRef struct {
	// Name of a RestAPIDeployment CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// DeploymentID is a direct AWS deployment ID, bypassing CR lookup.
	// +optional
	DeploymentID string `json:"deploymentId,omitempty"`
}

// RestAPIStageSpec defines the desired state of a REST API stage.
type RestAPIStageSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// RestAPIRef references the RestAPI this stage belongs to.
	RestAPIRef APIRef `json:"restApiRef"`

	// StageName is the name of the stage. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="stageName is immutable"
	StageName string `json:"stageName"`

	// DeploymentRef references the RestAPIDeployment the stage points at.
	DeploymentRef RestAPIDeploymentRef `json:"deploymentRef"`

	// Description of the stage.
	// +optional
	Description string `json:"description,omitempty"`

	// Variables are stage variables (name/value pairs).
	// +optional
	Variables map[string]string `json:"variables,omitempty"`

	// TracingEnabled enables X-Ray tracing for the stage.
	// +optional
	TracingEnabled bool `json:"tracingEnabled,omitempty"`

	// Tags are AWS resource tags to apply on creation.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// RestAPIStageStatus defines the observed state of RestAPIStage.
type RestAPIStageStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// StageName is the stage name in AWS (also the primary identifier
	// together with the API ID).
	// +optional
	StageName string `json:"stageName,omitempty"`

	// APIID is the resolved REST API identifier the stage was created in.
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
// +kubebuilder:printcolumn:name="Stage",type="string",JSONPath=".status.stageName"
// +kubebuilder:printcolumn:name="API-ID",type="string",JSONPath=".status.apiId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// RestAPIStage is the Schema for managing API Gateway REST API stages.
type RestAPIStage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RestAPIStageSpec   `json:"spec,omitempty"`
	Status RestAPIStageStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RestAPIStageList contains a list of RestAPIStage
type RestAPIStageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RestAPIStage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RestAPIStage{}, &RestAPIStageList{})
}
