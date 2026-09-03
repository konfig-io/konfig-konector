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

// PermissionSetRef references either a managed PermissionSet CR or a direct
// permission set ARN.
type PermissionSetRef struct {
	// Name of a PermissionSet CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ARN is a direct IAM Identity Center permission set ARN.
	// If set, Name is ignored.
	// +optional
	ARN string `json:"arn,omitempty"`
}

// SSOAssignmentSpec defines the desired state of an IAM Identity Center
// account assignment. Creation and deletion are asynchronous; the controller
// polls the assignment operation status. Assignments only reconcile
// successfully from the account where the Identity Center instance lives.
type SSOAssignmentSpec struct {
	// InstanceArn is the ARN of the IAM Identity Center instance. Immutable
	// after creation.
	// +kubebuilder:validation:MinLength=10
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="instanceArn is immutable"
	InstanceArn string `json:"instanceArn"`

	// PermissionSetRef references the permission set to assign.
	PermissionSetRef PermissionSetRef `json:"permissionSetRef"`

	// PrincipalType is the type of the Identity Center principal. Immutable
	// after creation.
	// +kubebuilder:validation:Enum=USER;GROUP
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="principalType is immutable"
	PrincipalType string `json:"principalType"`

	// PrincipalId is the GUID of the Identity Center user or group.
	// Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="principalId is immutable"
	PrincipalId string `json:"principalId"`

	// TargetType is the entity type the assignment targets. Only AWS_ACCOUNT
	// is supported.
	// +kubebuilder:validation:Enum=AWS_ACCOUNT
	// +kubebuilder:default=AWS_ACCOUNT
	// +optional
	TargetType string `json:"targetType,omitempty"`

	// TargetId is the 12-digit AWS account ID the assignment targets.
	// Immutable after creation.
	// +kubebuilder:validation:MinLength=12
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="targetId is immutable"
	TargetId string `json:"targetId"`
}

// SSOAssignmentStatus defines the observed state of SSOAssignment.
type SSOAssignmentStatus struct {
	// PermissionSetArn is the resolved permission set ARN that was assigned.
	// +optional
	PermissionSetArn string `json:"permissionSetArn,omitempty"`

	// RequestId tracks the in-flight asynchronous create operation.
	// +optional
	RequestId string `json:"requestId,omitempty"`

	// State is the last observed assignment operation state
	// (IN_PROGRESS, SUCCEEDED, or FAILED).
	// +optional
	State string `json:"state,omitempty"`

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
// +kubebuilder:printcolumn:name="PermissionSet-ARN",type="string",JSONPath=".status.permissionSetArn"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SSOAssignment is the Schema for managing IAM Identity Center account assignments.
type SSOAssignment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SSOAssignmentSpec   `json:"spec,omitempty"`
	Status SSOAssignmentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SSOAssignmentList contains a list of SSOAssignment
type SSOAssignmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SSOAssignment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SSOAssignment{}, &SSOAssignmentList{})
}
