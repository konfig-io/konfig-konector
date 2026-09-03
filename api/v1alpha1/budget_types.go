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

// BudgetSubscriber is a subscriber to a budget notification.
type BudgetSubscriber struct {
	// Address is the email address or SNS topic ARN to notify.
	// +kubebuilder:validation:MinLength=1
	Address string `json:"address"`

	// SubscriptionType is how notifications are delivered.
	// +kubebuilder:validation:Enum=EMAIL;SNS
	SubscriptionType string `json:"subscriptionType"`
}

// BudgetNotification attaches an alert threshold and subscribers to a budget.
type BudgetNotification struct {
	// NotificationType is whether the alert is on actual or forecasted spend.
	// +kubebuilder:validation:Enum=ACTUAL;FORECASTED
	NotificationType string `json:"notificationType"`

	// ComparisonOperator compares spend against the threshold.
	// +kubebuilder:validation:Enum=GREATER_THAN;LESS_THAN;EQUAL_TO
	ComparisonOperator string `json:"comparisonOperator"`

	// Threshold is the notification threshold value, as a numeric string
	// (e.g. "80" for 80% with thresholdType PERCENTAGE).
	// +kubebuilder:validation:Pattern=`^[0-9]+(\.[0-9]+)?$`
	Threshold string `json:"threshold"`

	// ThresholdType is PERCENTAGE or ABSOLUTE_VALUE. Defaults to PERCENTAGE.
	// +kubebuilder:validation:Enum=PERCENTAGE;ABSOLUTE_VALUE
	// +optional
	ThresholdType string `json:"thresholdType,omitempty"`

	// Subscribers to notify when the threshold is exceeded.
	// +kubebuilder:validation:MinItems=1
	Subscribers []BudgetSubscriber `json:"subscribers"`
}

// BudgetSpec defines the desired state of an AWS Budgets budget.
type BudgetSpec struct {
	// AccountID is the 12-digit AWS account ID that owns the budget. Immutable.
	// +kubebuilder:validation:Pattern=`^[0-9]{12}$`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="accountId is immutable"
	AccountID string `json:"accountId"`

	// BudgetName is the name of the budget, unique within the account. Immutable.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=100
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="budgetName is immutable"
	BudgetName string `json:"budgetName"`

	// BudgetType is what the budget tracks.
	// +kubebuilder:validation:Enum=COST;USAGE;RI_UTILIZATION;RI_COVERAGE;SAVINGS_PLANS_UTILIZATION;SAVINGS_PLANS_COVERAGE
	BudgetType string `json:"budgetType"`

	// TimeUnit is the period after which the budget resets.
	// +kubebuilder:validation:Enum=DAILY;MONTHLY;QUARTERLY;ANNUALLY
	TimeUnit string `json:"timeUnit"`

	// LimitAmount is the budgeted amount, as a numeric string (e.g. "100.0").
	// +kubebuilder:validation:Pattern=`^[0-9]+(\.[0-9]+)?$`
	LimitAmount string `json:"limitAmount"`

	// LimitUnit is the unit of the limit, e.g. "USD" for COST budgets or a
	// usage unit like "GB" for USAGE budgets.
	// +kubebuilder:validation:MinLength=1
	LimitUnit string `json:"limitUnit"`

	// CostFilters restrict the costs the budget tracks, e.g.
	// Service: ["Amazon Elastic Compute Cloud - Compute"].
	// +optional
	CostFilters map[string][]string `json:"costFilters,omitempty"`

	// Notifications are alert thresholds with subscribers.
	// +optional
	Notifications []BudgetNotification `json:"notifications,omitempty"`
}

// BudgetStatus defines the observed state of Budget.
type BudgetStatus struct {
	// BudgetName is the name of the budget in AWS (its primary identifier
	// together with the account ID).
	// +optional
	BudgetName string `json:"budgetName,omitempty"`

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

// Budget is the Schema for managing AWS Budgets budgets.
type Budget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BudgetSpec   `json:"spec,omitempty"`
	Status BudgetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BudgetList contains a list of Budget
type BudgetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Budget `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Budget{}, &BudgetList{})
}
