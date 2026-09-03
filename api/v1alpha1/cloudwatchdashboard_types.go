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

// CloudWatchDashboardSpec defines the desired state of a CloudWatch Dashboard.
type CloudWatchDashboardSpec struct {
	// DashboardName is the name of the dashboard.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="dashboardName is immutable"
	DashboardName string `json:"dashboardName"`

	// DashboardBody is the JSON body of the dashboard configuration.
	DashboardBody string `json:"dashboardBody"`
}

// CloudWatchDashboardStatus defines the observed state of CloudWatchDashboard.
type CloudWatchDashboardStatus struct {
	// DashboardARN is the ARN of the dashboard.
	// +optional
	DashboardARN string `json:"dashboardArn,omitempty"`

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
// +kubebuilder:printcolumn:name="Name",type="string",JSONPath=".spec.dashboardName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CloudWatchDashboard is the Schema for managing CloudWatch dashboards.
type CloudWatchDashboard struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudWatchDashboardSpec   `json:"spec,omitempty"`
	Status CloudWatchDashboardStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CloudWatchDashboardList contains a list of CloudWatchDashboard.
type CloudWatchDashboardList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudWatchDashboard `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudWatchDashboard{}, &CloudWatchDashboardList{})
}
