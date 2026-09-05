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

// CloudMapNamespaceRef references either a managed CloudMapNamespace CR or a
// direct AWS namespace ID.
type CloudMapNamespaceRef struct {
	// Name of a CloudMapNamespace CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ID is a direct AWS Cloud Map namespace ID (e.g. ns-abc123).
	// If set, Name is ignored.
	// +optional
	ID string `json:"id,omitempty"`
}

// CloudMapDnsConfig configures the DNS records Cloud Map creates when an
// instance is registered.
type CloudMapDnsConfig struct {
	// RecordType of the Route 53 record.
	// +kubebuilder:validation:Enum=A;AAAA;SRV;CNAME
	RecordType string `json:"recordType"`

	// TTL of the record in seconds.
	// +kubebuilder:validation:Minimum=0
	TTL int64 `json:"ttl"`

	// RoutingPolicy applied to the records.
	// +kubebuilder:validation:Enum=MULTIVALUE;WEIGHTED
	// +optional
	RoutingPolicy string `json:"routingPolicy,omitempty"`
}

// CloudMapHealthCheckCustomConfig configures a custom health check.
type CloudMapHealthCheckCustomConfig struct {
	// FailureThreshold is deprecated by AWS and always treated as 1.
	// +kubebuilder:validation:Minimum=1
	// +optional
	FailureThreshold int32 `json:"failureThreshold,omitempty"`
}

// CloudMapServiceSpec defines the desired state of a Cloud Map service.
type CloudMapServiceSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name of the service. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=127
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// NamespaceRef references the namespace the service is created in.
	NamespaceRef CloudMapNamespaceRef `json:"namespaceRef"`

	// Description of the service.
	// +optional
	Description string `json:"description,omitempty"`

	// DnsConfig configures the DNS records for registered instances.
	// +optional
	DnsConfig *CloudMapDnsConfig `json:"dnsConfig,omitempty"`

	// HealthCheckCustomConfig configures a custom health check.
	// +optional
	HealthCheckCustomConfig *CloudMapHealthCheckCustomConfig `json:"healthCheckCustomConfig,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// CloudMapServiceStatus defines the observed state of CloudMapService.
type CloudMapServiceStatus struct {
	// ServiceID is the ID of the service.
	// +optional
	ServiceID string `json:"serviceId,omitempty"`

	// ARN of the service.
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
// +kubebuilder:printcolumn:name="Service-ID",type="string",JSONPath=".status.serviceId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CloudMapService is the Schema for managing AWS Cloud Map services.
type CloudMapService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudMapServiceSpec   `json:"spec,omitempty"`
	Status CloudMapServiceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CloudMapServiceList contains a list of CloudMapService
type CloudMapServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudMapService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudMapService{}, &CloudMapServiceList{})
}
