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

// EKSAccessScope defines the scope of an EKS access policy.
type EKSAccessScope struct {
	// Type is the scope type: cluster or namespace.
	// +kubebuilder:validation:Enum=cluster;namespace
	Type string `json:"type"`

	// Namespaces is the list of Kubernetes namespaces the policy applies to.
	// Required when Type is "namespace".
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`
}

// EKSAccessPolicyAssociation associates an EKS access policy with an access entry.
type EKSAccessPolicyAssociation struct {
	// PolicyArn is the ARN of the EKS access policy.
	// Example: arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy
	PolicyArn string `json:"policyArn"`

	// AccessScope defines the scope of the policy.
	AccessScope EKSAccessScope `json:"accessScope"`
}

// EKSAccessEntrySpec defines the desired state of an EKS Access Entry.
type EKSAccessEntrySpec struct {
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

	// PrincipalArn is the IAM role or user ARN to grant cluster access.
	// Either principalArn or principalRef must be set.
	// +optional
	PrincipalArn string `json:"principalArn,omitempty"`

	// PrincipalRef references an IAMRole CR in the same namespace.
	// +optional
	PrincipalRef *RoleRef `json:"principalRef,omitempty"`

	// Type is the access entry type.
	// +kubebuilder:validation:Enum=STANDARD;FARGATE_LINUX;EC2_LINUX;EC2_WINDOWS
	// +optional
	Type string `json:"type,omitempty"`

	// KubernetesGroups are the Kubernetes RBAC groups to assign to this principal.
	// +optional
	KubernetesGroups []string `json:"kubernetesGroups,omitempty"`

	// Username is the Kubernetes username for this principal.
	// +optional
	Username string `json:"username,omitempty"`

	// AccessPolicies are the EKS access policies to associate with this entry.
	// +optional
	AccessPolicies []EKSAccessPolicyAssociation `json:"accessPolicies,omitempty"`

	// Tags are AWS resource tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// EKSAccessEntryStatus defines the observed state of EKSAccessEntry.
type EKSAccessEntryStatus struct {
	// AccessEntryArn is the ARN of the access entry.
	// +optional
	AccessEntryArn string `json:"accessEntryArn,omitempty"`

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
// +kubebuilder:printcolumn:name="Principal",type="string",JSONPath=".spec.principalArn"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EKSAccessEntry is the Schema for managing EKS Access Entries.
type EKSAccessEntry struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EKSAccessEntrySpec   `json:"spec,omitempty"`
	Status EKSAccessEntryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EKSAccessEntryList contains a list of EKSAccessEntry.
type EKSAccessEntryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EKSAccessEntry `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EKSAccessEntry{}, &EKSAccessEntryList{})
}
