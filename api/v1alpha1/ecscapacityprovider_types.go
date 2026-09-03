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

// ECSManagedScaling defines the managed scaling configuration for an ECS capacity provider.
type ECSManagedScaling struct {
	// Status enables or disables managed scaling.
	// +kubebuilder:validation:Enum=ENABLED;DISABLED
	// +optional
	Status string `json:"status,omitempty"`

	// TargetCapacity is the target capacity utilization as a percentage (1-100).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	TargetCapacity *int32 `json:"targetCapacity,omitempty"`

	// MinimumScalingStepSize is the minimum number of instances by which to scale.
	// +optional
	MinimumScalingStepSize *int32 `json:"minimumScalingStepSize,omitempty"`

	// MaximumScalingStepSize is the maximum number of instances by which to scale.
	// +optional
	MaximumScalingStepSize *int32 `json:"maximumScalingStepSize,omitempty"`

	// InstanceWarmupPeriod is the period of time (in seconds) during which scale-in protection is applied.
	// +optional
	InstanceWarmupPeriod *int32 `json:"instanceWarmupPeriod,omitempty"`
}

// ECSAutoScalingGroupProvider defines the Auto Scaling group provider for an ECS capacity provider.
type ECSAutoScalingGroupProvider struct {
	// AutoScalingGroupARN is the ARN of the Auto Scaling group.
	AutoScalingGroupARN string `json:"autoScalingGroupArn"`

	// ManagedScaling defines the managed scaling configuration.
	// +optional
	ManagedScaling *ECSManagedScaling `json:"managedScaling,omitempty"`

	// ManagedTerminationProtection controls whether to enable managed termination protection.
	// +kubebuilder:validation:Enum=ENABLED;DISABLED
	// +optional
	ManagedTerminationProtection string `json:"managedTerminationProtection,omitempty"`
}

// ECSCapacityProviderSpec defines the desired state of an ECS Capacity Provider.
type ECSCapacityProviderSpec struct {
	// Name is the name of the capacity provider.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// AutoScalingGroupProvider is the Auto Scaling group provider configuration.
	AutoScalingGroupProvider ECSAutoScalingGroupProvider `json:"autoScalingGroupProvider"`

	// Tags are metadata tags for the capacity provider.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ECSCapacityProviderStatus defines the observed state of ECSCapacityProvider.
type ECSCapacityProviderStatus struct {
	// CapacityProviderARN is the ARN of the capacity provider.
	// +optional
	CapacityProviderARN string `json:"capacityProviderArn,omitempty"`

	// ProviderStatus is the current status of the capacity provider.
	// +optional
	ProviderStatus string `json:"providerStatus,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.capacityProviderArn"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.providerStatus"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ECSCapacityProvider is the Schema for managing ECS capacity providers.
type ECSCapacityProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ECSCapacityProviderSpec   `json:"spec,omitempty"`
	Status ECSCapacityProviderStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ECSCapacityProviderList contains a list of ECSCapacityProvider.
type ECSCapacityProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ECSCapacityProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ECSCapacityProvider{}, &ECSCapacityProviderList{})
}
