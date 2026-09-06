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

// ConfigRuleScope constrains which resources trigger evaluations for a rule.
type ConfigRuleScope struct {
	// ComplianceResourceTypes are the resource types that trigger evaluation
	// (e.g. AWS::EC2::Instance).
	// +optional
	ComplianceResourceTypes []string `json:"complianceResourceTypes,omitempty"`

	// TagKey triggers evaluation for resources with this tag key.
	// +optional
	TagKey string `json:"tagKey,omitempty"`

	// TagValue triggers evaluation for resources with this tag value.
	// Requires TagKey.
	// +optional
	TagValue string `json:"tagValue,omitempty"`
}

// ConfigRuleSpec defines the desired state of an AWS Config rule.
type ConfigRuleSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// RuleName is the name of the Config rule. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="ruleName is immutable"
	RuleName string `json:"ruleName"`

	// Description of the rule.
	// +optional
	Description string `json:"description,omitempty"`

	// SourceOwner indicates who owns the rule: AWS for managed rules,
	// CUSTOM_LAMBDA for custom Lambda rules.
	// +kubebuilder:validation:Enum=AWS;CUSTOM_LAMBDA
	SourceOwner string `json:"sourceOwner"`

	// SourceIdentifier is the managed rule identifier (e.g.
	// IAM_PASSWORD_POLICY) or, for CUSTOM_LAMBDA, the Lambda function ARN.
	// +kubebuilder:validation:MinLength=1
	SourceIdentifier string `json:"sourceIdentifier"`

	// InputParameters is a JSON string passed to the rule.
	// +optional
	InputParameters string `json:"inputParameters,omitempty"`

	// Scope constrains which resources trigger an evaluation.
	// +optional
	Scope *ConfigRuleScope `json:"scope,omitempty"`

	// MaximumExecutionFrequency is the maximum frequency of evaluations for
	// periodic rules.
	// +kubebuilder:validation:Enum=One_Hour;Three_Hours;Six_Hours;Twelve_Hours;TwentyFour_Hours
	// +optional
	MaximumExecutionFrequency string `json:"maximumExecutionFrequency,omitempty"`
}

// ConfigRuleStatus defines the observed state of ConfigRule.
type ConfigRuleStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// RuleARN is the ARN of the Config rule.
	// +optional
	RuleARN string `json:"ruleArn,omitempty"`

	// RuleID is the ID of the Config rule.
	// +optional
	RuleID string `json:"ruleId,omitempty"`

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
// +kubebuilder:printcolumn:name="Rule-ARN",type="string",JSONPath=".status.ruleArn"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ConfigRule is the Schema for managing AWS Config rules.
type ConfigRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfigRuleSpec   `json:"spec,omitempty"`
	Status ConfigRuleStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ConfigRuleList contains a list of ConfigRule
type ConfigRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConfigRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConfigRule{}, &ConfigRuleList{})
}
