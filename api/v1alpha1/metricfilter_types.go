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

// LogGroupRef references either a managed LogGroup CR or a direct log group name.
type LogGroupRef struct {
	// Name of a LogGroup CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// LogGroupName is a direct AWS log group name.
	// +optional
	LogGroupName string `json:"logGroupName,omitempty"`
}

// MetricTransformation defines a metric transformation for a metric filter.
type MetricTransformation struct {
	// MetricName is the name of the CloudWatch metric.
	MetricName string `json:"metricName"`
	// MetricNamespace is the namespace for the metric.
	MetricNamespace string `json:"metricNamespace"`
	// MetricValue is the value to publish when the filter pattern matches.
	MetricValue string `json:"metricValue"`
	// DefaultValue is the value to emit when no data matches.
	// +optional
	DefaultValue *float64 `json:"defaultValue,omitempty"`
}

// MetricFilterSpec defines the desired state of a Metric Filter.
type MetricFilterSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// LogGroupRef references the log group.
	LogGroupRef LogGroupRef `json:"logGroupRef"`

	// FilterName is the name of the metric filter. Immutable.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="filterName is immutable"
	FilterName string `json:"filterName"`

	// FilterPattern is the CloudWatch Logs filter pattern.
	FilterPattern string `json:"filterPattern"`

	// MetricTransformations are the transformations to apply.
	// +kubebuilder:validation:MinItems=1
	MetricTransformations []MetricTransformation `json:"metricTransformations"`
}

// MetricFilterStatus defines the observed state of MetricFilter.
type MetricFilterStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
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

// MetricFilter is the Schema for managing CloudWatch Logs Metric Filters.
type MetricFilter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MetricFilterSpec   `json:"spec,omitempty"`
	Status MetricFilterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MetricFilterList contains a list of MetricFilter
type MetricFilterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MetricFilter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MetricFilter{}, &MetricFilterList{})
}
