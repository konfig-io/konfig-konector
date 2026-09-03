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

// PrometheusWorkspaceRef references either a managed PrometheusWorkspace CR
// or a direct AWS workspace ID.
type PrometheusWorkspaceRef struct {
	// Name of a PrometheusWorkspace CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// WorkspaceID is a direct AWS workspace ID (ws-...), bypassing CR lookup.
	// +optional
	WorkspaceID string `json:"workspaceId,omitempty"`
}

// PrometheusRuleGroupsNamespaceSpec defines the desired state of a rule
// groups namespace in an Amazon Managed Prometheus workspace.
type PrometheusRuleGroupsNamespaceSpec struct {
	// WorkspaceRef references the Prometheus workspace containing the namespace.
	WorkspaceRef PrometheusWorkspaceRef `json:"workspaceRef"`

	// Name is the name of the rule groups namespace. Immutable.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Data is the rule groups file for the namespace, as a YAML string.
	// +kubebuilder:validation:MinLength=1
	Data string `json:"data"`

	// Tags are AWS resource tags to apply at creation time.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// PrometheusRuleGroupsNamespaceStatus defines the observed state of
// PrometheusRuleGroupsNamespace.
type PrometheusRuleGroupsNamespaceStatus struct {
	// ARN is the ARN of the rule groups namespace.
	// +optional
	ARN string `json:"arn,omitempty"`

	// WorkspaceID is the resolved AWS workspace ID the namespace was created in.
	// +optional
	WorkspaceID string `json:"workspaceId,omitempty"`

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

// PrometheusRuleGroupsNamespace is the Schema for managing rule groups
// namespaces in Amazon Managed Prometheus workspaces.
type PrometheusRuleGroupsNamespace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PrometheusRuleGroupsNamespaceSpec   `json:"spec,omitempty"`
	Status PrometheusRuleGroupsNamespaceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PrometheusRuleGroupsNamespaceList contains a list of PrometheusRuleGroupsNamespace
type PrometheusRuleGroupsNamespaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PrometheusRuleGroupsNamespace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PrometheusRuleGroupsNamespace{}, &PrometheusRuleGroupsNamespaceList{})
}
