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

// AppRunnerAutoScalingSpec defines the desired state of an App Runner
// auto scaling configuration. Configurations are immutable versioned
// resources in AWS: spec changes after creation are not supported.
type AppRunnerAutoScalingSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name of the auto scaling configuration. Immutable after creation.
	// +kubebuilder:validation:MinLength=4
	// +kubebuilder:validation:MaxLength=32
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// MaxConcurrency is the maximum concurrent requests per instance.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxConcurrency int32 `json:"maxConcurrency,omitempty"`

	// MaxSize is the maximum number of instances.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxSize int32 `json:"maxSize,omitempty"`

	// MinSize is the minimum number of provisioned instances.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MinSize int32 `json:"minSize,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// AppRunnerAutoScalingStatus defines the observed state of AppRunnerAutoScaling.
type AppRunnerAutoScalingStatus struct {
	// AutoScalingConfigurationARN is the ARN of the configuration revision.
	// +optional
	AutoScalingConfigurationARN string `json:"autoScalingConfigurationArn,omitempty"`

	// Revision is the created configuration revision.
	// +optional
	Revision int32 `json:"revision,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.autoScalingConfigurationArn"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AppRunnerAutoScaling is the Schema for managing AWS App Runner auto
// scaling configurations.
type AppRunnerAutoScaling struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppRunnerAutoScalingSpec   `json:"spec,omitempty"`
	Status AppRunnerAutoScalingStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AppRunnerAutoScalingList contains a list of AppRunnerAutoScaling
type AppRunnerAutoScalingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AppRunnerAutoScaling `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AppRunnerAutoScaling{}, &AppRunnerAutoScalingList{})
}
