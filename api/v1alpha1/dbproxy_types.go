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

// DBProxyAuthConfig defines an authentication configuration for an RDS Proxy.
type DBProxyAuthConfig struct {
	// Description is an optional description of this auth config.
	// +optional
	Description string `json:"description,omitempty"`

	// IAMAuth enables or disables IAM authentication.
	// +kubebuilder:validation:Enum=DISABLED;REQUIRED;ALLOWED
	// +optional
	IAMAuth string `json:"iamAuth,omitempty"`

	// SecretARN is the ARN of the Secrets Manager secret holding the credentials.
	// +optional
	SecretARN string `json:"secretArn,omitempty"`

	// AuthScheme is the authentication scheme (SECRETS).
	// +optional
	AuthScheme string `json:"authScheme,omitempty"`
}

// DBProxySpec defines the desired state of an RDS Proxy.
type DBProxySpec struct {
	// DBProxyName is the name of the proxy. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="dbProxyName is immutable"
	DBProxyName string `json:"dbProxyName"`

	// EngineFamily is the database engine family (MYSQL or POSTGRESQL).
	// +kubebuilder:validation:Enum=MYSQL;POSTGRESQL;SQLSERVER
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="engineFamily is immutable"
	EngineFamily string `json:"engineFamily"`

	// Auth is the authorization configuration for the proxy.
	// +kubebuilder:validation:MinItems=1
	Auth []DBProxyAuthConfig `json:"auth"`

	// RoleARN is the ARN of the IAM role the proxy uses to access Secrets Manager.
	// +kubebuilder:validation:MinLength=1
	RoleARN string `json:"roleArn"`

	// VPCSubnetIDs are the VPC subnet IDs for the proxy.
	// +kubebuilder:validation:MinItems=1
	VPCSubnetIDs []string `json:"vpcSubnetIds"`

	// VPCSecurityGroupIDs are optional security groups for the proxy.
	// +optional
	VPCSecurityGroupIDs []string `json:"vpcSecurityGroupIds,omitempty"`

	// RequireTLS requires TLS for all connections to the proxy.
	// +optional
	RequireTLS bool `json:"requireTls,omitempty"`

	// IdleClientTimeout is the number of seconds a connection can be inactive before being closed.
	// +optional
	IdleClientTimeout int32 `json:"idleClientTimeout,omitempty"`

	// DebugLogging enables enhanced logging for the proxy.
	// +optional
	DebugLogging bool `json:"debugLogging,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// DBProxyStatus defines the observed state of DBProxy.
type DBProxyStatus struct {
	// DBProxyARN is the ARN of the RDS proxy.
	// +optional
	DBProxyARN string `json:"dbProxyArn,omitempty"`

	// Endpoint is the proxy endpoint for client connections.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// Status is the current state of the proxy.
	// +optional
	Status string `json:"status,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.dbProxyArn"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DBProxy is the Schema for managing RDS DB Proxies.
type DBProxy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DBProxySpec   `json:"spec,omitempty"`
	Status DBProxyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DBProxyList contains a list of DBProxy.
type DBProxyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DBProxy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DBProxy{}, &DBProxyList{})
}
