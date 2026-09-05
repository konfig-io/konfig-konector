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

// EKSAddonSpec defines the desired state of an EKS Add-on.
type EKSAddonSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ClusterName is the EKS cluster name. Either clusterName or clusterRef must be set.
	// +optional
	ClusterName string `json:"clusterName,omitempty"`

	// ClusterRef references an EKSCluster CR in the same namespace.
	// +optional
	ClusterRef *EKSClusterRef `json:"clusterRef,omitempty"`

	// AddonName is the name of the EKS add-on (e.g. vpc-cni, kube-proxy, coredns).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="addonName is immutable"
	AddonName string `json:"addonName"`

	// AddonVersion is the version of the add-on. Leave empty for the latest recommended version.
	// +optional
	AddonVersion string `json:"addonVersion,omitempty"`

	// ServiceAccountRoleArn is an IAM role ARN for the add-on's service account (IRSA).
	// +optional
	ServiceAccountRoleArn string `json:"serviceAccountRoleArn,omitempty"`

	// ServiceAccountRoleRef references an IAMRole CR in the same namespace.
	// +optional
	ServiceAccountRoleRef *RoleRef `json:"serviceAccountRoleRef,omitempty"`

	// ResolveConflicts controls how conflicts are resolved when updating the add-on.
	// +kubebuilder:validation:Enum=OVERWRITE;NONE;PRESERVE
	// +optional
	ResolveConflicts string `json:"resolveConflicts,omitempty"`

	// ConfigurationValues is a JSON or YAML string of configuration values for the add-on.
	// +optional
	ConfigurationValues string `json:"configurationValues,omitempty"`

	// Tags are AWS resource tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// EKSAddonStatus defines the observed state of EKSAddon.
type EKSAddonStatus struct {
	// AddonArn is the ARN of the EKS add-on.
	// +optional
	AddonArn string `json:"addonArn,omitempty"`

	// AddonVersion is the currently installed add-on version.
	// +optional
	AddonVersion string `json:"addonVersion,omitempty"`

	// Status is the add-on status: CREATING, ACTIVE, UPDATING, DEGRADED, DELETING.
	// +optional
	Status string `json:"status,omitempty"`

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
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.clusterName"
// +kubebuilder:printcolumn:name="Addon",type="string",JSONPath=".spec.addonName"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".status.addonVersion"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EKSAddon is the Schema for managing EKS Add-ons.
type EKSAddon struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EKSAddonSpec   `json:"spec,omitempty"`
	Status EKSAddonStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EKSAddonList contains a list of EKSAddon.
type EKSAddonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EKSAddon `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EKSAddon{}, &EKSAddonList{})
}
