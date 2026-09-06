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

// SNSTopicRef references an SNS topic by CR name or direct ARN.
type SNSTopicRef struct {
	// Name is the name of an SNSTopic CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`

	// ARN is a direct SNS topic ARN (bypasses CR lookup).
	// +optional
	ARN string `json:"arn,omitempty"`
}

// SNSSubscriptionSpec defines the desired state of an SNS Subscription.
type SNSSubscriptionSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// TopicRef references the SNS topic to subscribe to.
	TopicRef SNSTopicRef `json:"topicRef"`

	// Protocol is the subscription protocol: sqs, https, email, lambda, etc.
	// +kubebuilder:validation:Enum=sqs;https;http;email;email-json;sms;lambda;firehose;application
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="protocol is immutable"
	Protocol string `json:"protocol"`

	// Endpoint is the subscription endpoint (queue ARN, URL, email, etc.).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="endpoint is immutable"
	Endpoint string `json:"endpoint"`

	// FilterPolicy is a JSON filter policy for message filtering.
	// +optional
	FilterPolicy string `json:"filterPolicy,omitempty"`

	// RawMessageDelivery enables raw message delivery (no JSON wrapping).
	// +optional
	RawMessageDelivery bool `json:"rawMessageDelivery,omitempty"`
}

// SNSSubscriptionStatus defines the observed state of SNSSubscription.
type SNSSubscriptionStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// SubscriptionARN is the ARN of the subscription.
	// +optional
	SubscriptionARN string `json:"subscriptionArn,omitempty"`

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
// +kubebuilder:printcolumn:name="Protocol",type="string",JSONPath=".spec.protocol"
// +kubebuilder:printcolumn:name="Subscription-ARN",type="string",JSONPath=".status.subscriptionArn"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SNSSubscription is the Schema for managing SNS Subscriptions.
type SNSSubscription struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SNSSubscriptionSpec   `json:"spec,omitempty"`
	Status SNSSubscriptionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SNSSubscriptionList contains a list of SNSSubscription
type SNSSubscriptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SNSSubscription `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SNSSubscription{}, &SNSSubscriptionList{})
}
