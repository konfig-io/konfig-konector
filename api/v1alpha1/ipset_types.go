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

// IPSetSpec defines the desired state of a WAFv2 IPSet.
type IPSetSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the name of the IP set. Immutable.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Scope is CLOUDFRONT or REGIONAL.
	// +kubebuilder:validation:Enum=CLOUDFRONT;REGIONAL
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="scope is immutable"
	Scope string `json:"scope"`

	// IPAddressVersion is IPV4 or IPV6.
	// +kubebuilder:validation:Enum=IPV4;IPV6
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="ipAddressVersion is immutable"
	IPAddressVersion string `json:"ipAddressVersion"`

	// Addresses are the CIDR blocks to include in the IP set.
	Addresses []string `json:"addresses"`

	// Description is a human-readable description.
	// +optional
	Description string `json:"description,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// IPSetStatus defines the observed state of IPSet.
type IPSetStatus struct {
	// ID is the IPSet ID.
	// +optional
	ID string `json:"id,omitempty"`

	// ARN is the ARN of the IP set.
	// +optional
	ARN string `json:"arn,omitempty"`

	// LockToken is the WAFv2 lock token required for updates/deletes.
	// +optional
	LockToken string `json:"lockToken,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.arn"
// +kubebuilder:printcolumn:name="Scope",type="string",JSONPath=".spec.scope"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// IPSet is the Schema for managing WAFv2 IPSets.
type IPSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IPSetSpec   `json:"spec,omitempty"`
	Status IPSetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IPSetList contains a list of IPSet
type IPSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IPSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IPSet{}, &IPSetList{})
}
