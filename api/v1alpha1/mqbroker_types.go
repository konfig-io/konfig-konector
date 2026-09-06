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

// MQUser defines a broker user. The password is always read from a
// Kubernetes Secret and never stored in the CR.
type MQUser struct {
	// Username of the broker user.
	// +kubebuilder:validation:MinLength=2
	// +kubebuilder:validation:MaxLength=100
	Username string `json:"username"`

	// PasswordRef references the Kubernetes Secret holding the user's
	// password. The password is never written to status or exported.
	PasswordRef SecretRef `json:"passwordRef"`

	// ConsoleAccess enables access to the ActiveMQ Web Console.
	// Does not apply to RabbitMQ brokers.
	// +optional
	ConsoleAccess bool `json:"consoleAccess,omitempty"`

	// Groups is the list of ActiveMQ groups the user belongs to.
	// Does not apply to RabbitMQ brokers.
	// +optional
	Groups []string `json:"groups,omitempty"`
}

// MQBrokerSpec defines the desired state of an Amazon MQ broker.
type MQBrokerSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// BrokerName is the name of the broker. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=50
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="brokerName is immutable"
	BrokerName string `json:"brokerName"`

	// EngineType is the broker engine. Immutable after creation.
	// +kubebuilder:validation:Enum=ACTIVEMQ;RABBITMQ
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="engineType is immutable"
	EngineType string `json:"engineType"`

	// EngineVersion is the broker engine version (e.g. 5.17.6, 3.11.20).
	// +optional
	EngineVersion string `json:"engineVersion,omitempty"`

	// HostInstanceType is the broker instance type (e.g. mq.t3.micro).
	// +kubebuilder:validation:MinLength=1
	HostInstanceType string `json:"hostInstanceType"`

	// DeploymentMode is the broker deployment mode. Immutable after creation.
	// +kubebuilder:validation:Enum=SINGLE_INSTANCE;ACTIVE_STANDBY_MULTI_AZ;CLUSTER_MULTI_AZ
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="deploymentMode is immutable"
	DeploymentMode string `json:"deploymentMode"`

	// PubliclyAccessible enables connections from outside the VPC.
	// +optional
	PubliclyAccessible bool `json:"publiclyAccessible,omitempty"`

	// SubnetRefs are the subnets the broker is placed into.
	// +optional
	SubnetRefs []SubnetRef `json:"subnetRefs,omitempty"`

	// SecurityGroupRefs are the security groups authorizing connections.
	// +optional
	SecurityGroupRefs []SecurityGroupRef `json:"securityGroupRefs,omitempty"`

	// Users are the broker users created at provisioning time.
	// +optional
	Users []MQUser `json:"users,omitempty"`

	// AutoMinorVersionUpgrade enables automatic patch version upgrades.
	// +optional
	AutoMinorVersionUpgrade bool `json:"autoMinorVersionUpgrade,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// MQBrokerStatus defines the observed state of MQBroker.
type MQBrokerStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// BrokerID is the unique ID Amazon MQ generates for the broker.
	// +optional
	BrokerID string `json:"brokerId,omitempty"`

	// BrokerARN is the ARN of the broker.
	// +optional
	BrokerARN string `json:"brokerArn,omitempty"`

	// BrokerState is the current broker state (e.g. RUNNING).
	// +optional
	BrokerState string `json:"brokerState,omitempty"`

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
// +kubebuilder:printcolumn:name="Broker-ID",type="string",JSONPath=".status.brokerId"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.brokerState"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// MQBroker is the Schema for managing Amazon MQ brokers.
type MQBroker struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MQBrokerSpec   `json:"spec,omitempty"`
	Status MQBrokerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MQBrokerList contains a list of MQBroker
type MQBrokerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MQBroker `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MQBroker{}, &MQBrokerList{})
}
