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

// HostedZoneRef references a HostedZone CR or a direct Route53 zone ID.
type HostedZoneRef struct {
	// Name of a HostedZone CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ID is a direct Route53 hosted zone ID (e.g. "Z1234567890").
	// If set, Name is ignored.
	// +optional
	ID string `json:"id,omitempty"`
}

// AliasTarget defines a Route53 alias record target.
type AliasTarget struct {
	// DNSName is the DNS name of the target resource.
	DNSName string `json:"dnsName"`
	// HostedZoneID is the hosted zone ID of the target resource.
	HostedZoneID string `json:"hostedZoneId"`
	// EvaluateTargetHealth enables health checking of the alias target.
	// +optional
	EvaluateTargetHealth bool `json:"evaluateTargetHealth,omitempty"`
}

// RecordSetSpec defines the desired state of a Route53 DNS record set.
type RecordSetSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// HostedZoneRef references the hosted zone that owns this record.
	HostedZoneRef HostedZoneRef `json:"hostedZoneRef"`

	// Name is the DNS record name (e.g. "www.example.com.").
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Type is the DNS record type (A, AAAA, CNAME, MX, TXT, NS, SOA, SRV, CAA, PTR).
	// +kubebuilder:validation:Enum=A;AAAA;CNAME;MX;TXT;NS;SOA;SRV;CAA;PTR
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="type is immutable"
	Type string `json:"type"`

	// TTL is the time-to-live for the record in seconds.
	// Required for non-alias records.
	// +optional
	TTL *int64 `json:"ttl,omitempty"`

	// Records are the record values. Mutually exclusive with Alias.
	// +optional
	Records []string `json:"records,omitempty"`

	// Alias defines an alias record target. Mutually exclusive with Records and TTL.
	// +optional
	Alias *AliasTarget `json:"alias,omitempty"`

	// Weight is used for weighted routing policy (0–255).
	// +optional
	Weight *int64 `json:"weight,omitempty"`

	// SetIdentifier is a unique identifier to differentiate records with the same
	// name and type in weighted, latency, failover, or geolocation routing.
	// +optional
	SetIdentifier string `json:"setIdentifier,omitempty"`

	// Failover sets PRIMARY or SECONDARY failover routing.
	// +kubebuilder:validation:Enum=PRIMARY;SECONDARY
	// +optional
	Failover string `json:"failover,omitempty"`

	// HealthCheckRef references a HealthCheck CR to associate with this record.
	// +optional
	HealthCheckRef *ResourceRef `json:"healthCheckRef,omitempty"`
}

// RecordSetStatus defines the observed state of RecordSet.
type RecordSetStatus struct {
	// ChangeID is the Route53 change ID for the last applied change.
	// +optional
	ChangeID string `json:"changeId,omitempty"`

	// ChangeStatus is the status of the last Route53 change (PENDING or INSYNC).
	// +optional
	ChangeStatus string `json:"changeStatus,omitempty"`

	// HostedZoneID is the resolved Route53 hosted zone ID.
	// +optional
	HostedZoneID string `json:"hostedZoneId,omitempty"`

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
// +kubebuilder:printcolumn:name="Name",type="string",JSONPath=".spec.name"
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type"
// +kubebuilder:printcolumn:name="ChangeStatus",type="string",JSONPath=".status.changeStatus"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// RecordSet is the Schema for managing AWS Route53 DNS record sets.
type RecordSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RecordSetSpec   `json:"spec,omitempty"`
	Status RecordSetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RecordSetList contains a list of RecordSet
type RecordSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RecordSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RecordSet{}, &RecordSetList{})
}
