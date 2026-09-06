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

// GlueTriggerAction is one action initiated by a trigger.
type GlueTriggerAction struct {
	// JobName is the name of the Glue job to run.
	// +kubebuilder:validation:MinLength=1
	JobName string `json:"jobName"`

	// Arguments are job arguments that replace the job's default arguments
	// for this run.
	// +optional
	Arguments map[string]string `json:"arguments,omitempty"`
}

// GlueTriggerCondition is one condition of a CONDITIONAL trigger's predicate.
type GlueTriggerCondition struct {
	// JobName is the job whose runs this condition applies to.
	// +optional
	JobName string `json:"jobName,omitempty"`

	// CrawlerName is the crawler this condition applies to.
	// +optional
	CrawlerName string `json:"crawlerName,omitempty"`

	// State is the job/crawler state the trigger listens for
	// (SUCCEEDED, STOPPED, FAILED, TIMEOUT for jobs).
	// +optional
	State string `json:"state,omitempty"`

	// LogicalOperator is the comparison operator; only EQUALS is supported.
	// +kubebuilder:validation:Enum=EQUALS
	// +optional
	LogicalOperator string `json:"logicalOperator,omitempty"`
}

// GlueTriggerPredicate defines when a CONDITIONAL trigger fires.
type GlueTriggerPredicate struct {
	// Logical is how multiple conditions combine: AND or ANY.
	// +kubebuilder:validation:Enum=AND;ANY
	// +optional
	Logical string `json:"logical,omitempty"`

	// Conditions determine when the trigger fires.
	// +optional
	Conditions []GlueTriggerCondition `json:"conditions,omitempty"`
}

// GlueTriggerSpec defines the desired state of a Glue trigger.
type GlueTriggerSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the name of the trigger. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Type is the trigger type. Immutable after creation.
	// +kubebuilder:validation:Enum=SCHEDULED;CONDITIONAL;ON_DEMAND
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="type is immutable"
	Type string `json:"type"`

	// Schedule is a cron expression for SCHEDULED triggers,
	// e.g. cron(15 12 * * ? *).
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// Actions are initiated when the trigger fires.
	// +kubebuilder:validation:MinItems=1
	Actions []GlueTriggerAction `json:"actions"`

	// Predicate defines when a CONDITIONAL trigger fires.
	// +optional
	Predicate *GlueTriggerPredicate `json:"predicate,omitempty"`

	// StartOnCreation activates SCHEDULED/CONDITIONAL triggers when created.
	// +optional
	StartOnCreation bool `json:"startOnCreation,omitempty"`

	// Description is a description of the trigger.
	// +optional
	Description string `json:"description,omitempty"`

	// Tags are AWS resource tags to apply at creation.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// GlueTriggerStatus defines the observed state of GlueTrigger.
type GlueTriggerStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// TriggerName is the name of the trigger in AWS.
	// +optional
	TriggerName string `json:"triggerName,omitempty"`

	// State is the current trigger state reported by AWS.
	// +optional
	State string `json:"state,omitempty"`

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
// +kubebuilder:printcolumn:name="Trigger",type="string",JSONPath=".status.triggerName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// GlueTrigger is the Schema for managing Glue triggers.
type GlueTrigger struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GlueTriggerSpec   `json:"spec,omitempty"`
	Status GlueTriggerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GlueTriggerList contains a list of GlueTrigger
type GlueTriggerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GlueTrigger `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GlueTrigger{}, &GlueTriggerList{})
}
