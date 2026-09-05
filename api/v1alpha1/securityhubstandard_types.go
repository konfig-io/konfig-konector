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

// SecurityHubStandardSpec defines the desired state of a Security Hub
// standards subscription.
type SecurityHubStandardSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// StandardsARN is the ARN of the standard to enable (see
	// DescribeStandards), e.g.
	// arn:aws:securityhub:us-east-1::standards/aws-foundational-security-best-practices/v/1.0.0.
	// Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="standardsArn is immutable"
	StandardsARN string `json:"standardsArn"`
}

// SecurityHubStandardStatus defines the observed state of SecurityHubStandard.
type SecurityHubStandardStatus struct {
	// SubscriptionARN is the ARN of the standards subscription.
	// +optional
	SubscriptionARN string `json:"subscriptionArn,omitempty"`

	// StandardsStatus is the subscription status reported by AWS
	// (PENDING, READY, INCOMPLETE, DELETING, FAILED).
	// +optional
	StandardsStatus string `json:"standardsStatus,omitempty"`

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
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.standardsStatus"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SecurityHubStandard is the Schema for managing Security Hub standards
// subscriptions.
type SecurityHubStandard struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SecurityHubStandardSpec   `json:"spec,omitempty"`
	Status SecurityHubStandardStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SecurityHubStandardList contains a list of SecurityHubStandard
type SecurityHubStandardList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SecurityHubStandard `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SecurityHubStandard{}, &SecurityHubStandardList{})
}
