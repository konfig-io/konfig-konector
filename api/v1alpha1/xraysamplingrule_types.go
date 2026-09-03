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

// XRaySamplingRuleSpec defines the desired state of an X-Ray sampling rule.
// Matcher fields default to "*" (match everything) when omitted.
type XRaySamplingRuleSpec struct {
	// RuleName is the name of the sampling rule. Immutable.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=32
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="ruleName is immutable"
	RuleName string `json:"ruleName"`

	// Priority of the sampling rule. Lower values are evaluated first.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=9999
	Priority int32 `json:"priority"`

	// FixedRate is the fraction of matching requests to instrument, after
	// the reservoir is exhausted (e.g. 0.05).
	FixedRate float64 `json:"fixedRate"`

	// ReservoirSize is a fixed number of matching requests to instrument per
	// second, before applying the fixed rate.
	// +kubebuilder:validation:Minimum=0
	ReservoirSize int32 `json:"reservoirSize"`

	// ServiceName matches the name that the service uses to identify itself
	// in segments. Defaults to "*".
	// +optional
	ServiceName string `json:"serviceName,omitempty"`

	// ServiceType matches the origin that the service uses to identify its
	// type in segments. Defaults to "*".
	// +optional
	ServiceType string `json:"serviceType,omitempty"`

	// Host matches the hostname from a request URL. Defaults to "*".
	// +optional
	Host string `json:"host,omitempty"`

	// HTTPMethod matches the HTTP method of a request. Defaults to "*".
	// +optional
	HTTPMethod string `json:"httpMethod,omitempty"`

	// URLPath matches the path from a request URL. Defaults to "*".
	// +optional
	URLPath string `json:"urlPath,omitempty"`

	// ResourceARN matches the ARN of the AWS resource on which the service
	// runs. Defaults to "*".
	// +optional
	ResourceARN string `json:"resourceArn,omitempty"`

	// Tags are AWS resource tags to apply at creation time.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// XRaySamplingRuleStatus defines the observed state of XRaySamplingRule.
type XRaySamplingRuleStatus struct {
	// ARN is the ARN of the sampling rule.
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

// XRaySamplingRule is the Schema for managing X-Ray sampling rules.
type XRaySamplingRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   XRaySamplingRuleSpec   `json:"spec,omitempty"`
	Status XRaySamplingRuleStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// XRaySamplingRuleList contains a list of XRaySamplingRule
type XRaySamplingRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []XRaySamplingRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&XRaySamplingRule{}, &XRaySamplingRuleList{})
}
