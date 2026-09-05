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

// ElasticIPSpec defines the desired state of an Elastic IP address.
type ElasticIPSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Domain is the domain for the Elastic IP. Defaults to "vpc".
	// +kubebuilder:validation:Enum=vpc
	// +optional
	Domain string `json:"domain,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ElasticIPStatus defines the observed state of ElasticIP.
type ElasticIPStatus struct {
	// AllocationID is the AWS allocation ID of the Elastic IP.
	// +optional
	AllocationID string `json:"allocationId,omitempty"`

	// PublicIP is the public IP address.
	// +optional
	PublicIP string `json:"publicIp,omitempty"`

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
// +kubebuilder:printcolumn:name="Allocation-ID",type="string",JSONPath=".status.allocationId"
// +kubebuilder:printcolumn:name="Public-IP",type="string",JSONPath=".status.publicIp"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ElasticIP is the Schema for managing AWS Elastic IP addresses.
type ElasticIP struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ElasticIPSpec   `json:"spec,omitempty"`
	Status ElasticIPStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ElasticIPList contains a list of ElasticIP
type ElasticIPList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ElasticIP `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ElasticIP{}, &ElasticIPList{})
}
