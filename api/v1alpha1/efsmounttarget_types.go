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

// EFSMountTargetSpec defines the desired state of an EFS Mount Target.
type EFSMountTargetSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// FileSystemID is the ID of the EFS file system.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="fileSystemId is immutable"
	FileSystemID string `json:"fileSystemId"`

	// SubnetID is the VPC subnet ID in which to create the mount target.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="subnetId is immutable"
	SubnetID string `json:"subnetId"`

	// SecurityGroups are the security group IDs to associate with the mount target.
	// +optional
	SecurityGroups []string `json:"securityGroups,omitempty"`

	// IPAddress is a valid IPv4 address within the address range of the subnet.
	// +optional
	IPAddress string `json:"ipAddress,omitempty"`
}

// EFSMountTargetStatus defines the observed state of EFSMountTarget.
type EFSMountTargetStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// MountTargetID is the ID of the mount target.
	// +optional
	MountTargetID string `json:"mountTargetId,omitempty"`

	// LifeCycleState is the current lifecycle state of the mount target.
	// +optional
	LifeCycleState string `json:"lifeCycleState,omitempty"`

	// IPAddress is the IP address of the mount target.
	// +optional
	IPAddress string `json:"ipAddress,omitempty"`

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
// +kubebuilder:printcolumn:name="MountTargetID",type="string",JSONPath=".status.mountTargetId"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.lifeCycleState"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EFSMountTarget is the Schema for managing EFS mount targets.
type EFSMountTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EFSMountTargetSpec   `json:"spec,omitempty"`
	Status EFSMountTargetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EFSMountTargetList contains a list of EFSMountTarget.
type EFSMountTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EFSMountTarget `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EFSMountTarget{}, &EFSMountTargetList{})
}
