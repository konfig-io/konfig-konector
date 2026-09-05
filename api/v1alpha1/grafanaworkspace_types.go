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

// GrafanaWorkspaceSpec defines the desired state of an Amazon Managed
// Grafana workspace.
type GrafanaWorkspaceSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// WorkspaceName is the name of the workspace. It does not have to be unique.
	// +kubebuilder:validation:MinLength=1
	WorkspaceName string `json:"workspaceName"`

	// AccountAccessType specifies whether the workspace accesses AWS resources
	// in this account only or across an organization.
	// +kubebuilder:validation:Enum=CURRENT_ACCOUNT;ORGANIZATION
	AccountAccessType string `json:"accountAccessType"`

	// AuthenticationProviders lists the user authentication methods.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:Enum=AWS_SSO;SAML
	AuthenticationProviders []string `json:"authenticationProviders"`

	// PermissionType is SERVICE_MANAGED or CUSTOMER_MANAGED. Use
	// CUSTOMER_MANAGED when creating workspaces through the API.
	// +kubebuilder:validation:Enum=SERVICE_MANAGED;CUSTOMER_MANAGED
	PermissionType string `json:"permissionType"`

	// WorkspaceRoleRef references the IAM role that the workspace uses to
	// access AWS data sources and notification channels, either a managed
	// IAMRole CR (name) or a direct role ARN (arn).
	// +optional
	WorkspaceRoleRef *RoleRef `json:"workspaceRoleRef,omitempty"`

	// DataSources lists AWS data sources the workspace can access, e.g.
	// CLOUDWATCH, PROMETHEUS, XRAY.
	// +optional
	DataSources []string `json:"dataSources,omitempty"`

	// GrafanaVersion is the Grafana version for the workspace (e.g. "10.4").
	// Defaults to the latest supported version.
	// +optional
	GrafanaVersion string `json:"grafanaVersion,omitempty"`

	// Tags are AWS resource tags to apply at creation time.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// GrafanaWorkspaceStatus defines the observed state of GrafanaWorkspace.
type GrafanaWorkspaceStatus struct {
	// WorkspaceID is the unique ID of the workspace (g-...).
	// +optional
	WorkspaceID string `json:"workspaceId,omitempty"`

	// Endpoint is the URL of the Grafana console for the workspace.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// Status is the current AWS status of the workspace (CREATING, ACTIVE, ...).
	// +optional
	Status string `json:"status,omitempty"`

	// GrafanaVersion is the Grafana version running in the workspace.
	// +optional
	GrafanaVersion string `json:"grafanaVersion,omitempty"`

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

// GrafanaWorkspace is the Schema for managing Amazon Managed Grafana workspaces.
type GrafanaWorkspace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GrafanaWorkspaceSpec   `json:"spec,omitempty"`
	Status GrafanaWorkspaceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GrafanaWorkspaceList contains a list of GrafanaWorkspace
type GrafanaWorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GrafanaWorkspace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GrafanaWorkspace{}, &GrafanaWorkspaceList{})
}
