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

// CodeBuildSource defines the source configuration for a CodeBuild project.
type CodeBuildSource struct {
	// Type is the source type (CODECOMMIT, CODEPIPELINE, GITHUB, S3, BITBUCKET, NO_SOURCE).
	// +kubebuilder:validation:Enum=CODECOMMIT;CODEPIPELINE;GITHUB;S3;BITBUCKET;NO_SOURCE
	Type string `json:"type"`

	// Location is the source location (e.g. repository URL or S3 path).
	// +optional
	Location string `json:"location,omitempty"`

	// Buildspec is the inline buildspec or path to the buildspec file.
	// +optional
	Buildspec string `json:"buildspec,omitempty"`
}

// CodeBuildArtifacts defines the artifacts configuration.
type CodeBuildArtifacts struct {
	// Type is the artifact type (CODEPIPELINE, S3, NO_ARTIFACTS).
	// +kubebuilder:validation:Enum=CODEPIPELINE;S3;NO_ARTIFACTS
	Type string `json:"type"`

	// Location is the S3 bucket name (required when type is S3).
	// +optional
	Location string `json:"location,omitempty"`
}

// CodeBuildEnvironment defines the build environment.
type CodeBuildEnvironment struct {
	// Type is the environment type (LINUX_CONTAINER, WINDOWS_SERVER_2019_CONTAINER, ARM_CONTAINER, etc.).
	// +kubebuilder:validation:Enum=LINUX_CONTAINER;WINDOWS_SERVER_2019_CONTAINER;ARM_CONTAINER;LINUX_GPU_CONTAINER
	Type string `json:"type"`

	// Image is the Docker image to use for the build environment.
	Image string `json:"image"`

	// ComputeType is the build instance type.
	// +kubebuilder:validation:Enum=BUILD_GENERAL1_SMALL;BUILD_GENERAL1_MEDIUM;BUILD_GENERAL1_LARGE;BUILD_GENERAL1_2XLARGE
	ComputeType string `json:"computeType"`

	// PrivilegedMode enables Docker daemon access.
	// +optional
	PrivilegedMode bool `json:"privilegedMode,omitempty"`
}

// CodeBuildProjectSpec defines the desired state of a CodeBuild project.
type CodeBuildProjectSpec struct {
	// Name is the project name. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Description is an optional project description.
	// +optional
	Description string `json:"description,omitempty"`

	// ServiceRoleARN is the ARN of the IAM role that enables CodeBuild to access resources.
	ServiceRoleARN string `json:"serviceRoleARN"`

	// Source defines the source configuration.
	Source CodeBuildSource `json:"source"`

	// Artifacts defines where build output is stored.
	Artifacts CodeBuildArtifacts `json:"artifacts"`

	// Environment defines the build environment.
	Environment CodeBuildEnvironment `json:"environment"`

	// Tags are metadata tags for the project.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// CodeBuildProjectStatus defines the observed state of CodeBuildProject.
type CodeBuildProjectStatus struct {
	// ProjectARN is the ARN of the CodeBuild project.
	// +optional
	ProjectARN string `json:"projectARN,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.projectARN"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CodeBuildProject is the Schema for managing AWS CodeBuild projects.
type CodeBuildProject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CodeBuildProjectSpec   `json:"spec,omitempty"`
	Status CodeBuildProjectStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CodeBuildProjectList contains a list of CodeBuildProject.
type CodeBuildProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CodeBuildProject `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CodeBuildProject{}, &CodeBuildProjectList{})
}
