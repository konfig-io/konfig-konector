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

// LatticeTargetGroupRef references a managed LatticeTargetGroup CR or a
// direct target group ID/ARN.
type LatticeTargetGroupRef struct {
	// Name of a LatticeTargetGroup CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ID is a direct AWS target group ID or ARN. If set, Name is ignored.
	// +optional
	ID string `json:"id,omitempty"`
}

// LatticeForwardTarget is one weighted target group in a forward action.
type LatticeForwardTarget struct {
	// TargetGroupRef references the target group to forward to.
	TargetGroupRef LatticeTargetGroupRef `json:"targetGroupRef"`
	// Weight determines how requests are distributed among target groups.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=999
	// +optional
	Weight int32 `json:"weight,omitempty"`
}

// LatticeFixedResponse returns a fixed HTTP status code.
type LatticeFixedResponse struct {
	// StatusCode is the HTTP response code to return.
	// +kubebuilder:validation:Minimum=100
	// +kubebuilder:validation:Maximum=599
	StatusCode int32 `json:"statusCode"`
}

// LatticeDefaultAction is the default action for a listener. Exactly one of
// Forward or FixedResponse must be set.
type LatticeDefaultAction struct {
	// Forward routes requests to one or more target groups.
	// +optional
	Forward []LatticeForwardTarget `json:"forward,omitempty"`
	// FixedResponse returns a fixed HTTP status code.
	// +optional
	FixedResponse *LatticeFixedResponse `json:"fixedResponse,omitempty"`
}

// LatticeListenerSpec defines the desired state of a VPC Lattice listener.
type LatticeListenerSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ServiceRef references the service the listener belongs to.
	ServiceRef LatticeServiceRef `json:"serviceRef"`

	// Name of the listener. Immutable after creation.
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Protocol of the listener.
	// +kubebuilder:validation:Enum=HTTP;HTTPS
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="protocol is immutable"
	Protocol string `json:"protocol"`

	// Port on which the listener accepts traffic. Defaults to the protocol's
	// standard port (80 for HTTP, 443 for HTTPS).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`

	// DefaultAction taken when no listener rule matches.
	DefaultAction LatticeDefaultAction `json:"defaultAction"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// LatticeListenerStatus defines the observed state of LatticeListener.
type LatticeListenerStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ARN is the Amazon Resource Name of the listener.
	// +optional
	ARN string `json:"arn,omitempty"`

	// ID is the unique identifier of the listener.
	// +optional
	ID string `json:"id,omitempty"`

	// ServiceID is the identifier of the owning service (needed for delete).
	// +optional
	ServiceID string `json:"serviceId,omitempty"`

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
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.id"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// LatticeListener is the Schema for managing VPC Lattice listeners.
type LatticeListener struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LatticeListenerSpec   `json:"spec,omitempty"`
	Status LatticeListenerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// LatticeListenerList contains a list of LatticeListener
type LatticeListenerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LatticeListener `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LatticeListener{}, &LatticeListenerList{})
}
