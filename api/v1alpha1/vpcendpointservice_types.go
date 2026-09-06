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

// VPCEndpointServiceSpec defines a PrivateLink endpoint service (the provider
// side). Consumers in other accounts create VPCEndpoints against
// status.serviceName; with AutoAcceptConnections the controller accepts their
// pending connections every reconcile.
type VPCEndpointServiceSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// NetworkLoadBalancerRefs front the service (LoadBalancer CRs or ARNs).
	// +optional
	NetworkLoadBalancerRefs []LoadBalancerRef `json:"networkLoadBalancerRefs,omitempty"`

	// GatewayLoadBalancerArns front the service instead of NLBs.
	// +optional
	GatewayLoadBalancerArns []string `json:"gatewayLoadBalancerArns,omitempty"`

	// AcceptanceRequired makes consumer connections wait for acceptance.
	// +optional
	AcceptanceRequired bool `json:"acceptanceRequired,omitempty"`

	// AutoAcceptConnections accepts every pendingAcceptance connection from an
	// allowed principal on each reconcile.
	// +optional
	AutoAcceptConnections bool `json:"autoAcceptConnections,omitempty"`

	// AllowedPrincipals are IAM ARNs (accounts, roles, users, "*") permitted to
	// discover and connect to the service.
	// +optional
	AllowedPrincipals []string `json:"allowedPrincipals,omitempty"`

	// PrivateDNSName is an optional private DNS name for the service.
	// +optional
	PrivateDNSName string `json:"privateDnsName,omitempty"`

	// SupportedIPAddressTypes: ipv4, ipv6.
	// +optional
	SupportedIPAddressTypes []string `json:"supportedIpAddressTypes,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// VPCEndpointServiceStatus defines the observed state of VPCEndpointService.
type VPCEndpointServiceStatus struct {
	// ServiceID is the endpoint service configuration ID (vpce-svc-...).
	// +optional
	ServiceID string `json:"serviceId,omitempty"`
	// ServiceName is what consumers put in VPCEndpoint.spec.serviceName.
	// +optional
	ServiceName string `json:"serviceName,omitempty"`
	// State of the service configuration.
	// +optional
	State string `json:"state,omitempty"`
	// AvailabilityZones the service is reachable in.
	// +optional
	AvailabilityZones []string `json:"availabilityZones,omitempty"`
	// PrivateDNSNameVerificationState for the private DNS name, if set.
	// +optional
	PrivateDNSNameVerificationState string `json:"privateDnsNameVerificationState,omitempty"`
	// PrivateDNSVerificationRecord is the TXT record (name/value) proving DNS ownership.
	// +optional
	PrivateDNSVerificationRecord string `json:"privateDnsVerificationRecord,omitempty"`
	// PendingConnections counts consumer endpoints awaiting acceptance.
	// +optional
	PendingConnections int32 `json:"pendingConnections,omitempty"`
	// Conditions describe the current state of the resource.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ServiceName",type="string",JSONPath=".status.serviceName"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Pending",type="integer",JSONPath=".status.pendingConnections"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// VPCEndpointService is a PrivateLink endpoint service configuration.
type VPCEndpointService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VPCEndpointServiceSpec   `json:"spec,omitempty"`
	Status VPCEndpointServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VPCEndpointServiceList contains a list of VPCEndpointService.
type VPCEndpointServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VPCEndpointService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VPCEndpointService{}, &VPCEndpointServiceList{})
}
