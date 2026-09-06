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

// LoadBalancerRef references either a managed LoadBalancer CR or a direct ARN.
type LoadBalancerRef struct {
	// Name of a LoadBalancer CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ARN is a direct AWS load balancer ARN.
	// +optional
	ARN string `json:"arn,omitempty"`
}

// TargetGroupRef references either a managed TargetGroup CR or a direct ARN.
type TargetGroupRef struct {
	// Name of a TargetGroup CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ARN is a direct AWS target group ARN.
	// +optional
	ARN string `json:"arn,omitempty"`
}

// ListenerDefaultAction defines the default action for a listener.
type ListenerDefaultAction struct {
	// Type is the action type.
	// +kubebuilder:validation:Enum=forward;redirect;fixed-response
	Type string `json:"type"`

	// TargetGroupRef is the target group for a forward action.
	// +optional
	TargetGroupRef *TargetGroupRef `json:"targetGroupRef,omitempty"`

	// RedirectConfig configures a redirect action.
	// +optional
	RedirectConfig *ListenerRedirectConfig `json:"redirectConfig,omitempty"`

	// FixedResponseConfig configures a fixed response.
	// +optional
	FixedResponseConfig *ListenerFixedResponseConfig `json:"fixedResponseConfig,omitempty"`
}

// ListenerRedirectConfig defines the redirect configuration.
type ListenerRedirectConfig struct {
	// StatusCode is the HTTP redirect code.
	// +kubebuilder:validation:Enum=HTTP_301;HTTP_302
	StatusCode string `json:"statusCode"`
	// Host is the redirect host.
	// +optional
	Host string `json:"host,omitempty"`
	// Path is the redirect path.
	// +optional
	Path string `json:"path,omitempty"`
	// Port is the redirect port.
	// +optional
	Port string `json:"port,omitempty"`
	// Protocol is the redirect protocol.
	// +optional
	Protocol string `json:"protocol,omitempty"`
}

// ListenerFixedResponseConfig defines fixed-response action config.
type ListenerFixedResponseConfig struct {
	// StatusCode is the HTTP status code.
	StatusCode string `json:"statusCode"`
	// ContentType is the response content type.
	// +optional
	ContentType string `json:"contentType,omitempty"`
	// MessageBody is the response body.
	// +optional
	MessageBody string `json:"messageBody,omitempty"`
}

// ListenerSpec defines the desired state of a Listener.
type ListenerSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// LoadBalancerRef references the load balancer.
	LoadBalancerRef LoadBalancerRef `json:"loadBalancerRef"`

	// Protocol is the listener protocol.
	// +kubebuilder:validation:Enum=HTTP;HTTPS;TCP;TLS;UDP;TCP_UDP;GENEVE
	Protocol string `json:"protocol"`

	// Port is the listener port.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// DefaultActions are the default actions for the listener.
	// +kubebuilder:validation:MinItems=1
	DefaultActions []ListenerDefaultAction `json:"defaultActions"`

	// CertificateARNs are the TLS certificate ARNs (for HTTPS/TLS).
	// +optional
	CertificateARNs []string `json:"certificateArns,omitempty"`

	// SSLPolicy is the security policy for HTTPS/TLS.
	// +optional
	SSLPolicy string `json:"sslPolicy,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ListenerStatus defines the observed state of Listener.
type ListenerStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ARN is the ARN of the listener.
	// +optional
	ARN string `json:"arn,omitempty"`

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
// +kubebuilder:printcolumn:name="Protocol",type="string",JSONPath=".spec.protocol"
// +kubebuilder:printcolumn:name="Port",type="integer",JSONPath=".spec.port"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Listener is the Schema for managing ELBv2 Listeners.
type Listener struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ListenerSpec   `json:"spec,omitempty"`
	Status ListenerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ListenerList contains a list of Listener
type ListenerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Listener `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Listener{}, &ListenerList{})
}
