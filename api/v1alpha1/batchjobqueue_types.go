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

// BatchComputeEnvironmentRef references either a managed
// BatchComputeEnvironment CR or a direct compute environment ARN.
type BatchComputeEnvironmentRef struct {
	// Name of a BatchComputeEnvironment CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ARN is a direct AWS compute environment ARN.
	// If set, Name is ignored.
	// +optional
	ARN string `json:"arn,omitempty"`
}

// BatchComputeEnvironmentOrder maps a compute environment to a job queue
// with an order.
type BatchComputeEnvironmentOrder struct {
	// ComputeEnvironmentRef references the compute environment.
	ComputeEnvironmentRef BatchComputeEnvironmentRef `json:"computeEnvironmentRef"`

	// Order in which compute environments are tried (ascending).
	Order int32 `json:"order"`
}

// BatchJobQueueSpec defines the desired state of a Batch job queue.
type BatchJobQueueSpec struct {
	// Name of the job queue. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// State of the job queue.
	// +kubebuilder:validation:Enum=ENABLED;DISABLED
	// +optional
	State string `json:"state,omitempty"`

	// Priority of the job queue. Higher values are evaluated first.
	Priority int32 `json:"priority"`

	// ComputeEnvironmentOrder maps compute environments to this queue.
	// +kubebuilder:validation:MinItems=1
	ComputeEnvironmentOrder []BatchComputeEnvironmentOrder `json:"computeEnvironmentOrder"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// BatchJobQueueStatus defines the observed state of BatchJobQueue.
type BatchJobQueueStatus struct {
	// JobQueueARN is the ARN of the job queue.
	// +optional
	JobQueueARN string `json:"jobQueueArn,omitempty"`

	// Status is the current job queue status (e.g. VALID).
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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.jobQueueArn"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// BatchJobQueue is the Schema for managing AWS Batch job queues.
type BatchJobQueue struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BatchJobQueueSpec   `json:"spec,omitempty"`
	Status BatchJobQueueStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BatchJobQueueList contains a list of BatchJobQueue
type BatchJobQueueList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BatchJobQueue `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BatchJobQueue{}, &BatchJobQueueList{})
}
