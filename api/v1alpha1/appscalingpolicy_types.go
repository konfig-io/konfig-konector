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

// ScalableTargetRef references a ScalableTarget CR in the same namespace.
type ScalableTargetRef struct {
	// Name of a ScalableTarget CR in the same namespace.
	Name string `json:"name"`
}

// AppScalingTargetTracking configures a target tracking scaling policy.
type AppScalingTargetTracking struct {
	// PredefinedMetricType is the predefined metric to track, e.g.
	// ECSServiceAverageCPUUtilization or DynamoDBReadCapacityUtilization.
	PredefinedMetricType string `json:"predefinedMetricType"`

	// ResourceLabel identifies a specific resource for ALBRequestCountPerTarget.
	// +optional
	ResourceLabel string `json:"resourceLabel,omitempty"`

	// TargetValue is the target value for the metric.
	TargetValue float64 `json:"targetValue"`

	// ScaleInCooldown is the cooldown (seconds) after a scale-in activity.
	// +kubebuilder:validation:Minimum=0
	// +optional
	ScaleInCooldown *int32 `json:"scaleInCooldown,omitempty"`

	// ScaleOutCooldown is the cooldown (seconds) after a scale-out activity.
	// +kubebuilder:validation:Minimum=0
	// +optional
	ScaleOutCooldown *int32 `json:"scaleOutCooldown,omitempty"`

	// DisableScaleIn prevents the policy from scaling in.
	// +optional
	DisableScaleIn bool `json:"disableScaleIn,omitempty"`
}

// AppScalingStepAdjustment defines one step of a step scaling policy.
type AppScalingStepAdjustment struct {
	// MetricIntervalLowerBound is the lower bound of the step (difference from threshold).
	// +optional
	MetricIntervalLowerBound *float64 `json:"metricIntervalLowerBound,omitempty"`

	// MetricIntervalUpperBound is the upper bound of the step (difference from threshold).
	// +optional
	MetricIntervalUpperBound *float64 `json:"metricIntervalUpperBound,omitempty"`

	// ScalingAdjustment is the amount to scale by (interpreted per adjustmentType).
	ScalingAdjustment int32 `json:"scalingAdjustment"`
}

// AppScalingStepScaling configures a step scaling policy.
type AppScalingStepScaling struct {
	// AdjustmentType: ChangeInCapacity, PercentChangeInCapacity, or ExactCapacity.
	// +kubebuilder:validation:Enum=ChangeInCapacity;PercentChangeInCapacity;ExactCapacity
	AdjustmentType string `json:"adjustmentType"`

	// Cooldown between scaling activities in seconds.
	// +optional
	Cooldown *int32 `json:"cooldown,omitempty"`

	// MetricAggregationType: Average, Minimum, or Maximum.
	// +kubebuilder:validation:Enum=Average;Minimum;Maximum
	// +optional
	MetricAggregationType string `json:"metricAggregationType,omitempty"`

	// MinAdjustmentMagnitude for PercentChangeInCapacity adjustments.
	// +optional
	MinAdjustmentMagnitude *int32 `json:"minAdjustmentMagnitude,omitempty"`

	// StepAdjustments are the scaling steps.
	// +kubebuilder:validation:MinItems=1
	StepAdjustments []AppScalingStepAdjustment `json:"stepAdjustments"`
}

// AppScalingPolicySpec defines the desired state of an Application Auto
// Scaling scaling policy.
type AppScalingPolicySpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// PolicyName is the name of the scaling policy. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="policyName is immutable"
	PolicyName string `json:"policyName"`

	// TargetRef references a ScalableTarget CR whose serviceNamespace,
	// resourceId and scalableDimension the policy applies to. Either targetRef
	// or the inline serviceNamespace/resourceId/scalableDimension must be set.
	// +optional
	TargetRef *ScalableTargetRef `json:"targetRef,omitempty"`

	// ServiceNamespace of the scalable target (inline alternative to targetRef).
	// +optional
	ServiceNamespace string `json:"serviceNamespace,omitempty"`

	// ResourceID of the scalable target (inline alternative to targetRef).
	// +optional
	ResourceID string `json:"resourceId,omitempty"`

	// ScalableDimension of the scalable target (inline alternative to targetRef).
	// +optional
	ScalableDimension string `json:"scalableDimension,omitempty"`

	// PolicyType is the scaling policy type.
	// +kubebuilder:validation:Enum=TargetTrackingScaling;StepScaling
	PolicyType string `json:"policyType"`

	// TargetTrackingConfiguration configures a TargetTrackingScaling policy.
	// +optional
	TargetTrackingConfiguration *AppScalingTargetTracking `json:"targetTrackingConfiguration,omitempty"`

	// StepScalingConfiguration configures a StepScaling policy.
	// +optional
	StepScalingConfiguration *AppScalingStepScaling `json:"stepScalingConfiguration,omitempty"`
}

// AppScalingPolicyStatus defines the observed state of AppScalingPolicy.
type AppScalingPolicyStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
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
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AppScalingPolicy is the Schema for managing Application Auto Scaling policies.
type AppScalingPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppScalingPolicySpec   `json:"spec,omitempty"`
	Status AppScalingPolicyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AppScalingPolicyList contains a list of AppScalingPolicy
type AppScalingPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AppScalingPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AppScalingPolicy{}, &AppScalingPolicyList{})
}
