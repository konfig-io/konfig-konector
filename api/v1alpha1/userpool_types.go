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

// UserPoolPasswordPolicy defines the password policy for a user pool.
type UserPoolPasswordPolicy struct {
	// MinimumLength is the minimum password length.
	// +kubebuilder:validation:Minimum=6
	// +kubebuilder:validation:Maximum=99
	// +optional
	MinimumLength int32 `json:"minimumLength,omitempty"`
	// RequireUppercase requires uppercase letters.
	// +optional
	RequireUppercase bool `json:"requireUppercase,omitempty"`
	// RequireLowercase requires lowercase letters.
	// +optional
	RequireLowercase bool `json:"requireLowercase,omitempty"`
	// RequireNumbers requires numbers.
	// +optional
	RequireNumbers bool `json:"requireNumbers,omitempty"`
	// RequireSymbols requires symbols.
	// +optional
	RequireSymbols bool `json:"requireSymbols,omitempty"`
}

// UserPoolSpec defines the desired state of a Cognito User Pool.
type UserPoolSpec struct {
	// PoolName is the name of the user pool. Immutable.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	PoolName string `json:"poolName"`

	// PasswordPolicy defines the password policy.
	// +optional
	PasswordPolicy *UserPoolPasswordPolicy `json:"passwordPolicy,omitempty"`

	// AutoVerifiedAttributes are the attributes to auto-verify (email, phone_number).
	// +optional
	AutoVerifiedAttributes []string `json:"autoVerifiedAttributes,omitempty"`

	// MfaConfiguration sets MFA.
	// +kubebuilder:validation:Enum=OFF;ON;OPTIONAL
	// +optional
	MfaConfiguration string `json:"mfaConfiguration,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// UserPoolStatus defines the observed state of UserPool.
type UserPoolStatus struct {
	// UserPoolID is the AWS Cognito user pool ID.
	// +optional
	UserPoolID string `json:"userPoolId,omitempty"`

	// ARN is the ARN of the user pool.
	// +optional
	ARN string `json:"arn,omitempty"`

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
// +kubebuilder:printcolumn:name="Pool-ID",type="string",JSONPath=".status.userPoolId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// UserPool is the Schema for managing Cognito User Pools.
type UserPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UserPoolSpec   `json:"spec,omitempty"`
	Status UserPoolStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// UserPoolList contains a list of UserPool
type UserPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UserPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&UserPool{}, &UserPoolList{})
}
