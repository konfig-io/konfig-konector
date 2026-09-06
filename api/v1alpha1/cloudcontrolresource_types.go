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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CloudControlResourceSpec manages any resource type supported by the AWS
// Cloud Control API (every CloudFormation "AWS::Service::Type" with a
// registered handler — over a thousand types). It is the escape hatch for
// services that have no native kind yet: the same finalizer, drift-correction,
// deletion-policy and multi-account semantics apply.
type CloudControlResourceSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// TypeName is the CloudFormation resource type, e.g. AWS::Logs::LogGroup.
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9]{2,64}::[A-Za-z0-9]{2,64}::[A-Za-z0-9]{2,64}$`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="typeName is immutable"
	TypeName string `json:"typeName"`

	// DesiredState is the resource's properties as a JSON object, using the
	// CloudFormation property names for the type. Properties that are
	// create-only in the type schema cannot be changed after creation.
	// +kubebuilder:pruning:PreserveUnknownFields
	DesiredState apiextensionsv1.JSON `json:"desiredState"`

	// Identifier adopts an existing resource by its Cloud Control primary
	// identifier instead of creating a new one.
	// +optional
	Identifier string `json:"identifier,omitempty"`

	// RoleARN is an IAM role Cloud Control assumes to perform the operation,
	// instead of the caller's credentials.
	// +optional
	RoleARN string `json:"roleArn,omitempty"`

	// TypeVersionID pins a specific registered version of the type.
	// +optional
	TypeVersionID string `json:"typeVersionId,omitempty"`
}

// CloudControlResourceStatus defines the observed state of CloudControlResource.
type CloudControlResourceStatus struct {
	// Identifier is the Cloud Control primary identifier of the resource.
	// +optional
	Identifier string `json:"identifier,omitempty"`
	// RequestToken tracks an in-flight asynchronous operation.
	// +optional
	RequestToken string `json:"requestToken,omitempty"`
	// Operation is the in-flight operation (CREATE, UPDATE, DELETE).
	// +optional
	Operation string `json:"operation,omitempty"`
	// OperationStatus is the last reported operation status.
	// +optional
	OperationStatus string `json:"operationStatus,omitempty"`
	// Properties is the live resource model as returned by GetResource.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Properties *apiextensionsv1.JSON `json:"properties,omitempty"`
	// Conditions describe the current state of the resource.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ccr
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.typeName"
// +kubebuilder:printcolumn:name="Identifier",type="string",JSONPath=".status.identifier"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CloudControlResource manages any AWS Cloud Control API resource type.
type CloudControlResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudControlResourceSpec   `json:"spec,omitempty"`
	Status CloudControlResourceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloudControlResourceList contains a list of CloudControlResource.
type CloudControlResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudControlResource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudControlResource{}, &CloudControlResourceList{})
}
