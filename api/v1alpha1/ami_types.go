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

// AMIBlockDeviceMapping defines an EBS block device mapping for an AMI.
type AMIBlockDeviceMapping struct {
	// DeviceName is the device name (e.g. /dev/xvda).
	// +kubebuilder:validation:MinLength=1
	DeviceName string `json:"deviceName"`

	// SnapshotID is the snapshot to use for this device.
	// +optional
	SnapshotID string `json:"snapshotId,omitempty"`

	// VolumeSize is the size of the volume in GiB.
	// +optional
	VolumeSize int32 `json:"volumeSize,omitempty"`

	// VolumeType is the EBS volume type.
	// +optional
	VolumeType string `json:"volumeType,omitempty"`

	// DeleteOnTermination controls whether to delete on instance termination.
	// +optional
	DeleteOnTermination bool `json:"deleteOnTermination,omitempty"`

	// Encrypted enables encryption for this volume.
	// +optional
	Encrypted bool `json:"encrypted,omitempty"`
}

// AMISpec defines the desired state of an AMI registration.
type AMISpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the AMI name. Must be unique within the account/region.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Description is an optional description.
	// +optional
	Description string `json:"description,omitempty"`

	// Architecture is the CPU architecture (x86_64 or arm64).
	// +kubebuilder:validation:Enum=x86_64;arm64;i386
	// +optional
	Architecture string `json:"architecture,omitempty"`

	// RootDeviceName is the root device name (e.g. /dev/xvda).
	// +optional
	RootDeviceName string `json:"rootDeviceName,omitempty"`

	// VirtualizationType is hvm or paravirtual.
	// +kubebuilder:validation:Enum=hvm;paravirtual
	// +optional
	VirtualizationType string `json:"virtualizationType,omitempty"`

	// KernelID is an optional kernel ID (for paravirtual).
	// +optional
	KernelID string `json:"kernelId,omitempty"`

	// RamdiskID is an optional ramdisk ID.
	// +optional
	RamdiskID string `json:"ramdiskId,omitempty"`

	// BlockDeviceMappings defines the EBS volumes and snapshots to attach.
	// +optional
	BlockDeviceMappings []AMIBlockDeviceMapping `json:"blockDeviceMappings,omitempty"`

	// SRIOVNetSupport enables SR-IOV networking (set to "simple").
	// +optional
	SRIOVNetSupport string `json:"sriovNetSupport,omitempty"`

	// ENASupport enables enhanced networking with ENA.
	// +optional
	ENASupport bool `json:"enaSupport,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// AMIStatus defines the observed state of AMI.
type AMIStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ImageID is the AMI ID.
	// +optional
	ImageID string `json:"imageId,omitempty"`

	// State is the current state of the AMI (pending, available, failed, etc.).
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
// +kubebuilder:printcolumn:name="ImageID",type="string",JSONPath=".status.imageId"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AMI is the Schema for managing EC2 Amazon Machine Images.
type AMI struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AMISpec   `json:"spec,omitempty"`
	Status AMIStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AMIList contains a list of AMI.
type AMIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AMI `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AMI{}, &AMIList{})
}
