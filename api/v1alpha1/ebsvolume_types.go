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

// EBSVolumeSpec defines the desired state of an EBS Volume.
type EBSVolumeSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// AvailabilityZone is the AZ in which to create the volume.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="availabilityZone is immutable"
	AvailabilityZone string `json:"availabilityZone"`

	// VolumeType is the EBS volume type (gp2, gp3, io1, io2, sc1, st1, standard).
	// +kubebuilder:validation:Enum=gp2;gp3;io1;io2;sc1;st1;standard
	// +optional
	VolumeType string `json:"volumeType,omitempty"`

	// Size is the volume size in GiB.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Size int32 `json:"size,omitempty"`

	// IOPS is the requested IOPS (for io1, io2, gp3).
	// +optional
	IOPS int32 `json:"iops,omitempty"`

	// Throughput is the throughput in MiB/s (for gp3).
	// +optional
	Throughput int32 `json:"throughput,omitempty"`

	// Encrypted enables EBS encryption.
	// +optional
	Encrypted bool `json:"encrypted,omitempty"`

	// KMSKeyID is the KMS key ID/ARN for encryption.
	// +optional
	KMSKeyID string `json:"kmsKeyId,omitempty"`

	// SnapshotID creates the volume from an existing snapshot.
	// +optional
	SnapshotID string `json:"snapshotId,omitempty"`

	// MultiAttachEnabled enables Multi-Attach for io1/io2 volumes.
	// +optional
	MultiAttachEnabled bool `json:"multiAttachEnabled,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// EBSVolumeStatus defines the observed state of EBSVolume.
type EBSVolumeStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// VolumeID is the EBS volume ID.
	// +optional
	VolumeID string `json:"volumeId,omitempty"`

	// State is the current state of the volume.
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
// +kubebuilder:printcolumn:name="VolumeID",type="string",JSONPath=".status.volumeId"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EBSVolume is the Schema for managing EBS Volumes.
type EBSVolume struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EBSVolumeSpec   `json:"spec,omitempty"`
	Status EBSVolumeStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EBSVolumeList contains a list of EBSVolume.
type EBSVolumeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EBSVolume `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EBSVolume{}, &EBSVolumeList{})
}
