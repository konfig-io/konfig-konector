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

// CostAnomalyMonitorSpec defines the desired state of a Cost Explorer
// anomaly monitor.
type CostAnomalyMonitorSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// MonitorName is the name of the monitor.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	MonitorName string `json:"monitorName"`

	// MonitorType is DIMENSIONAL or CUSTOM. Immutable.
	// +kubebuilder:validation:Enum=DIMENSIONAL;CUSTOM
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="monitorType is immutable"
	MonitorType string `json:"monitorType"`

	// MonitorDimension is the dimension to evaluate for DIMENSIONAL monitors.
	// Currently only SERVICE is supported by AWS.
	// +kubebuilder:validation:Enum=SERVICE
	// +optional
	MonitorDimension string `json:"monitorDimension,omitempty"`
}

// CostAnomalyMonitorStatus defines the observed state of CostAnomalyMonitor.
type CostAnomalyMonitorStatus struct {
	// ARN is the ARN of the anomaly monitor.
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

// CostAnomalyMonitor is the Schema for managing Cost Explorer anomaly monitors.
type CostAnomalyMonitor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CostAnomalyMonitorSpec   `json:"spec,omitempty"`
	Status CostAnomalyMonitorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CostAnomalyMonitorList contains a list of CostAnomalyMonitor
type CostAnomalyMonitorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CostAnomalyMonitor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CostAnomalyMonitor{}, &CostAnomalyMonitorList{})
}
