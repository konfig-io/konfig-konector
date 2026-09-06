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

// ScheduleGroupRef references either a managed ScheduleGroup CR or a direct
// AWS schedule group name.
type ScheduleGroupRef struct {
	// Name of a ScheduleGroup CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// GroupName is a direct AWS schedule group name, bypassing CR lookup.
	// +optional
	GroupName string `json:"groupName,omitempty"`
}

// ScheduleFlexibleTimeWindow configures a flexible invocation window.
type ScheduleFlexibleTimeWindow struct {
	// Mode is OFF or FLEXIBLE.
	// +kubebuilder:validation:Enum=OFF;FLEXIBLE
	Mode string `json:"mode"`

	// MaximumWindowInMinutes is the maximum window when mode is FLEXIBLE.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1440
	// +optional
	MaximumWindowInMinutes *int32 `json:"maximumWindowInMinutes,omitempty"`
}

// ScheduleRetryPolicy configures target invocation retries.
type ScheduleRetryPolicy struct {
	// MaximumRetryAttempts before the invocation is dropped or dead-lettered.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=185
	// +optional
	MaximumRetryAttempts *int32 `json:"maximumRetryAttempts,omitempty"`

	// MaximumEventAgeInSeconds is the maximum age of an event to retry.
	// +kubebuilder:validation:Minimum=60
	// +kubebuilder:validation:Maximum=86400
	// +optional
	MaximumEventAgeInSeconds *int32 `json:"maximumEventAgeInSeconds,omitempty"`
}

// ScheduleTarget defines the target invoked by the schedule.
type ScheduleTarget struct {
	// ARN of the target resource (Lambda function, SQS queue, ECS cluster...).
	// +kubebuilder:validation:MinLength=1
	ARN string `json:"arn"`

	// RoleRef references the IAM role EventBridge Scheduler assumes to invoke
	// the target (by CR name or direct ARN).
	RoleRef RoleRef `json:"roleRef"`

	// Input is a static JSON payload passed to the target.
	// +optional
	Input string `json:"input,omitempty"`

	// RetryPolicy for target invocation.
	// +optional
	RetryPolicy *ScheduleRetryPolicy `json:"retryPolicy,omitempty"`

	// DeadLetterARN is the ARN of an SQS queue for failed invocations.
	// +optional
	DeadLetterARN string `json:"deadLetterArn,omitempty"`
}

// ScheduleSpec defines the desired state of an EventBridge Scheduler schedule.
type ScheduleSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the name of the schedule. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// GroupRef references the schedule group. If unset, the default group is used.
	// +optional
	GroupRef *ScheduleGroupRef `json:"groupRef,omitempty"`

	// ScheduleExpression, e.g. rate(5 minutes), cron(0 12 * * ? *), or at(...).
	// +kubebuilder:validation:MinLength=1
	ScheduleExpression string `json:"scheduleExpression"`

	// Timezone for the schedule expression, e.g. America/New_York.
	// +optional
	Timezone string `json:"timezone,omitempty"`

	// FlexibleTimeWindow configures the invocation window.
	FlexibleTimeWindow ScheduleFlexibleTimeWindow `json:"flexibleTimeWindow"`

	// Target is the resource invoked by the schedule.
	Target ScheduleTarget `json:"target"`

	// State is ENABLED or DISABLED. Defaults to ENABLED.
	// +kubebuilder:validation:Enum=ENABLED;DISABLED
	// +optional
	State string `json:"state,omitempty"`

	// Description of the schedule.
	// +optional
	Description string `json:"description,omitempty"`
}

// ScheduleStatus defines the observed state of Schedule.
type ScheduleStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ARN is the ARN of the schedule.
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
// +kubebuilder:printcolumn:name="Expression",type="string",JSONPath=".spec.scheduleExpression"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Schedule is the Schema for managing EventBridge Scheduler schedules.
type Schedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScheduleSpec   `json:"spec,omitempty"`
	Status ScheduleStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ScheduleList contains a list of Schedule
type ScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Schedule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Schedule{}, &ScheduleList{})
}
