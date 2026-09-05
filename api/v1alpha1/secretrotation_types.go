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

// SecretsManagerSecretRef references either a managed Secret CR or a direct secret ARN/name.
type SecretsManagerSecretRef struct {
	// Name of a Secret CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// SecretARN is a direct AWS Secrets Manager secret ARN or name.
	// +optional
	SecretARN string `json:"secretArn,omitempty"`
}

// SecretRotationSpec defines the desired state of a Secret Rotation configuration.
type SecretRotationSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// SecretRef references the secret to rotate.
	SecretRef SecretsManagerSecretRef `json:"secretRef"`

	// RotationLambdaARN is the ARN of the Lambda function that rotates the secret.
	RotationLambdaARN string `json:"rotationLambdaArn"`

	// AutomaticallyAfterDays is the rotation interval in days.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=365
	AutomaticallyAfterDays int32 `json:"automaticallyAfterDays"`
}

// SecretRotationStatus defines the observed state of SecretRotation.
type SecretRotationStatus struct {
	// RotationEnabled indicates whether rotation is enabled.
	// +optional
	RotationEnabled bool `json:"rotationEnabled,omitempty"`

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
// +kubebuilder:printcolumn:name="Rotation-Enabled",type="boolean",JSONPath=".status.rotationEnabled"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SecretRotation is the Schema for managing Secrets Manager rotation configuration.
type SecretRotation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SecretRotationSpec   `json:"spec,omitempty"`
	Status SecretRotationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SecretRotationList contains a list of SecretRotation
type SecretRotationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SecretRotation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SecretRotation{}, &SecretRotationList{})
}
