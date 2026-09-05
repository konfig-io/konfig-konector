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

// VPCRef references an AWS VPC for private hosted zones.
type VPCRef struct {
	// ID is the VPC ID.
	ID string `json:"id"`
	// Region is the AWS region of the VPC.
	Region string `json:"region"`
}

// HostedZoneSpec defines the desired state of a Route53 Hosted Zone.
type HostedZoneSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the DNS zone name (e.g. "example.com.").
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Comment is a human-readable description for the hosted zone.
	// +optional
	Comment string `json:"comment,omitempty"`

	// Private indicates this is a private hosted zone (requires VPCRef).
	// +optional
	Private bool `json:"private,omitempty"`

	// VPCRef is required when Private is true.
	// +optional
	VPCRef *VPCRef `json:"vpcRef,omitempty"`

	// DelegationSetID is the reusable delegation set ID to assign to this zone.
	// +optional
	DelegationSetID string `json:"delegationSetId,omitempty"`

	// Tags are AWS resource tags applied to the hosted zone.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// HostedZoneStatus defines the observed state of HostedZone.
type HostedZoneStatus struct {
	// HostedZoneID is the Route53 hosted zone identifier (e.g. "Z1234567890").
	// +optional
	HostedZoneID string `json:"hostedZoneId,omitempty"`

	// NameServers are the authoritative name servers for the zone.
	// +optional
	NameServers []string `json:"nameServers,omitempty"`

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
// +kubebuilder:printcolumn:name="ZoneID",type="string",JSONPath=".status.hostedZoneId"
// +kubebuilder:printcolumn:name="Name",type="string",JSONPath=".spec.name"
// +kubebuilder:printcolumn:name="Private",type="boolean",JSONPath=".spec.private"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// HostedZone is the Schema for managing AWS Route53 Hosted Zones.
type HostedZone struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HostedZoneSpec   `json:"spec,omitempty"`
	Status HostedZoneStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// HostedZoneList contains a list of HostedZone
type HostedZoneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HostedZone `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HostedZone{}, &HostedZoneList{})
}
