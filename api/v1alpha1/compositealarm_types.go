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

// CompositeAlarmSpec defines the desired state of a CloudWatch Composite Alarm.
type CompositeAlarmSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// AlarmName is the name of the composite alarm.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="alarmName is immutable"
	AlarmName string `json:"alarmName"`

	// AlarmRule is the expression that specifies which alarms trigger the composite alarm.
	AlarmRule string `json:"alarmRule"`

	// AlarmDescription is a description for the alarm.
	// +optional
	AlarmDescription string `json:"alarmDescription,omitempty"`

	// ActionsEnabled indicates whether actions should be executed during alarm state changes.
	// +optional
	ActionsEnabled *bool `json:"actionsEnabled,omitempty"`

	// AlarmActions are ARNs of actions to execute when the alarm transitions to ALARM state.
	// +optional
	AlarmActions []string `json:"alarmActions,omitempty"`

	// OKActions are ARNs of actions to execute when the alarm transitions to OK state.
	// +optional
	OKActions []string `json:"okActions,omitempty"`

	// InsufficientDataActions are ARNs of actions to execute when the alarm has insufficient data.
	// +optional
	InsufficientDataActions []string `json:"insufficientDataActions,omitempty"`

	// Tags are metadata tags for the composite alarm.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// CompositeAlarmStatus defines the observed state of CompositeAlarm.
type CompositeAlarmStatus struct {
	// AlarmARN is the ARN of the composite alarm.
	// +optional
	AlarmARN string `json:"alarmArn,omitempty"`

	// AlarmState is the current state of the alarm.
	// +optional
	AlarmState string `json:"alarmState,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.alarmArn"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.alarmState"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CompositeAlarm is the Schema for managing CloudWatch composite alarms.
type CompositeAlarm struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CompositeAlarmSpec   `json:"spec,omitempty"`
	Status CompositeAlarmStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CompositeAlarmList contains a list of CompositeAlarm.
type CompositeAlarmList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CompositeAlarm `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CompositeAlarm{}, &CompositeAlarmList{})
}
