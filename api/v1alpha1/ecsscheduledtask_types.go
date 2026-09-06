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

// ECSTaskNetworkConfiguration defines the network configuration for the ECS task target.
type ECSTaskNetworkConfiguration struct {
	// Subnets are the subnet IDs for the task.
	Subnets []string `json:"subnets"`

	// SecurityGroups are the security group IDs for the task.
	// +optional
	SecurityGroups []string `json:"securityGroups,omitempty"`

	// AssignPublicIP determines whether to assign a public IP.
	// +kubebuilder:validation:Enum=ENABLED;DISABLED
	// +optional
	AssignPublicIP string `json:"assignPublicIp,omitempty"`
}

// ECSScheduledTaskTarget defines the ECS task to run.
type ECSScheduledTaskTarget struct {
	// ClusterARN is the ARN of the ECS cluster.
	ClusterARN string `json:"clusterArn"`

	// TaskDefinitionARN is the ARN of the ECS task definition.
	TaskDefinitionARN string `json:"taskDefinitionArn"`

	// LaunchType is the launch type to use (FARGATE or EC2).
	// +kubebuilder:validation:Enum=FARGATE;EC2;EXTERNAL
	// +optional
	LaunchType string `json:"launchType,omitempty"`

	// TaskCount is the number of tasks to run (defaults to 1).
	// +optional
	TaskCount *int32 `json:"taskCount,omitempty"`

	// NetworkConfiguration is the network configuration for the task.
	// +optional
	NetworkConfiguration *ECSTaskNetworkConfiguration `json:"networkConfiguration,omitempty"`

	// PlatformVersion is the Fargate platform version (e.g., "LATEST").
	// +optional
	PlatformVersion string `json:"platformVersion,omitempty"`
}

// ECSScheduledTaskSpec defines the desired state of an ECS Scheduled Task.
type ECSScheduledTaskSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// RuleName is the name for the EventBridge rule.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="ruleName is immutable"
	RuleName string `json:"ruleName"`

	// ScheduleExpression is the schedule expression (e.g., "rate(5 minutes)" or "cron(0 12 * * ? *)").
	ScheduleExpression string `json:"scheduleExpression"`

	// EventBusName is the event bus name or ARN (defaults to default bus).
	// +optional
	EventBusName string `json:"eventBusName,omitempty"`

	// Description is an optional description for the rule.
	// +optional
	Description string `json:"description,omitempty"`

	// RoleARN is the IAM role ARN used to invoke the ECS task.
	RoleARN string `json:"roleArn"`

	// Target is the ECS task target configuration.
	Target ECSScheduledTaskTarget `json:"target"`

	// Tags are metadata tags for the EventBridge rule.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ECSScheduledTaskStatus defines the observed state of ECSScheduledTask.
type ECSScheduledTaskStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// RuleARN is the ARN of the EventBridge rule.
	// +optional
	RuleARN string `json:"ruleArn,omitempty"`

	// TargetID is the ID of the EventBridge rule target.
	// +optional
	TargetID string `json:"targetId,omitempty"`

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
// +kubebuilder:printcolumn:name="Rule",type="string",JSONPath=".spec.ruleName"
// +kubebuilder:printcolumn:name="Schedule",type="string",JSONPath=".spec.scheduleExpression"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ECSScheduledTask is the Schema for managing ECS scheduled tasks via EventBridge rules.
type ECSScheduledTask struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ECSScheduledTaskSpec   `json:"spec,omitempty"`
	Status ECSScheduledTaskStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ECSScheduledTaskList contains a list of ECSScheduledTask.
type ECSScheduledTaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ECSScheduledTask `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ECSScheduledTask{}, &ECSScheduledTaskList{})
}
