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

// EKSFargateSelector defines a namespace + label selector for Fargate pod scheduling.
type EKSFargateSelector struct {
	// Namespace is the Kubernetes namespace to match.
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`

	// Labels are the Kubernetes pod labels to match. If empty, all pods in the namespace match.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// EKSFargateProfileSpec defines the desired state of an EKS Fargate Profile.
type EKSFargateProfileSpec struct {
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

	// FargateProfileName is the name of the Fargate profile. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="fargateProfileName is immutable"
	FargateProfileName string `json:"fargateProfileName"`

	// PodExecutionRoleArn is the IAM role ARN for Fargate pod execution.
	// Either podExecutionRoleArn or podExecutionRoleRef must be set.
	// +optional
	PodExecutionRoleArn string `json:"podExecutionRoleArn,omitempty"`

	// PodExecutionRoleRef references an IAMRole CR in the same namespace.
	// +optional
	PodExecutionRoleRef *RoleRef `json:"podExecutionRoleRef,omitempty"`

	// SubnetRefs are the private subnets to run Fargate pods in.
	// +optional
	SubnetRefs []SubnetRef `json:"subnetRefs,omitempty"`

	// Selectors define which pods are scheduled on Fargate.
	// +kubebuilder:validation:MinItems=1
	Selectors []EKSFargateSelector `json:"selectors"`

	// Tags are AWS resource tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// EKSFargateProfileStatus defines the observed state of EKSFargateProfile.
type EKSFargateProfileStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// FargateProfileArn is the ARN of the Fargate profile.
	// +optional
	FargateProfileArn string `json:"fargateProfileArn,omitempty"`

	// Status is the profile status: CREATING, ACTIVE, DELETING, CREATE_FAILED, DELETE_FAILED.
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
// +kubebuilder:printcolumn:name="Profile",type="string",JSONPath=".spec.fargateProfileName"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EKSFargateProfile is the Schema for managing EKS Fargate Profiles.
type EKSFargateProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EKSFargateProfileSpec   `json:"spec,omitempty"`
	Status EKSFargateProfileStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EKSFargateProfileList contains a list of EKSFargateProfile.
type EKSFargateProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EKSFargateProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EKSFargateProfile{}, &EKSFargateProfileList{})
}
