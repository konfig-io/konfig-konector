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

// DynamoDBTablePolicySpec defines the desired state of a DynamoDB resource policy.
type DynamoDBTablePolicySpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ResourceARN is the ARN of the DynamoDB table or stream to attach the policy to.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="resourceARN is immutable"
	ResourceARN string `json:"resourceARN"`

	// PolicyDocument is the JSON policy document.
	PolicyDocument string `json:"policyDocument"`
}

// DynamoDBTablePolicyStatus defines the observed state of DynamoDBTablePolicy.
type DynamoDBTablePolicyStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// RevisionID is the policy revision ID.
	// +optional
	RevisionID string `json:"revisionID,omitempty"`

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
// +kubebuilder:printcolumn:name="RevisionID",type="string",JSONPath=".status.revisionID"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DynamoDBTablePolicy is the Schema for managing DynamoDB resource policies.
type DynamoDBTablePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DynamoDBTablePolicySpec   `json:"spec,omitempty"`
	Status DynamoDBTablePolicyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DynamoDBTablePolicyList contains a list of DynamoDBTablePolicy.
type DynamoDBTablePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DynamoDBTablePolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DynamoDBTablePolicy{}, &DynamoDBTablePolicyList{})
}
