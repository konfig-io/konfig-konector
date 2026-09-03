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

// PrometheusWorkspaceSpec defines the desired state of an Amazon Managed
// Service for Prometheus workspace.
type PrometheusWorkspaceSpec struct {
	// Alias is a friendly name assigned to the workspace. It does not need to
	// be unique and can be updated.
	// +optional
	Alias string `json:"alias,omitempty"`

	// KMSKeyARN is the ARN of a customer managed KMS key used to encrypt data
	// in the workspace. Immutable in AWS after creation.
	// +optional
	KMSKeyARN string `json:"kmsKeyArn,omitempty"`

	// Tags are AWS resource tags to apply at creation time.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// PrometheusWorkspaceStatus defines the observed state of PrometheusWorkspace.
type PrometheusWorkspaceStatus struct {
	// WorkspaceID is the unique ID of the workspace (ws-...).
	// +optional
	WorkspaceID string `json:"workspaceId,omitempty"`

	// ARN is the ARN of the workspace.
	// +optional
	ARN string `json:"arn,omitempty"`

	// Status is the current AWS status of the workspace (CREATING, ACTIVE, ...).
	// +optional
	Status string `json:"status,omitempty"`

	// PrometheusEndpoint is the Prometheus endpoint URL of the workspace.
	// +optional
	PrometheusEndpoint string `json:"prometheusEndpoint,omitempty"`

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
// +kubebuilder:printcolumn:name="Workspace-ID",type="string",JSONPath=".status.workspaceId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PrometheusWorkspace is the Schema for managing Amazon Managed Prometheus workspaces.
type PrometheusWorkspace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PrometheusWorkspaceSpec   `json:"spec,omitempty"`
	Status PrometheusWorkspaceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PrometheusWorkspaceList contains a list of PrometheusWorkspace
type PrometheusWorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PrometheusWorkspace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PrometheusWorkspace{}, &PrometheusWorkspaceList{})
}
