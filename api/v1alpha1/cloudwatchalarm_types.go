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

// CloudWatchDimension is a name/value pair that is part of the identity of a CloudWatch metric.
type CloudWatchDimension struct {
	// Name is the dimension name.
	Name string `json:"name"`
	// Value is the dimension value.
	Value string `json:"value"`
}

// CloudWatchAlarmSpec defines the desired state of a CloudWatch Alarm.
type CloudWatchAlarmSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// AlarmName is the name of the alarm.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="alarmName is immutable"
	AlarmName string `json:"alarmName"`

	// ComparisonOperator is the arithmetic operation to use when comparing the statistic and threshold.
	// +kubebuilder:validation:Enum=GreaterThanOrEqualToThreshold;GreaterThanThreshold;LessThanThreshold;LessThanOrEqualToThreshold;LessThanLowerOrGreaterThanUpperThreshold;LessThanLowerThreshold;GreaterThanUpperThreshold
	ComparisonOperator string `json:"comparisonOperator"`

	// EvaluationPeriods is the number of periods over which data is compared to the threshold.
	EvaluationPeriods int32 `json:"evaluationPeriods"`

	// MetricName is the name of the metric.
	// +optional
	MetricName string `json:"metricName,omitempty"`

	// Namespace is the namespace of the metric.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Statistic is the statistic for the metric.
	// +kubebuilder:validation:Enum=SampleCount;Average;Sum;Minimum;Maximum
	// +optional
	Statistic string `json:"statistic,omitempty"`

	// Period is the period in seconds over which the statistic is applied.
	// +optional
	Period *int32 `json:"period,omitempty"`

	// Threshold is the value to compare with the specified statistic.
	// +optional
	Threshold *float64 `json:"threshold,omitempty"`

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

	// Dimensions are the dimensions for the metric.
	// +optional
	Dimensions []CloudWatchDimension `json:"dimensions,omitempty"`

	// DatapointsToAlarm is the number of data points that must be breaching to trigger the alarm.
	// +optional
	DatapointsToAlarm *int32 `json:"datapointsToAlarm,omitempty"`

	// TreatMissingData sets how to handle missing data points.
	// +kubebuilder:validation:Enum=breaching;notBreaching;ignore;missing
	// +optional
	TreatMissingData string `json:"treatMissingData,omitempty"`

	// Unit is the unit of the metric.
	// +optional
	Unit string `json:"unit,omitempty"`

	// Tags are metadata tags for the alarm.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// CloudWatchAlarmStatus defines the observed state of CloudWatchAlarm.
type CloudWatchAlarmStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// AlarmARN is the ARN of the alarm.
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

// CloudWatchAlarm is the Schema for managing CloudWatch metric alarms.
type CloudWatchAlarm struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudWatchAlarmSpec   `json:"spec,omitempty"`
	Status CloudWatchAlarmStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CloudWatchAlarmList contains a list of CloudWatchAlarm.
type CloudWatchAlarmList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudWatchAlarm `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudWatchAlarm{}, &CloudWatchAlarmList{})
}
