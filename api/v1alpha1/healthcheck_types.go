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

// HealthCheckSpec defines the desired state of a Route53 Health Check.
type HealthCheckSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Type is the type of health check.
	// +kubebuilder:validation:Enum=HTTP;HTTPS;HTTP_STR_MATCH;HTTPS_STR_MATCH;TCP;CALCULATED;CLOUDWATCH_METRIC;RECOVERY_CONTROL
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="type is immutable"
	Type string `json:"type"`

	// IPAddress is the IPv4 or IPv6 address of the endpoint.
	// +optional
	IPAddress string `json:"ipAddress,omitempty"`

	// FQDN is the fully qualified domain name of the endpoint.
	// +optional
	FQDN string `json:"fqdn,omitempty"`

	// Port is the port on the endpoint to connect to.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`

	// ResourcePath is the path that Route53 requests when performing health checks (HTTP/HTTPS).
	// +optional
	ResourcePath string `json:"resourcePath,omitempty"`

	// SearchString is the string to search for in HTTP/HTTPS responses.
	// +optional
	SearchString string `json:"searchString,omitempty"`

	// RequestInterval is the frequency of health checks in seconds (10 or 30).
	// +kubebuilder:validation:Enum=10;30
	// +optional
	RequestInterval int32 `json:"requestInterval,omitempty"`

	// FailureThreshold is the number of consecutive failures before marking unhealthy (1–10).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	// +optional
	FailureThreshold int32 `json:"failureThreshold,omitempty"`

	// Tags are AWS resource tags applied to the health check.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// HealthCheckStatus defines the observed state of HealthCheck.
type HealthCheckStatus struct {
	// HealthCheckID is the Route53 health check identifier.
	// +optional
	HealthCheckID string `json:"healthCheckId,omitempty"`

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
// +kubebuilder:printcolumn:name="HealthCheckID",type="string",JSONPath=".status.healthCheckId"
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// HealthCheck is the Schema for managing AWS Route53 Health Checks.
type HealthCheck struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HealthCheckSpec   `json:"spec,omitempty"`
	Status HealthCheckStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// HealthCheckList contains a list of HealthCheck
type HealthCheckList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HealthCheck `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HealthCheck{}, &HealthCheckList{})
}
