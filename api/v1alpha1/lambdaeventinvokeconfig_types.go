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

// LambdaEventInvokeConfigSpec defines the desired state of a Lambda function
// event invoke config (async invocation settings).
type LambdaEventInvokeConfigSpec struct {
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

	// Qualifier is a version number or alias name. Defaults to $LATEST.
	// +optional
	Qualifier string `json:"qualifier,omitempty"`

	// MaximumRetryAttempts for async invocation (0-2).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=2
	// +optional
	MaximumRetryAttempts *int32 `json:"maximumRetryAttempts,omitempty"`

	// MaximumEventAgeInSeconds is the maximum age of an async event (60-21600).
	// +kubebuilder:validation:Minimum=60
	// +kubebuilder:validation:Maximum=21600
	// +optional
	MaximumEventAgeInSeconds *int32 `json:"maximumEventAgeInSeconds,omitempty"`

	// OnSuccessDestinationARN receives records of successful async invocations
	// (SQS, SNS, Lambda, or EventBridge ARN).
	// +optional
	OnSuccessDestinationARN string `json:"onSuccessDestinationArn,omitempty"`

	// OnFailureDestinationARN receives records of failed async invocations.
	// +optional
	OnFailureDestinationARN string `json:"onFailureDestinationArn,omitempty"`
}

// LambdaEventInvokeConfigStatus defines the observed state of LambdaEventInvokeConfig.
type LambdaEventInvokeConfigStatus struct {
	// FunctionARN is the ARN of the function (with qualifier) the config applies to.
	// +optional
	FunctionARN string `json:"functionArn,omitempty"`

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
// +kubebuilder:printcolumn:name="Function-ARN",type="string",JSONPath=".status.functionArn"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// LambdaEventInvokeConfig is the Schema for managing Lambda event invoke configs.
type LambdaEventInvokeConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LambdaEventInvokeConfigSpec   `json:"spec,omitempty"`
	Status LambdaEventInvokeConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// LambdaEventInvokeConfigList contains a list of LambdaEventInvokeConfig
type LambdaEventInvokeConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LambdaEventInvokeConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LambdaEventInvokeConfig{}, &LambdaEventInvokeConfigList{})
}
