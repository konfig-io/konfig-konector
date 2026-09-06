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

// SecurityHubAccountSpec defines the desired state of the Security Hub
// account subscription. Security Hub is a singleton per account/region:
// create at most one SecurityHubAccount CR per region.
type SecurityHubAccountSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// EnableDefaultStandards enables the standards Security Hub designates as
	// automatically enabled. Defaults to true in AWS if unset.
	// +optional
	EnableDefaultStandards *bool `json:"enableDefaultStandards,omitempty"`

	// ControlFindingGenerator specifies whether a control check generates a
	// single consolidated finding (SECURITY_CONTROL) or per-standard findings
	// (STANDARD_CONTROL).
	// +kubebuilder:validation:Enum=SECURITY_CONTROL;STANDARD_CONTROL
	// +optional
	ControlFindingGenerator string `json:"controlFindingGenerator,omitempty"`

	// Tags to add to the hub resource when enabling Security Hub.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// SecurityHubAccountStatus defines the observed state of SecurityHubAccount.
type SecurityHubAccountStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// HubARN is the ARN of the Hub resource.
	// +optional
	HubARN string `json:"hubArn,omitempty"`

	// SubscribedAt is when Security Hub was enabled in the account.
	// +optional
	SubscribedAt string `json:"subscribedAt,omitempty"`

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
// +kubebuilder:printcolumn:name="Hub-ARN",type="string",JSONPath=".status.hubArn"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SecurityHubAccount is the Schema for managing the Security Hub account
// subscription in a region.
type SecurityHubAccount struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SecurityHubAccountSpec   `json:"spec,omitempty"`
	Status SecurityHubAccountStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SecurityHubAccountList contains a list of SecurityHubAccount
type SecurityHubAccountList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SecurityHubAccount `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SecurityHubAccount{}, &SecurityHubAccountList{})
}
