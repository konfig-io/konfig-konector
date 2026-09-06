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

// SNSTopicSpec defines the desired state of an SNS Topic.
type SNSTopicSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// TopicName is the name of the SNS topic. Immutable after creation.
	// FIFO topics must end with ".fifo".
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="topicName is immutable"
	TopicName string `json:"topicName"`

	// FIFO enables FIFO ordering and exactly-once delivery.
	// +optional
	FIFO bool `json:"fifo,omitempty"`

	// ContentBasedDeduplication enables deduplication based on message body
	// (only valid for FIFO topics).
	// +optional
	ContentBasedDeduplication bool `json:"contentBasedDeduplication,omitempty"`

	// KMSKeyID is the KMS key ARN or alias for SSE-KMS encryption.
	// +optional
	KMSKeyID string `json:"kmsKeyId,omitempty"`

	// Policy is a JSON resource policy for the topic.
	// +optional
	Policy string `json:"policy,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// SNSTopicStatus defines the observed state of SNSTopic.
type SNSTopicStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// TopicARN is the ARN of the SNS topic.
	// +optional
	TopicARN string `json:"topicArn,omitempty"`

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
// +kubebuilder:printcolumn:name="Topic-ARN",type="string",JSONPath=".status.topicArn"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SNSTopic is the Schema for managing SNS Topics.
type SNSTopic struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SNSTopicSpec   `json:"spec,omitempty"`
	Status SNSTopicStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SNSTopicList contains a list of SNSTopic
type SNSTopicList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SNSTopic `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SNSTopic{}, &SNSTopicList{})
}
