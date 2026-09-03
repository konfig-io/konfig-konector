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

// SpotFleetLaunchSpec defines a single launch specification for Spot Fleet.
type SpotFleetLaunchSpec struct {
	// ImageID is the AMI ID for instances in this spec.
	// +kubebuilder:validation:MinLength=1
	ImageID string `json:"imageId"`

	// InstanceType is the EC2 instance type.
	// +kubebuilder:validation:MinLength=1
	InstanceType string `json:"instanceType"`

	// SubnetID is the subnet to launch instances in.
	// +optional
	SubnetID string `json:"subnetId,omitempty"`

	// KeyName is the EC2 key pair name.
	// +optional
	KeyName string `json:"keyName,omitempty"`

	// SecurityGroupIDs are the security groups to attach.
	// +optional
	SecurityGroupIDs []string `json:"securityGroupIds,omitempty"`

	// SpotPrice is the maximum spot price for this spec.
	// +optional
	SpotPrice string `json:"spotPrice,omitempty"`

	// WeightedCapacity is the number of capacity units this spec provides.
	// +optional
	WeightedCapacity float64 `json:"weightedCapacity,omitempty"`
}

// SpotFleetSpec defines the desired state of a Spot Fleet Request.
type SpotFleetSpec struct {
	// IAMFleetRole is the ARN of the IAM role for the Spot Fleet.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="iamFleetRole is immutable"
	IAMFleetRole string `json:"iamFleetRole"`

	// TargetCapacity is the number of units to request.
	// +kubebuilder:validation:Minimum=1
	TargetCapacity int32 `json:"targetCapacity"`

	// LaunchSpecs defines the launch specifications.
	// +kubebuilder:validation:MinItems=1
	LaunchSpecs []SpotFleetLaunchSpec `json:"launchSpecs"`

	// AllocationStrategy controls how Spot Fleet selects instances.
	// +kubebuilder:validation:Enum=lowestPrice;diversified;capacityOptimized;priceCapacityOptimized
	// +optional
	AllocationStrategy string `json:"allocationStrategy,omitempty"`

	// SpotPrice is the global maximum spot price bid.
	// +optional
	SpotPrice string `json:"spotPrice,omitempty"`

	// TerminateInstancesWithExpiration terminates all instances when the request expires.
	// +optional
	TerminateInstancesWithExpiration bool `json:"terminateInstancesWithExpiration,omitempty"`

	// ValidUntil is an optional expiry time for the request (RFC3339).
	// +optional
	ValidUntil *metav1.Time `json:"validUntil,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// SpotFleetStatus defines the observed state of SpotFleet.
type SpotFleetStatus struct {
	// SpotFleetRequestID is the ID of the Spot Fleet request.
	// +optional
	SpotFleetRequestID string `json:"spotFleetRequestId,omitempty"`

	// RequestState is the current state of the Spot Fleet request.
	// +optional
	RequestState string `json:"requestState,omitempty"`

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
// +kubebuilder:printcolumn:name="RequestID",type="string",JSONPath=".status.spotFleetRequestId"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.requestState"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SpotFleet is the Schema for managing EC2 Spot Fleet requests.
type SpotFleet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SpotFleetSpec   `json:"spec,omitempty"`
	Status SpotFleetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SpotFleetList contains a list of SpotFleet.
type SpotFleetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SpotFleet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SpotFleet{}, &SpotFleetList{})
}
