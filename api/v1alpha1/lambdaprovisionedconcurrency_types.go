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

// LambdaProvisionedConcurrencySpec defines the desired state of a Lambda
// provisioned concurrency config.
type LambdaProvisionedConcurrencySpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// FunctionName is a direct Lambda function name or ARN. Either
	// functionName or functionRef must be set.
	// +optional
	FunctionName string `json:"functionName,omitempty"`

	// FunctionRef references a LambdaFunction CR.
	// +optional
	FunctionRef *LambdaFunctionRef `json:"functionRef,omitempty"`

	// Qualifier is the version number or alias name the config applies to.
	// Provisioned concurrency cannot be set on $LATEST.
	// +kubebuilder:validation:MinLength=1
	Qualifier string `json:"qualifier"`

	// ProvisionedConcurrentExecutions is the amount of provisioned concurrency.
	// +kubebuilder:validation:Minimum=1
	ProvisionedConcurrentExecutions int32 `json:"provisionedConcurrentExecutions"`
}

// LambdaProvisionedConcurrencyStatus defines the observed state of
// LambdaProvisionedConcurrency.
type LambdaProvisionedConcurrencyStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// FunctionName is the resolved function name or ARN the config was applied to.
	// +optional
	FunctionName string `json:"functionName,omitempty"`

	// AllocatedConcurrentExecutions is the amount currently allocated.
	// +optional
	AllocatedConcurrentExecutions int32 `json:"allocatedConcurrentExecutions,omitempty"`

	// Status is the provisioned concurrency status (IN_PROGRESS, READY, FAILED).
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
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// LambdaProvisionedConcurrency is the Schema for managing Lambda provisioned
// concurrency configs.
type LambdaProvisionedConcurrency struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LambdaProvisionedConcurrencySpec   `json:"spec,omitempty"`
	Status LambdaProvisionedConcurrencyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// LambdaProvisionedConcurrencyList contains a list of LambdaProvisionedConcurrency
type LambdaProvisionedConcurrencyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LambdaProvisionedConcurrency `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LambdaProvisionedConcurrency{}, &LambdaProvisionedConcurrencyList{})
}
