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

// GluePhysicalConnectionRequirements specifies the VPC placement of a
// connection.
type GluePhysicalConnectionRequirements struct {
	// SubnetRef references the subnet the connection uses.
	// +optional
	SubnetRef *SubnetRef `json:"subnetRef,omitempty"`

	// SecurityGroupRefs reference the security groups the connection uses.
	// +optional
	SecurityGroupRefs []SecurityGroupRef `json:"securityGroupRefs,omitempty"`

	// AvailabilityZone is the connection's Availability Zone.
	// +optional
	AvailabilityZone string `json:"availabilityZone,omitempty"`
}

// GlueConnectionSpec defines the desired state of a Glue connection.
type GlueConnectionSpec struct {
	// Name is the name of the connection. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// ConnectionType is the type of connection.
	// +kubebuilder:validation:Enum=JDBC;KAFKA;MONGODB;NETWORK;MARKETPLACE;CUSTOM
	ConnectionType string `json:"connectionType"`

	// ConnectionProperties are key-value pairs for the connection (HOST,
	// PORT, USERNAME, JDBC_CONNECTION_URL, ...). The PASSWORD property must
	// NOT be set here; use PasswordSecretRef instead so the secret value
	// never appears in the CR.
	// +optional
	ConnectionProperties map[string]string `json:"connectionProperties,omitempty"`

	// PasswordSecretRef references a Kubernetes Secret holding the value of
	// the PASSWORD connection property. Never place the password inline in
	// ConnectionProperties.
	// +optional
	PasswordSecretRef *SecretRef `json:"passwordSecretRef,omitempty"`

	// PhysicalConnectionRequirements specifies VPC placement for the
	// connection.
	// +optional
	PhysicalConnectionRequirements *GluePhysicalConnectionRequirements `json:"physicalConnectionRequirements,omitempty"`

	// Description is a description of the connection.
	// +optional
	Description string `json:"description,omitempty"`

	// Tags are AWS resource tags to apply at creation.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// GlueConnectionStatus defines the observed state of GlueConnection.
type GlueConnectionStatus struct {
	// ConnectionName is the name of the connection in AWS.
	// +optional
	ConnectionName string `json:"connectionName,omitempty"`

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
// +kubebuilder:printcolumn:name="Connection",type="string",JSONPath=".status.connectionName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// GlueConnection is the Schema for managing Glue connections.
type GlueConnection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GlueConnectionSpec   `json:"spec,omitempty"`
	Status GlueConnectionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GlueConnectionList contains a list of GlueConnection
type GlueConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GlueConnection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GlueConnection{}, &GlueConnectionList{})
}
