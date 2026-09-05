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

// ECSPortMapping defines a container port mapping.
type ECSPortMapping struct {
	// ContainerPort is the port exposed by the container.
	ContainerPort int32 `json:"containerPort"`
	// Protocol is the transport protocol: tcp or udp. Default: tcp.
	// +kubebuilder:validation:Enum=tcp;udp
	// +optional
	Protocol string `json:"protocol,omitempty"`
}

// ECSContainerDefinition defines a container within a task definition.
type ECSContainerDefinition struct {
	// Name is the container name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Image is the container image URI.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// CPU units to reserve for the container. 1024 = 1 vCPU.
	// +optional
	CPU *int32 `json:"cpu,omitempty"`

	// Memory is the memory in MiB to reserve for the container.
	// +optional
	Memory *int32 `json:"memory,omitempty"`

	// Essential — if true, stopping this container stops the entire task. Default: true.
	// +optional
	Essential *bool `json:"essential,omitempty"`

	// PortMappings are the ports to expose from the container.
	// +optional
	PortMappings []ECSPortMapping `json:"portMappings,omitempty"`

	// Environment variables for the container.
	// +optional
	Environment map[string]string `json:"environment,omitempty"`

	// Command overrides the container CMD.
	// +optional
	Command []string `json:"command,omitempty"`

	// EntryPoint overrides the container ENTRYPOINT.
	// +optional
	EntryPoint []string `json:"entryPoint,omitempty"`

	// LogGroup is a CloudWatch log group name to send container logs to.
	// +optional
	LogGroup string `json:"logGroup,omitempty"`
}

// ECSTaskDefinitionSpec defines the desired state of an ECS Task Definition.
// A new revision is registered whenever the spec changes.
type ECSTaskDefinitionSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Family is the task definition family name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="family is immutable"
	Family string `json:"family"`

	// ContainerDefinitions is the list of containers in the task.
	// +kubebuilder:validation:MinItems=1
	ContainerDefinitions []ECSContainerDefinition `json:"containerDefinitions"`

	// CPU is the number of vCPU units for the task (e.g. "256", "512", "1024").
	// Required for Fargate.
	// +optional
	CPU string `json:"cpu,omitempty"`

	// Memory is the memory in MiB for the task (e.g. "512", "1024").
	// Required for Fargate.
	// +optional
	Memory string `json:"memory,omitempty"`

	// NetworkMode is the Docker networking mode.
	// +kubebuilder:validation:Enum=awsvpc;bridge;host;none
	// +optional
	NetworkMode string `json:"networkMode,omitempty"`

	// ExecutionRoleArn is the ARN of the task execution role.
	// Either executionRoleArn or executionRoleRef must be set for Fargate.
	// +optional
	ExecutionRoleArn string `json:"executionRoleArn,omitempty"`

	// ExecutionRoleRef references an IAMRole CR in the same namespace.
	// +optional
	ExecutionRoleRef *RoleRef `json:"executionRoleRef,omitempty"`

	// TaskRoleArn is the ARN of the task role (permissions for your application code).
	// +optional
	TaskRoleArn string `json:"taskRoleArn,omitempty"`

	// TaskRoleRef references an IAMRole CR in the same namespace.
	// +optional
	TaskRoleRef *RoleRef `json:"taskRoleRef,omitempty"`

	// Tags are AWS resource tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ECSTaskDefinitionStatus defines the observed state of ECSTaskDefinition.
type ECSTaskDefinitionStatus struct {
	// TaskDefinitionARN is the full ARN including revision of the latest registered task definition.
	// +optional
	TaskDefinitionARN string `json:"taskDefinitionArn,omitempty"`

	// Revision is the current task definition revision number.
	// +optional
	Revision int32 `json:"revision,omitempty"`

	// SpecHash is the hash of the spec used to detect changes.
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
// +kubebuilder:printcolumn:name="Family",type="string",JSONPath=".spec.family"
// +kubebuilder:printcolumn:name="Revision",type="integer",JSONPath=".status.revision"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ECSTaskDefinition is the Schema for managing ECS Task Definitions.
type ECSTaskDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ECSTaskDefinitionSpec   `json:"spec,omitempty"`
	Status ECSTaskDefinitionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ECSTaskDefinitionList contains a list of ECSTaskDefinition.
type ECSTaskDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ECSTaskDefinition `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ECSTaskDefinition{}, &ECSTaskDefinitionList{})
}
