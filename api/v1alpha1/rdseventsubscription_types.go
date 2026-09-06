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

// RDSEventSubscriptionSpec defines the desired state of an RDS event subscription.
type RDSEventSubscriptionSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// SubscriptionName is the name of the subscription. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="subscriptionName is immutable"
	SubscriptionName string `json:"subscriptionName"`

	// SnsTopicRef references the SNS topic that receives event notifications
	// (by SNSTopic CR name or direct ARN).
	SnsTopicRef SNSTopicRef `json:"snsTopicRef"`

	// SourceType of events, e.g. db-instance, db-cluster, db-snapshot.
	// +optional
	SourceType string `json:"sourceType,omitempty"`

	// EventCategories to subscribe to (e.g. failover, maintenance).
	// +optional
	EventCategories []string `json:"eventCategories,omitempty"`

	// SourceIds restricts events to specific source identifiers.
	// +optional
	SourceIds []string `json:"sourceIds,omitempty"`

	// Enabled activates the subscription. Defaults to true when omitted.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// RDSEventSubscriptionStatus defines the observed state of RDSEventSubscription.
type RDSEventSubscriptionStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ARN is the ARN of the event subscription.
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
// +kubebuilder:printcolumn:name="Subscription",type="string",JSONPath=".spec.subscriptionName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// RDSEventSubscription is the Schema for managing RDS event subscriptions.
type RDSEventSubscription struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RDSEventSubscriptionSpec   `json:"spec,omitempty"`
	Status RDSEventSubscriptionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RDSEventSubscriptionList contains a list of RDSEventSubscription
type RDSEventSubscriptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RDSEventSubscription `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RDSEventSubscription{}, &RDSEventSubscriptionList{})
}
