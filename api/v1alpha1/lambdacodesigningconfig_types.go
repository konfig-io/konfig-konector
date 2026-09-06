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

// LambdaCodeSigningConfigSpec defines the desired state of a Lambda Code Signing Config.
type LambdaCodeSigningConfigSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// AllowedPublisherARNs are the signing profile version ARNs.
	// +kubebuilder:validation:MinItems=1
	AllowedPublisherARNs []string `json:"allowedPublisherArns"`

	// Description is an optional description.
	// +optional
	Description string `json:"description,omitempty"`

	// UntrustedArtifactOnDeployment controls behavior when deployment validation fails.
	// +kubebuilder:validation:Enum=Warn;Enforce
	// +optional
	UntrustedArtifactOnDeployment string `json:"untrustedArtifactOnDeployment,omitempty"`
}

// LambdaCodeSigningConfigStatus defines the observed state of LambdaCodeSigningConfig.
type LambdaCodeSigningConfigStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// CodeSigningConfigARN is the ARN of the code signing config.
	// +optional
	CodeSigningConfigARN string `json:"codeSigningConfigArn,omitempty"`

	// CodeSigningConfigID is the ID of the code signing config.
	// +optional
	CodeSigningConfigID string `json:"codeSigningConfigId,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.codeSigningConfigArn"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// LambdaCodeSigningConfig is the Schema for managing Lambda code signing configurations.
type LambdaCodeSigningConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LambdaCodeSigningConfigSpec   `json:"spec,omitempty"`
	Status LambdaCodeSigningConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// LambdaCodeSigningConfigList contains a list of LambdaCodeSigningConfig.
type LambdaCodeSigningConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LambdaCodeSigningConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LambdaCodeSigningConfig{}, &LambdaCodeSigningConfigList{})
}
