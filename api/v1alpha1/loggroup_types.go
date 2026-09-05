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

// LogGroupSpec defines the desired state of a CloudWatch Log Group.
type LogGroupSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// LogGroupName is the name of the log group. Immutable.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="logGroupName is immutable"
	LogGroupName string `json:"logGroupName"`

	// RetentionInDays sets the log retention period. 0 means never expire.
	// +kubebuilder:validation:Enum=0;1;3;5;7;14;30;60;90;120;150;180;365;400;545;731;1827;3653
	// +optional
	RetentionInDays int32 `json:"retentionInDays,omitempty"`

	// KMSKeyARN is the KMS key ARN for encrypting log data.
	// +optional
	KMSKeyARN string `json:"kmsKeyArn,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// LogGroupStatus defines the observed state of LogGroup.
type LogGroupStatus struct {
	// ARN is the ARN of the log group.
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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.arn"
// +kubebuilder:printcolumn:name="Retention",type="integer",JSONPath=".spec.retentionInDays"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// LogGroup is the Schema for managing CloudWatch Log Groups.
type LogGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LogGroupSpec   `json:"spec,omitempty"`
	Status LogGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// LogGroupList contains a list of LogGroup
type LogGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LogGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LogGroup{}, &LogGroupList{})
}
