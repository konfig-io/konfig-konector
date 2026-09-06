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

// DelegationSignerRecordSpec defines the desired state of a DS record for DNSSEC delegation.
type DelegationSignerRecordSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// HostedZoneID is the Route53 hosted zone ID of the parent zone.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="hostedZoneID is immutable"
	HostedZoneID string `json:"hostedZoneID"`

	// ChildZoneName is the domain name of the child zone.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="childZoneName is immutable"
	ChildZoneName string `json:"childZoneName"`

	// DSRecord is the full DS record value (e.g. "12345 8 2 <hash>").
	DSRecord string `json:"dsRecord"`

	// TTL is the TTL in seconds for the DS record.
	// +optional
	TTL *int64 `json:"ttl,omitempty"`
}

// DelegationSignerRecordStatus defines the observed state of DelegationSignerRecord.
type DelegationSignerRecordStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ChangeID is the Route53 change ID for the DS record change.
	// +optional
	ChangeID string `json:"changeID,omitempty"`

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
// +kubebuilder:printcolumn:name="ChangeID",type="string",JSONPath=".status.changeID"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DelegationSignerRecord is the Schema for managing DS records in Route53 for DNSSEC delegation.
type DelegationSignerRecord struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DelegationSignerRecordSpec   `json:"spec,omitempty"`
	Status DelegationSignerRecordStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DelegationSignerRecordList contains a list of DelegationSignerRecord.
type DelegationSignerRecordList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DelegationSignerRecord `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DelegationSignerRecord{}, &DelegationSignerRecordList{})
}
