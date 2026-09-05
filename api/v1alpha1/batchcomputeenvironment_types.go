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

// BatchComputeResources defines the compute resources managed by a
// MANAGED Batch compute environment.
type BatchComputeResources struct {
	// Type of compute resource.
	// +kubebuilder:validation:Enum=FARGATE;FARGATE_SPOT;EC2;SPOT
	Type string `json:"type"`

	// MaxvCpus is the maximum number of vCPUs.
	// +kubebuilder:validation:Minimum=0
	MaxvCpus int32 `json:"maxvCpus"`

	// MinvCpus is the minimum number of vCPUs (EC2/SPOT only).
	// +optional
	MinvCpus *int32 `json:"minvCpus,omitempty"`

	// DesiredvCpus is the desired number of vCPUs (EC2/SPOT only).
	// +optional
	DesiredvCpus *int32 `json:"desiredvCpus,omitempty"`

	// SubnetRefs are the VPC subnets where compute resources are launched.
	// +optional
	SubnetRefs []SubnetRef `json:"subnetRefs,omitempty"`

	// SecurityGroupRefs are the security groups for the compute resources.
	// +optional
	SecurityGroupRefs []SecurityGroupRef `json:"securityGroupRefs,omitempty"`

	// InstanceTypes are EC2 instance types that can be launched (EC2/SPOT only).
	// +optional
	InstanceTypes []string `json:"instanceTypes,omitempty"`

	// InstanceRoleArn is the ECS instance profile applied to EC2 instances
	// (EC2/SPOT only).
	// +optional
	InstanceRoleArn string `json:"instanceRoleArn,omitempty"`
}

// BatchComputeEnvironmentSpec defines the desired state of a Batch compute environment.
type BatchComputeEnvironmentSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name of the compute environment. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Type of the compute environment. Immutable after creation.
	// +kubebuilder:validation:Enum=MANAGED;UNMANAGED
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="type is immutable"
	Type string `json:"type"`

	// State of the compute environment.
	// +kubebuilder:validation:Enum=ENABLED;DISABLED
	// +optional
	State string `json:"state,omitempty"`

	// ComputeResources configures the managed compute resources.
	// Required for MANAGED compute environments.
	// +optional
	ComputeResources *BatchComputeResources `json:"computeResources,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// BatchComputeEnvironmentStatus defines the observed state of BatchComputeEnvironment.
type BatchComputeEnvironmentStatus struct {
	// ComputeEnvironmentARN is the ARN of the compute environment.
	// +optional
	ComputeEnvironmentARN string `json:"computeEnvironmentArn,omitempty"`

	// Status is the current compute environment status (e.g. VALID).
	// +optional
	Status string `json:"status,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.computeEnvironmentArn"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// BatchComputeEnvironment is the Schema for managing AWS Batch compute environments.
type BatchComputeEnvironment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BatchComputeEnvironmentSpec   `json:"spec,omitempty"`
	Status BatchComputeEnvironmentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BatchComputeEnvironmentList contains a list of BatchComputeEnvironment
type BatchComputeEnvironmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BatchComputeEnvironment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BatchComputeEnvironment{}, &BatchComputeEnvironmentList{})
}
