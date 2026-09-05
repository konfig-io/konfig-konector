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

// StepAdjustment defines a step adjustment for a step scaling policy.
type StepAdjustment struct {
	// ScalingAdjustment is the amount by which to scale.
	ScalingAdjustment int32 `json:"scalingAdjustment"`
	// MetricIntervalLowerBound is the lower bound for the metric interval.
	// +optional
	MetricIntervalLowerBound *float64 `json:"metricIntervalLowerBound,omitempty"`
	// MetricIntervalUpperBound is the upper bound for the metric interval.
	// +optional
	MetricIntervalUpperBound *float64 `json:"metricIntervalUpperBound,omitempty"`
}

// TargetTrackingConfiguration defines the target tracking policy configuration.
type TargetTrackingConfiguration struct {
	// TargetValue is the target value for the metric.
	TargetValue float64 `json:"targetValue"`
	// PredefinedMetricType is a predefined metric (e.g. ASGAverageCPUUtilization).
	// +optional
	PredefinedMetricType string `json:"predefinedMetricType,omitempty"`
	// DisableScaleIn prevents scale-in when true.
	// +optional
	DisableScaleIn bool `json:"disableScaleIn,omitempty"`
}

// ScalingPolicySpec defines the desired state of a Scaling Policy.
type ScalingPolicySpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// AutoScalingGroupRef references the AutoScalingGroup CR.
	AutoScalingGroupRef ResourceRef `json:"autoScalingGroupRef"`

	// PolicyName is the name of the scaling policy. Immutable.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="policyName is immutable"
	PolicyName string `json:"policyName"`

	// PolicyType is the policy type.
	// +kubebuilder:validation:Enum=SimpleScaling;StepScaling;TargetTrackingScaling
	PolicyType string `json:"policyType"`

	// AdjustmentType is the adjustment type (for SimpleScaling/StepScaling).
	// +kubebuilder:validation:Enum=ChangeInCapacity;ExactCapacity;PercentChangeInCapacity
	// +optional
	AdjustmentType string `json:"adjustmentType,omitempty"`

	// ScalingAdjustment is the adjustment value (for SimpleScaling).
	// +optional
	ScalingAdjustment int32 `json:"scalingAdjustment,omitempty"`

	// Cooldown is the cooldown period in seconds (for SimpleScaling).
	// +optional
	Cooldown int32 `json:"cooldown,omitempty"`

	// StepAdjustments are the step adjustments (for StepScaling).
	// +optional
	StepAdjustments []StepAdjustment `json:"stepAdjustments,omitempty"`

	// TargetTrackingConfiguration defines target tracking (for TargetTrackingScaling).
	// +optional
	TargetTrackingConfiguration *TargetTrackingConfiguration `json:"targetTrackingConfiguration,omitempty"`
}

// ScalingPolicyStatus defines the observed state of ScalingPolicy.
type ScalingPolicyStatus struct {
	// PolicyARN is the ARN of the scaling policy.
	// +optional
	PolicyARN string `json:"policyArn,omitempty"`

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
// +kubebuilder:printcolumn:name="Policy-ARN",type="string",JSONPath=".status.policyArn"
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.policyType"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ScalingPolicy is the Schema for managing AutoScaling Scaling Policies.
type ScalingPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScalingPolicySpec   `json:"spec,omitempty"`
	Status ScalingPolicyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ScalingPolicyList contains a list of ScalingPolicy
type ScalingPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScalingPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScalingPolicy{}, &ScalingPolicyList{})
}
