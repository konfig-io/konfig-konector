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

// LambdaPermissionSpec defines the desired state of a Lambda Permission.
type LambdaPermissionSpec struct {
	// FunctionRef references the Lambda function to grant permission on.
	// Either functionRef or functionArn must be set.
	// +optional
	FunctionRef *LambdaFunctionRef `json:"functionRef,omitempty"`

	// FunctionArn is a direct Lambda function ARN or name.
	// +optional
	FunctionArn string `json:"functionArn,omitempty"`

	// StatementId is a unique identifier for this permission statement.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="statementId is immutable"
	StatementId string `json:"statementId"`

	// Action is the Lambda API action that the principal can use (e.g. lambda:InvokeFunction).
	// +kubebuilder:validation:MinLength=1
	Action string `json:"action"`

	// Principal is the AWS service or account that is being granted permission (e.g. sns.amazonaws.com).
	// +kubebuilder:validation:MinLength=1
	Principal string `json:"principal"`

	// SourceArn restricts which resource the principal can invoke the function from.
	// +optional
	SourceArn string `json:"sourceArn,omitempty"`

	// SourceAccount restricts permissions to resources owned by this account.
	// +optional
	SourceAccount string `json:"sourceAccount,omitempty"`
}

// LambdaPermissionStatus defines the observed state of LambdaPermission.
type LambdaPermissionStatus struct {
	// StatementExists is true when the permission statement is confirmed in AWS.
	// +optional
	StatementExists bool `json:"statementExists,omitempty"`

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
// +kubebuilder:printcolumn:name="StatementId",type="string",JSONPath=".spec.statementId"
// +kubebuilder:printcolumn:name="Principal",type="string",JSONPath=".spec.principal"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// LambdaPermission is the Schema for managing Lambda resource-based policy statements.
type LambdaPermission struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LambdaPermissionSpec   `json:"spec,omitempty"`
	Status LambdaPermissionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LambdaPermissionList contains a list of LambdaPermission.
type LambdaPermissionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LambdaPermission `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LambdaPermission{}, &LambdaPermissionList{})
}
