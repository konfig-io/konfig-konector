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

// FlowLogSpec defines the desired state of a VPC Flow Log.
type FlowLogSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ResourceID is the ID of the VPC, subnet, or network interface to capture traffic for.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="resourceId is immutable"
	ResourceID string `json:"resourceId"`

	// ResourceType is the type of resource to log traffic for.
	// +kubebuilder:validation:Enum=VPC;Subnet;NetworkInterface
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="resourceType is immutable"
	ResourceType string `json:"resourceType"`

	// TrafficType specifies which traffic to capture.
	// +kubebuilder:validation:Enum=ACCEPT;REJECT;ALL
	// +optional
	TrafficType string `json:"trafficType,omitempty"`

	// LogDestinationType is the destination type for flow log data.
	// +kubebuilder:validation:Enum=cloud-watch-logs;s3;kinesis-data-firehose
	// +optional
	LogDestinationType string `json:"logDestinationType,omitempty"`

	// LogDestination is the ARN of the destination (CloudWatch log group, S3 bucket, or Firehose).
	// +optional
	LogDestination string `json:"logDestination,omitempty"`

	// LogGroupName is the CloudWatch log group name (used when LogDestinationType is cloud-watch-logs).
	// +optional
	LogGroupName string `json:"logGroupName,omitempty"`

	// DeliverLogsPermissionARN is the IAM role ARN for delivering logs to CloudWatch.
	// +optional
	DeliverLogsPermissionARN string `json:"deliverLogsPermissionArn,omitempty"`

	// LogFormat is a custom format string for flow log records.
	// +optional
	LogFormat string `json:"logFormat,omitempty"`

	// MaxAggregationInterval is the maximum interval in seconds to aggregate records (60 or 600).
	// +optional
	MaxAggregationInterval int32 `json:"maxAggregationInterval,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// FlowLogStatus defines the observed state of FlowLog.
type FlowLogStatus struct {
	// FlowLogID is the ID of the flow log.
	// +optional
	FlowLogID string `json:"flowLogId,omitempty"`

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
// +kubebuilder:printcolumn:name="FlowLogID",type="string",JSONPath=".status.flowLogId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// FlowLog is the Schema for managing VPC Flow Logs.
type FlowLog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FlowLogSpec   `json:"spec,omitempty"`
	Status FlowLogStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// FlowLogList contains a list of FlowLog.
type FlowLogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FlowLog `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FlowLog{}, &FlowLogList{})
}
