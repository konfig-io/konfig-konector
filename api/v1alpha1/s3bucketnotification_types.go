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

// S3LambdaNotificationConfig defines a Lambda function notification.
type S3LambdaNotificationConfig struct {
	// ID is an optional unique identifier for this configuration.
	// +optional
	ID string `json:"id,omitempty"`

	// LambdaFunctionARN is the ARN of the Lambda function.
	// +kubebuilder:validation:MinLength=1
	LambdaFunctionARN string `json:"lambdaFunctionArn"`

	// Events is the list of S3 events that trigger this notification.
	// +kubebuilder:validation:MinItems=1
	Events []string `json:"events"`
}

// S3TopicNotificationConfig defines an SNS topic notification.
type S3TopicNotificationConfig struct {
	// ID is an optional unique identifier for this configuration.
	// +optional
	ID string `json:"id,omitempty"`

	// TopicARN is the ARN of the SNS topic.
	// +kubebuilder:validation:MinLength=1
	TopicARN string `json:"topicArn"`

	// Events is the list of S3 events that trigger this notification.
	// +kubebuilder:validation:MinItems=1
	Events []string `json:"events"`
}

// S3QueueNotificationConfig defines an SQS queue notification.
type S3QueueNotificationConfig struct {
	// ID is an optional unique identifier for this configuration.
	// +optional
	ID string `json:"id,omitempty"`

	// QueueARN is the ARN of the SQS queue.
	// +kubebuilder:validation:MinLength=1
	QueueARN string `json:"queueArn"`

	// Events is the list of S3 events that trigger this notification.
	// +kubebuilder:validation:MinItems=1
	Events []string `json:"events"`
}

// S3BucketNotificationSpec defines the desired state of S3BucketNotification.
type S3BucketNotificationSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// BucketName is the name of the S3 bucket.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="bucketName is immutable"
	BucketName string `json:"bucketName"`

	// LambdaFunctionConfigurations are Lambda function notification configurations.
	// +optional
	LambdaFunctionConfigurations []S3LambdaNotificationConfig `json:"lambdaFunctionConfigurations,omitempty"`

	// TopicConfigurations are SNS topic notification configurations.
	// +optional
	TopicConfigurations []S3TopicNotificationConfig `json:"topicConfigurations,omitempty"`

	// QueueConfigurations are SQS queue notification configurations.
	// +optional
	QueueConfigurations []S3QueueNotificationConfig `json:"queueConfigurations,omitempty"`

	// EventBridgeEnabled enables delivery of all events to Amazon EventBridge.
	// +optional
	EventBridgeEnabled bool `json:"eventBridgeEnabled,omitempty"`
}

// S3BucketNotificationStatus defines the observed state of S3BucketNotification.
type S3BucketNotificationStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
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
// +kubebuilder:printcolumn:name="Bucket",type="string",JSONPath=".spec.bucketName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// S3BucketNotification is the Schema for managing S3 bucket notification configurations.
type S3BucketNotification struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   S3BucketNotificationSpec   `json:"spec,omitempty"`
	Status S3BucketNotificationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// S3BucketNotificationList contains a list of S3BucketNotification.
type S3BucketNotificationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []S3BucketNotification `json:"items"`
}

func init() {
	SchemeBuilder.Register(&S3BucketNotification{}, &S3BucketNotificationList{})
}
