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

// BatchResourceRequirement declares a resource (VCPU, MEMORY, GPU) required
// by the container.
type BatchResourceRequirement struct {
	// Type of resource.
	// +kubebuilder:validation:Enum=VCPU;MEMORY;GPU
	Type string `json:"type"`

	// Value is the quantity of the resource (e.g. "1" vCPU, "2048" MiB).
	Value string `json:"value"`
}

// BatchContainerProperties defines the container run by a Batch job.
type BatchContainerProperties struct {
	// Image used to start the container.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// ResourceRequirements declare VCPU/MEMORY/GPU requirements.
	// +optional
	ResourceRequirements []BatchResourceRequirement `json:"resourceRequirements,omitempty"`

	// JobRoleArn is a direct IAM role ARN the job assumes.
	// +optional
	JobRoleArn string `json:"jobRoleArn,omitempty"`

	// JobRoleRef references an IAMRole CR for the job role.
	// +optional
	JobRoleRef *RoleRef `json:"jobRoleRef,omitempty"`

	// ExecutionRoleArn is a direct IAM role ARN for the execution role
	// (required for Fargate).
	// +optional
	ExecutionRoleArn string `json:"executionRoleArn,omitempty"`

	// ExecutionRoleRef references an IAMRole CR for the execution role.
	// +optional
	ExecutionRoleRef *RoleRef `json:"executionRoleRef,omitempty"`

	// Command passed to the container.
	// +optional
	Command []string `json:"command,omitempty"`

	// Environment variables for the container.
	// +optional
	Environment map[string]string `json:"environment,omitempty"`
}

// BatchRetryStrategy configures job retries.
type BatchRetryStrategy struct {
	// Attempts is the number of times to move a job to RUNNABLE status (1-10).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	Attempts int32 `json:"attempts"`
}

// BatchJobTimeout configures the job timeout.
type BatchJobTimeout struct {
	// AttemptDurationSeconds after which Batch terminates unfinished jobs.
	// +kubebuilder:validation:Minimum=60
	AttemptDurationSeconds int32 `json:"attemptDurationSeconds"`
}

// BatchJobDefinitionSpec defines the desired state of a Batch job definition.
type BatchJobDefinitionSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name of the job definition. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Type of job definition.
	// +kubebuilder:validation:Enum=container
	Type string `json:"type"`

	// ContainerProperties describe the container run by the job.
	ContainerProperties BatchContainerProperties `json:"containerProperties"`

	// PlatformCapabilities required by the job (EC2 or FARGATE).
	// +optional
	PlatformCapabilities []string `json:"platformCapabilities,omitempty"`

	// RetryStrategy configures retries for failed jobs.
	// +optional
	RetryStrategy *BatchRetryStrategy `json:"retryStrategy,omitempty"`

	// Timeout configures the job timeout.
	// +optional
	Timeout *BatchJobTimeout `json:"timeout,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// BatchJobDefinitionStatus defines the observed state of BatchJobDefinition.
type BatchJobDefinitionStatus struct {
	// JobDefinitionARN is the ARN of the active job definition revision.
	// +optional
	JobDefinitionARN string `json:"jobDefinitionArn,omitempty"`

	// Revision is the current job definition revision.
	// +optional
	Revision int32 `json:"revision,omitempty"`

	// SpecHash tracks the spec that produced the current revision.
	// +optional
	SpecHash string `json:"specHash,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.jobDefinitionArn"
// +kubebuilder:printcolumn:name="Revision",type="integer",JSONPath=".status.revision"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// BatchJobDefinition is the Schema for managing AWS Batch job definitions.
type BatchJobDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BatchJobDefinitionSpec   `json:"spec,omitempty"`
	Status BatchJobDefinitionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BatchJobDefinitionList contains a list of BatchJobDefinition
type BatchJobDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BatchJobDefinition `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BatchJobDefinition{}, &BatchJobDefinitionList{})
}
