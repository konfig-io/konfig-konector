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

// SQSRedrivePolicy configures a dead-letter queue for an SQS queue.
type SQSRedrivePolicy struct {
	// DeadLetterQueueRef is the name of an SQSQueue CR in the same namespace.
	DeadLetterQueueRef string `json:"deadLetterQueueRef"`

	// MaxReceiveCount is the number of times a message can be received before
	// it is sent to the dead-letter queue.
	// +kubebuilder:validation:Minimum=1
	MaxReceiveCount int32 `json:"maxReceiveCount"`
}

// SQSQueueSpec defines the desired state of an SQS Queue.
type SQSQueueSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// QueueName is the name of the queue. Immutable after creation.
	// FIFO queues must end with ".fifo".
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=80
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="queueName is immutable"
	QueueName string `json:"queueName"`

	// FIFO enables FIFO queue ordering and exactly-once processing.
	// +optional
	FIFO bool `json:"fifo,omitempty"`

	// VisibilityTimeout is the duration (seconds) that a received message is hidden.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=43200
	// +optional
	VisibilityTimeout int32 `json:"visibilityTimeout,omitempty"`

	// MessageRetentionPeriod is how long (seconds) messages are retained.
	// +kubebuilder:validation:Minimum=60
	// +kubebuilder:validation:Maximum=1209600
	// +optional
	MessageRetentionPeriod int32 `json:"messageRetentionPeriod,omitempty"`

	// DelaySeconds is the time (seconds) messages are delayed before delivery.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=900
	// +optional
	DelaySeconds int32 `json:"delaySeconds,omitempty"`

	// ReceiveMessageWaitTime enables long polling when > 0 (seconds).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=20
	// +optional
	ReceiveMessageWaitTime int32 `json:"receiveMessageWaitTime,omitempty"`

	// KMSKeyID is the KMS key ARN or alias for SSE-KMS encryption.
	// +optional
	KMSKeyID string `json:"kmsKeyId,omitempty"`

	// Policy is a JSON resource policy to attach to the queue.
	// +optional
	Policy string `json:"policy,omitempty"`

	// RedrivePolicy configures a dead-letter queue.
	// +optional
	RedrivePolicy *SQSRedrivePolicy `json:"redrivePolicy,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// SQSQueueStatus defines the observed state of SQSQueue.
type SQSQueueStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// QueueURL is the URL of the SQS queue.
	// +optional
	QueueURL string `json:"queueUrl,omitempty"`

	// QueueARN is the ARN of the SQS queue.
	// +optional
	QueueARN string `json:"queueArn,omitempty"`

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
// +kubebuilder:printcolumn:name="Queue-ARN",type="string",JSONPath=".status.queueArn"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SQSQueue is the Schema for managing SQS Queues.
type SQSQueue struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SQSQueueSpec   `json:"spec,omitempty"`
	Status SQSQueueStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SQSQueueList contains a list of SQSQueue
type SQSQueueList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SQSQueue `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SQSQueue{}, &SQSQueueList{})
}
