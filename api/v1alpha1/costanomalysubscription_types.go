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

// CostAnomalyMonitorRef references either a managed CostAnomalyMonitor CR or
// a direct monitor ARN.
type CostAnomalyMonitorRef struct {
	// Name of a CostAnomalyMonitor CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ARN is a direct Cost Explorer anomaly monitor ARN, bypassing CR lookup.
	// +optional
	ARN string `json:"arn,omitempty"`
}

// CostAnomalySubscriber is a subscriber to cost anomaly alerts.
type CostAnomalySubscriber struct {
	// Address is the email address or SNS topic ARN to notify.
	// +kubebuilder:validation:MinLength=1
	Address string `json:"address"`

	// Type is the delivery channel. Use EMAIL for DAILY/WEEKLY frequencies
	// and SNS for IMMEDIATE.
	// +kubebuilder:validation:Enum=EMAIL;SNS
	Type string `json:"type"`
}

// CostAnomalySubscriptionSpec defines the desired state of a Cost Explorer
// anomaly subscription.
type CostAnomalySubscriptionSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// SubscriptionName is the name of the subscription.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	SubscriptionName string `json:"subscriptionName"`

	// MonitorRefs references the anomaly monitors this subscription alerts on.
	// +kubebuilder:validation:MinItems=1
	MonitorRefs []CostAnomalyMonitorRef `json:"monitorRefs"`

	// Threshold is the absolute dollar impact an anomaly must exceed to
	// trigger a notification, as a numeric string (e.g. "100").
	// +kubebuilder:validation:Pattern=`^[0-9]+(\.[0-9]+)?$`
	Threshold string `json:"threshold"`

	// Frequency is how often notifications are sent. IMMEDIATE requires SNS
	// subscribers; DAILY and WEEKLY use email.
	// +kubebuilder:validation:Enum=DAILY;IMMEDIATE;WEEKLY
	Frequency string `json:"frequency"`

	// Subscribers to notify about anomalies.
	// +kubebuilder:validation:MinItems=1
	Subscribers []CostAnomalySubscriber `json:"subscribers"`
}

// CostAnomalySubscriptionStatus defines the observed state of CostAnomalySubscription.
type CostAnomalySubscriptionStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ARN is the ARN of the anomaly subscription.
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
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CostAnomalySubscription is the Schema for managing Cost Explorer anomaly
// subscriptions.
type CostAnomalySubscription struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CostAnomalySubscriptionSpec   `json:"spec,omitempty"`
	Status CostAnomalySubscriptionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CostAnomalySubscriptionList contains a list of CostAnomalySubscription
type CostAnomalySubscriptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CostAnomalySubscription `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CostAnomalySubscription{}, &CostAnomalySubscriptionList{})
}
