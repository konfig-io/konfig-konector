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

// SubnetSpec defines the desired state of an AWS Subnet.
type SubnetSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// VPCRef references the VPC this subnet belongs to.
	VPCRef VPCResourceRef `json:"vpcRef"`

	// CIDRBlock is the IPv4 CIDR for the subnet (e.g. "10.0.1.0/24").
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^\d+\.\d+\.\d+\.\d+/\d+$`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="cidrBlock is immutable"
	CIDRBlock string `json:"cidrBlock"`

	// AvailabilityZone is the AZ in which to create the subnet (e.g. "us-east-1a").
	// +optional
	AvailabilityZone string `json:"availabilityZone,omitempty"`

	// MapPublicIPOnLaunch automatically assigns a public IP to instances launched in this subnet.
	// +optional
	MapPublicIPOnLaunch bool `json:"mapPublicIpOnLaunch,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// SubnetStatus defines the observed state of Subnet.
type SubnetStatus struct {
	// SubnetID is the AWS Subnet identifier.
	// +optional
	SubnetID string `json:"subnetId,omitempty"`

	// AvailableIPAddressCount is the number of available IP addresses.
	// +optional
	AvailableIPAddressCount int32 `json:"availableIpAddressCount,omitempty"`

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
// +kubebuilder:printcolumn:name="Subnet-ID",type="string",JSONPath=".status.subnetId"
// +kubebuilder:printcolumn:name="CIDR",type="string",JSONPath=".spec.cidrBlock"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Subnet is the Schema for managing AWS Subnets.
type Subnet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SubnetSpec   `json:"spec,omitempty"`
	Status SubnetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SubnetList contains a list of Subnet
type SubnetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Subnet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Subnet{}, &SubnetList{})
}
