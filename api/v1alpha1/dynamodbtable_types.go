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

// DynamoDBAttributeDefinition defines a table attribute.
type DynamoDBAttributeDefinition struct {
	// AttributeName is the name of the attribute.
	AttributeName string `json:"attributeName"`
	// AttributeType is the data type (S=String, N=Number, B=Binary).
	// +kubebuilder:validation:Enum=S;N;B
	AttributeType string `json:"attributeType"`
}

// DynamoDBKeySchema defines the key schema for a table or index.
type DynamoDBKeySchema struct {
	// AttributeName is the name of the key attribute.
	AttributeName string `json:"attributeName"`
	// KeyType is HASH or RANGE.
	// +kubebuilder:validation:Enum=HASH;RANGE
	KeyType string `json:"keyType"`
}

// DynamoDBProvisionedThroughput defines read/write capacity.
type DynamoDBProvisionedThroughput struct {
	// ReadCapacityUnits is the read capacity.
	// +kubebuilder:validation:Minimum=1
	ReadCapacityUnits int64 `json:"readCapacityUnits"`
	// WriteCapacityUnits is the write capacity.
	// +kubebuilder:validation:Minimum=1
	WriteCapacityUnits int64 `json:"writeCapacityUnits"`
}

// DynamoDBTableSpec defines the desired state of a DynamoDB Table.
type DynamoDBTableSpec struct {
	// TableName is the name of the table. Immutable.
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="tableName is immutable"
	TableName string `json:"tableName"`

	// AttributeDefinitions define the table attributes.
	// +kubebuilder:validation:MinItems=1
	AttributeDefinitions []DynamoDBAttributeDefinition `json:"attributeDefinitions"`

	// KeySchema defines the primary key.
	// +kubebuilder:validation:MinItems=1
	KeySchema []DynamoDBKeySchema `json:"keySchema"`

	// BillingMode is the billing mode.
	// +kubebuilder:validation:Enum=PROVISIONED;PAY_PER_REQUEST
	// +optional
	BillingMode string `json:"billingMode,omitempty"`

	// ProvisionedThroughput is required when BillingMode is PROVISIONED.
	// +optional
	ProvisionedThroughput *DynamoDBProvisionedThroughput `json:"provisionedThroughput,omitempty"`

	// SSEEnabled enables server-side encryption.
	// +optional
	SSEEnabled bool `json:"sseEnabled,omitempty"`

	// KMSKeyARN is the KMS key ARN for SSE.
	// +optional
	KMSKeyARN string `json:"kmsKeyArn,omitempty"`

	// PointInTimeRecovery enables PITR.
	// +optional
	PointInTimeRecovery bool `json:"pointInTimeRecovery,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// DynamoDBTableStatus defines the observed state of DynamoDBTable.
type DynamoDBTableStatus struct {
	// ARN is the ARN of the DynamoDB table.
	// +optional
	ARN string `json:"arn,omitempty"`

	// TableStatus is the current table status.
	// +optional
	TableStatus string `json:"tableStatus,omitempty"`

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
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.tableStatus"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DynamoDBTable is the Schema for managing DynamoDB Tables.
type DynamoDBTable struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DynamoDBTableSpec   `json:"spec,omitempty"`
	Status DynamoDBTableStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DynamoDBTableList contains a list of DynamoDBTable
type DynamoDBTableList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DynamoDBTable `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DynamoDBTable{}, &DynamoDBTableList{})
}
