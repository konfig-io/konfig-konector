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

// EFSPosixUser defines the POSIX user identity for an EFS access point.
type EFSPosixUser struct {
	// UID is the POSIX user ID.
	UID int64 `json:"uid"`

	// GID is the POSIX group ID.
	GID int64 `json:"gid"`

	// SecondaryGIDs are additional POSIX group IDs.
	// +optional
	SecondaryGIDs []int64 `json:"secondaryGids,omitempty"`
}

// EFSRootDirectory defines the root directory configuration for an EFS access point.
type EFSRootDirectory struct {
	// Path is the full path to the root directory. Created if it does not exist.
	// +optional
	Path string `json:"path,omitempty"`

	// CreationInfo specifies the POSIX identity used to create the root directory.
	// +optional
	CreationInfo *EFSCreationInfo `json:"creationInfo,omitempty"`
}

// EFSCreationInfo defines the POSIX identity used when creating the root directory.
type EFSCreationInfo struct {
	// OwnerUID is the POSIX user ID of the directory owner.
	OwnerUID int64 `json:"ownerUid"`

	// OwnerGID is the POSIX group ID of the directory owner.
	OwnerGID int64 `json:"ownerGid"`

	// Permissions are the POSIX permissions of the directory in octal notation.
	// +kubebuilder:validation:MinLength=3
	Permissions string `json:"permissions"`
}

// EFSAccessPointSpec defines the desired state of an EFS Access Point.
type EFSAccessPointSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// FileSystemID is the ID of the EFS file system.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="fileSystemId is immutable"
	FileSystemID string `json:"fileSystemId"`

	// PosixUser specifies the POSIX user identity for the access point.
	// +optional
	PosixUser *EFSPosixUser `json:"posixUser,omitempty"`

	// RootDirectory specifies the root directory configuration.
	// +optional
	RootDirectory *EFSRootDirectory `json:"rootDirectory,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// EFSAccessPointStatus defines the observed state of EFSAccessPoint.
type EFSAccessPointStatus struct {
	// AccessPointID is the ID of the access point.
	// +optional
	AccessPointID string `json:"accessPointId,omitempty"`

	// AccessPointARN is the ARN of the access point.
	// +optional
	AccessPointARN string `json:"accessPointArn,omitempty"`

	// LifeCycleState is the current state of the access point.
	// +optional
	LifeCycleState string `json:"lifeCycleState,omitempty"`

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
// +kubebuilder:printcolumn:name="AccessPointID",type="string",JSONPath=".status.accessPointId"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.lifeCycleState"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EFSAccessPoint is the Schema for managing EFS access points.
type EFSAccessPoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EFSAccessPointSpec   `json:"spec,omitempty"`
	Status EFSAccessPointStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EFSAccessPointList contains a list of EFSAccessPoint.
type EFSAccessPointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EFSAccessPoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EFSAccessPoint{}, &EFSAccessPointList{})
}
