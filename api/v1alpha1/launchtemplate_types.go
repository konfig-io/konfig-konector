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

// BlockDeviceMapping defines an EBS block device mapping for a launch template.
type BlockDeviceMapping struct {
	// DeviceName is the device name (e.g. /dev/xvda).
	DeviceName string `json:"deviceName"`

	// VolumeSize is the size of the EBS volume in GiB.
	// +optional
	VolumeSize int32 `json:"volumeSize,omitempty"`

	// VolumeType is the EBS volume type (gp2, gp3, io1, etc.).
	// +optional
	VolumeType string `json:"volumeType,omitempty"`

	// Encrypted enables EBS encryption.
	// +optional
	Encrypted bool `json:"encrypted,omitempty"`
}

// LaunchTemplateSpec defines the desired state of an EC2 Launch Template.
type LaunchTemplateSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// LaunchTemplateName is the name of the launch template. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="launchTemplateName is immutable"
	LaunchTemplateName string `json:"launchTemplateName"`

	// ImageID is the AMI ID.
	// +kubebuilder:validation:MinLength=1
	ImageID string `json:"imageId"`

	// InstanceType is the EC2 instance type (e.g. t3.micro).
	// +optional
	InstanceType string `json:"instanceType,omitempty"`

	// KeyName is the name of the EC2 key pair.
	// +optional
	KeyName string `json:"keyName,omitempty"`

	// SecurityGroupRefs is the list of security group references.
	// +optional
	SecurityGroupRefs []SecurityGroupRef `json:"securityGroupRefs,omitempty"`

	// UserData is base64-encoded user data to pass to the instances.
	// +optional
	UserData string `json:"userData,omitempty"`

	// IAMInstanceProfile is the name or ARN of an IAM instance profile.
	// +optional
	IAMInstanceProfile string `json:"iamInstanceProfile,omitempty"`

	// BlockDeviceMappings is the list of block device mappings.
	// +optional
	BlockDeviceMappings []BlockDeviceMapping `json:"blockDeviceMappings,omitempty"`

	// Tags are AWS resource tags to apply to the launch template resource.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// LaunchTemplateStatus defines the observed state of LaunchTemplate.
type LaunchTemplateStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// LaunchTemplateID is the AWS Launch Template identifier.
	// +optional
	LaunchTemplateID string `json:"launchTemplateId,omitempty"`

	// LatestVersionNumber is the most recently created version number.
	// +optional
	LatestVersionNumber int64 `json:"latestVersionNumber,omitempty"`

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
// +kubebuilder:printcolumn:name="Template-ID",type="string",JSONPath=".status.launchTemplateId"
// +kubebuilder:printcolumn:name="Version",type="integer",JSONPath=".status.latestVersionNumber"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// LaunchTemplate is the Schema for managing EC2 Launch Templates.
type LaunchTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LaunchTemplateSpec   `json:"spec,omitempty"`
	Status LaunchTemplateStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// LaunchTemplateList contains a list of LaunchTemplate
type LaunchTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LaunchTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LaunchTemplate{}, &LaunchTemplateList{})
}
