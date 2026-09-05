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

// LoadBalancerSpec defines the desired state of an ELBv2 Load Balancer.
type LoadBalancerSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the name of the load balancer. Immutable.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=32
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Type is the load balancer type.
	// +kubebuilder:validation:Enum=application;network;gateway
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="type is immutable"
	Type string `json:"type"`

	// Scheme determines if the load balancer is internet-facing or internal.
	// +kubebuilder:validation:Enum=internet-facing;internal
	// +optional
	Scheme string `json:"scheme,omitempty"`

	// SubnetRefs are the subnets in which to place the load balancer.
	// +kubebuilder:validation:MinItems=2
	SubnetRefs []SubnetRef `json:"subnetRefs"`

	// SecurityGroupRefs are the security groups to attach (ALB only).
	// +optional
	SecurityGroupRefs []SecurityGroupRef `json:"securityGroupRefs,omitempty"`

	// DeletionProtection prevents accidental deletion.
	// +optional
	DeletionProtection bool `json:"deletionProtection,omitempty"`

	// IdleTimeout is the idle timeout in seconds for ALBs.
	// +optional
	IdleTimeout int32 `json:"idleTimeout,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// LoadBalancerStatus defines the observed state of LoadBalancer.
type LoadBalancerStatus struct {
	// ARN is the ARN of the load balancer.
	// +optional
	ARN string `json:"arn,omitempty"`

	// DNSName is the DNS name of the load balancer.
	// +optional
	DNSName string `json:"dnsName,omitempty"`

	// State is the current state of the load balancer.
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
// +kubebuilder:printcolumn:name="DNS-Name",type="string",JSONPath=".status.dnsName"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// LoadBalancer is the Schema for managing ELBv2 Load Balancers.
type LoadBalancer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LoadBalancerSpec   `json:"spec,omitempty"`
	Status LoadBalancerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// LoadBalancerList contains a list of LoadBalancer
type LoadBalancerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LoadBalancer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LoadBalancer{}, &LoadBalancerList{})
}
