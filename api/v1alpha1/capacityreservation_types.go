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

// CapacityReservationSpec defines the desired state of an EC2 capacity reservation.
type CapacityReservationSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// InstanceType for which to reserve capacity. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="instanceType is immutable"
	InstanceType string `json:"instanceType"`

	// InstancePlatform is the operating system type for the reserved capacity
	// (e.g. Linux/UNIX, Windows). Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="instancePlatform is immutable"
	InstancePlatform string `json:"instancePlatform"`

	// AvailabilityZone in which to create the reservation. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="availabilityZone is immutable"
	AvailabilityZone string `json:"availabilityZone"`

	// InstanceCount is the number of instances to reserve capacity for.
	// +kubebuilder:validation:Minimum=1
	InstanceCount int32 `json:"instanceCount"`

	// Tenancy of the reservation.
	// +kubebuilder:validation:Enum=default;dedicated
	// +optional
	Tenancy string `json:"tenancy,omitempty"`

	// EndDateType indicates how the reservation ends.
	// +kubebuilder:validation:Enum=unlimited;limited
	// +optional
	EndDateType string `json:"endDateType,omitempty"`

	// EndDate is when the reservation expires (required when endDateType is
	// limited).
	// +optional
	EndDate *metav1.Time `json:"endDate,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// CapacityReservationStatus defines the observed state of CapacityReservation.
type CapacityReservationStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// CapacityReservationID is the AWS capacity reservation identifier.
	// +optional
	CapacityReservationID string `json:"capacityReservationId,omitempty"`

	// ARN is the Amazon Resource Name of the reservation.
	// +optional
	ARN string `json:"arn,omitempty"`

	// State is the lifecycle state reported by AWS.
	// +optional
	State string `json:"state,omitempty"`

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
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.capacityReservationId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CapacityReservation is the Schema for managing EC2 capacity reservations.
type CapacityReservation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CapacityReservationSpec   `json:"spec,omitempty"`
	Status CapacityReservationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CapacityReservationList contains a list of CapacityReservation
type CapacityReservationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CapacityReservation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CapacityReservation{}, &CapacityReservationList{})
}
