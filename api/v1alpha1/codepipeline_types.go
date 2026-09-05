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

// CodePipelineArtifactStore defines the artifact store for a pipeline.
type CodePipelineArtifactStore struct {
	// Type is the artifact store type (S3).
	// +kubebuilder:validation:Enum=S3
	Type string `json:"type"`

	// Location is the S3 bucket name.
	Location string `json:"location"`
}

// CodePipelineActionTypeID identifies a CodePipeline action type.
type CodePipelineActionTypeID struct {
	// Category is the action category (Source, Build, Deploy, etc.).
	Category string `json:"category"`

	// Owner is the action owner (AWS, ThirdParty, or Custom).
	Owner string `json:"owner"`

	// Provider is the action provider (e.g. CodeBuild, S3).
	Provider string `json:"provider"`

	// Version is the action version (typically "1").
	Version string `json:"version"`
}

// CodePipelineAction defines a pipeline action.
type CodePipelineAction struct {
	// Name is the action name.
	Name string `json:"name"`

	// ActionTypeID identifies the action type.
	ActionTypeID CodePipelineActionTypeID `json:"actionTypeID"`

	// Configuration is the action configuration key/value pairs.
	// +optional
	Configuration map[string]string `json:"configuration,omitempty"`

	// InputArtifacts is the list of input artifact names.
	// +optional
	InputArtifacts []string `json:"inputArtifacts,omitempty"`

	// OutputArtifacts is the list of output artifact names.
	// +optional
	OutputArtifacts []string `json:"outputArtifacts,omitempty"`

	// RunOrder is the action run order within the stage.
	// +optional
	RunOrder *int32 `json:"runOrder,omitempty"`
}

// CodePipelineStage defines a pipeline stage.
type CodePipelineStage struct {
	// Name is the stage name.
	Name string `json:"name"`

	// Actions is the list of actions in this stage.
	Actions []CodePipelineAction `json:"actions"`
}

// CodePipelineSpec defines the desired state of a CodePipeline pipeline.
type CodePipelineSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// PipelineName is the name of the pipeline. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="pipelineName is immutable"
	PipelineName string `json:"pipelineName"`

	// RoleARN is the IAM role ARN for CodePipeline.
	RoleARN string `json:"roleARN"`

	// ArtifactStore defines the artifact store.
	ArtifactStore CodePipelineArtifactStore `json:"artifactStore"`

	// Stages is the ordered list of pipeline stages (minimum 2).
	Stages []CodePipelineStage `json:"stages"`

	// Tags are metadata tags for the pipeline.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// CodePipelineStatus defines the observed state of CodePipeline.
type CodePipelineStatus struct {
	// PipelineARN is the ARN of the pipeline.
	// +optional
	PipelineARN string `json:"pipelineARN,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.pipelineARN"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CodePipeline is the Schema for managing AWS CodePipeline pipelines.
type CodePipeline struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CodePipelineSpec   `json:"spec,omitempty"`
	Status CodePipelineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CodePipelineList contains a list of CodePipeline.
type CodePipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CodePipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CodePipeline{}, &CodePipelineList{})
}
