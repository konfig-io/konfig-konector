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

// PodIdentityAssociationSpec defines the desired state of an EKS Pod Identity Association.
type PodIdentityAssociationSpec struct {
	// ClusterName is the name of the EKS cluster.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="clusterName is immutable"
	ClusterName string `json:"clusterName"`

	// Namespace is the Kubernetes namespace of the target ServiceAccount.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="targetNamespace is immutable"
	TargetNamespace string `json:"targetNamespace"`

	// ServiceAccountName is the name of the Kubernetes ServiceAccount to bind.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serviceAccountName is immutable"
	ServiceAccountName string `json:"serviceAccountName"`

	// RoleRef references the IAMRole CR or a direct ARN to bind to the ServiceAccount.
	RoleRef RoleRef `json:"roleRef"`

	// AnnotateServiceAccount controls whether the operator also writes the
	// eks.amazonaws.com/role-arn annotation on the target ServiceAccount (for IRSA
	// compatibility). Defaults to false.
	// +optional
	AnnotateServiceAccount bool `json:"annotateServiceAccount,omitempty"`

	// Tags are AWS resource tags applied to the association.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// PodIdentityAssociationStatus defines the observed state of PodIdentityAssociation.
type PodIdentityAssociationStatus struct {
	// AssociationID is the EKS Pod Identity association identifier.
	// +optional
	AssociationID string `json:"associationId,omitempty"`

	// AssociationARN is the ARN of the EKS Pod Identity association.
	// +optional
	AssociationARN string `json:"associationArn,omitempty"`

	// RoleARN is the resolved IAM role ARN.
	// +optional
	RoleARN string `json:"roleArn,omitempty"`

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
// +kubebuilder:printcolumn:name="Namespace",type="string",JSONPath=".spec.targetNamespace"
// +kubebuilder:printcolumn:name="ServiceAccount",type="string",JSONPath=".spec.serviceAccountName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PodIdentityAssociation links a Kubernetes ServiceAccount to an AWS IAM Role
// via EKS Pod Identity.
type PodIdentityAssociation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodIdentityAssociationSpec   `json:"spec,omitempty"`
	Status PodIdentityAssociationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PodIdentityAssociationList contains a list of PodIdentityAssociation
type PodIdentityAssociationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodIdentityAssociation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PodIdentityAssociation{}, &PodIdentityAssociationList{})
}
