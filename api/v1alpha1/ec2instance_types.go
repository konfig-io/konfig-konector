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

// EC2InstanceSpec defines the desired state of an EC2 Instance.
type EC2InstanceSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ImageID is the AMI ID. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="imageId is immutable"
	ImageID string `json:"imageId"`

	// InstanceType is the EC2 instance type (e.g. t3.micro).
	// +kubebuilder:validation:MinLength=1
	InstanceType string `json:"instanceType"`

	// KeyName is the name of the key pair to associate with the instance.
	// +optional
	KeyName string `json:"keyName,omitempty"`

	// SubnetRef is the name of a Subnet CR in the same namespace.
	// +optional
	SubnetRef string `json:"subnetRef,omitempty"`

	// SecurityGroupRefs is the list of SecurityGroup CRs (or direct IDs) to attach.
	// +optional
	SecurityGroupRefs []SecurityGroupRef `json:"securityGroupRefs,omitempty"`

	// IAMInstanceProfile is the name or ARN of the IAM instance profile.
	// +optional
	IAMInstanceProfile string `json:"iamInstanceProfile,omitempty"`

	// UserData is the base64-encoded user data script.
	// +optional
	UserData string `json:"userData,omitempty"`

	// AssociatePublicIPAddress controls whether a public IP is assigned.
	// +optional
	AssociatePublicIPAddress bool `json:"associatePublicIpAddress,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// EC2InstanceStatus defines the observed state of EC2Instance.
type EC2InstanceStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// InstanceID is the EC2 instance ID.
	// +optional
	InstanceID string `json:"instanceId,omitempty"`

	// PrivateIP is the private IPv4 address.
	// +optional
	PrivateIP string `json:"privateIp,omitempty"`

	// PublicIP is the public IPv4 address (if assigned).
	// +optional
	PublicIP string `json:"publicIp,omitempty"`

	// State is the current instance state (pending, running, stopped, terminated).
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
// +kubebuilder:printcolumn:name="Instance-ID",type="string",JSONPath=".status.instanceId"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Private-IP",type="string",JSONPath=".status.privateIp"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EC2Instance is the Schema for managing EC2 Instances.
type EC2Instance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EC2InstanceSpec   `json:"spec,omitempty"`
	Status EC2InstanceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EC2InstanceList contains a list of EC2Instance
type EC2InstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EC2Instance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EC2Instance{}, &EC2InstanceList{})
}
