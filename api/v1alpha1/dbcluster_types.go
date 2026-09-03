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

// RDSServerlessV2ScalingConfig defines capacity for Aurora Serverless v2.
type RDSServerlessV2ScalingConfig struct {
	// MinCapacity is the minimum Aurora capacity units (ACUs). Minimum 0.5.
	MinCapacity float64 `json:"minCapacity"`
	// MaxCapacity is the maximum Aurora capacity units (ACUs). Maximum 128.
	MaxCapacity float64 `json:"maxCapacity"`
}

// DBClusterSpec defines the desired state of an Aurora DB Cluster.
type DBClusterSpec struct {
	// DBClusterIdentifier is the unique name for the DB cluster. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="dbClusterIdentifier is immutable"
	DBClusterIdentifier string `json:"dbClusterIdentifier"`

	// Engine is the Aurora engine (aurora-mysql or aurora-postgresql).
	// +kubebuilder:validation:Enum=aurora-mysql;aurora-postgresql
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="engine is immutable"
	Engine string `json:"engine"`

	// EngineVersion is the version of the Aurora engine.
	// +optional
	EngineVersion string `json:"engineVersion,omitempty"`

	// MasterUsername is the login for the master user. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="masterUsername is immutable"
	MasterUsername string `json:"masterUsername"`

	// MasterUserPasswordRef references a Kubernetes Secret containing the master password.
	MasterUserPasswordRef SecretRef `json:"masterUserPasswordRef"`

	// DBSubnetGroupRef is the name of a DBSubnetGroup CR in the same namespace.
	// +optional
	DBSubnetGroupRef string `json:"dbSubnetGroupRef,omitempty"`

	// DBClusterParameterGroupRef is the name of a DBClusterParameterGroup CR in the same namespace.
	// +optional
	DBClusterParameterGroupRef string `json:"dbClusterParameterGroupRef,omitempty"`

	// VPCSecurityGroupRefs is the list of VPC security group references.
	// +optional
	VPCSecurityGroupRefs []SecurityGroupRef `json:"vpcSecurityGroupRefs,omitempty"`

	// BackupRetentionPeriod is the number of days to retain automated backups.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=35
	// +optional
	BackupRetentionPeriod int32 `json:"backupRetentionPeriod,omitempty"`

	// StorageEncrypted enables encryption at rest.
	// +optional
	StorageEncrypted bool `json:"storageEncrypted,omitempty"`

	// KMSKeyID is the KMS key ARN/ID for storage encryption.
	// +optional
	KMSKeyID string `json:"kmsKeyId,omitempty"`

	// DeletionProtection prevents the cluster from being deleted.
	// +optional
	DeletionProtection bool `json:"deletionProtection,omitempty"`

	// SkipFinalSnapshot skips the final snapshot when the cluster is deleted.
	// +optional
	SkipFinalSnapshot bool `json:"skipFinalSnapshot,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`

	// PreferredBackupWindow is the daily time range for automated backups (e.g. "02:00-03:00").
	// +optional
	PreferredBackupWindow string `json:"preferredBackupWindow,omitempty"`

	// PreferredMaintenanceWindow is the weekly time range for maintenance (e.g. "sun:05:00-sun:06:00").
	// +optional
	PreferredMaintenanceWindow string `json:"preferredMaintenanceWindow,omitempty"`

	// EnabledCloudwatchLogsExports is the list of log types to export to CloudWatch.
	// +optional
	EnabledCloudwatchLogsExports []string `json:"enabledCloudwatchLogsExports,omitempty"`

	// EnableIAMDatabaseAuthentication enables IAM database authentication.
	// +optional
	EnableIAMDatabaseAuthentication *bool `json:"enableIAMDatabaseAuthentication,omitempty"`

	// CopyTagsToSnapshot copies tags to automated DB snapshots.
	// +optional
	CopyTagsToSnapshot *bool `json:"copyTagsToSnapshot,omitempty"`

	// AutoMinorVersionUpgrade enables automatic minor version upgrades during the maintenance window.
	// +optional
	AutoMinorVersionUpgrade *bool `json:"autoMinorVersionUpgrade,omitempty"`

	// EnableHttpEndpoint enables the Data API for Aurora Serverless.
	// +optional
	EnableHttpEndpoint *bool `json:"enableHttpEndpoint,omitempty"`

	// EngineMode is the DB engine mode: provisioned, serverless, or parallelquery.
	// +kubebuilder:validation:Enum=provisioned;serverless;parallelquery;global;multimaster
	// +optional
	EngineMode string `json:"engineMode,omitempty"`

	// ServerlessV2ScalingConfig defines the capacity range for Aurora Serverless v2.
	// +optional
	ServerlessV2ScalingConfig *RDSServerlessV2ScalingConfig `json:"serverlessV2ScalingConfig,omitempty"`

	// BacktrackWindow is the backtrack window in seconds for Aurora MySQL.
	// +optional
	BacktrackWindow *int64 `json:"backtrackWindow,omitempty"`

	// NetworkType is the network type: IPV4 or DUAL.
	// +kubebuilder:validation:Enum=IPV4;DUAL
	// +optional
	NetworkType string `json:"networkType,omitempty"`

	// PerformanceInsightsEnabled enables Performance Insights for cluster instances.
	// +optional
	PerformanceInsightsEnabled *bool `json:"performanceInsightsEnabled,omitempty"`

	// PerformanceInsightsKMSKeyID is the KMS key ARN/ID for Performance Insights encryption.
	// +optional
	PerformanceInsightsKMSKeyID string `json:"performanceInsightsKmsKeyId,omitempty"`

	// PerformanceInsightsRetentionPeriod is the number of days to retain Performance Insights data.
	// +optional
	PerformanceInsightsRetentionPeriod *int32 `json:"performanceInsightsRetentionPeriod,omitempty"`

	// AllocatedStorage is the storage size in GiB (required for Aurora I/O-Optimized).
	// +optional
	AllocatedStorage *int32 `json:"allocatedStorage,omitempty"`

	// StorageType is the storage type (aurora, aurora-iopt1).
	// +optional
	StorageType string `json:"storageType,omitempty"`

	// Port is the port the cluster listens on.
	// +optional
	Port *int32 `json:"port,omitempty"`
}

// DBClusterStatus defines the observed state of DBCluster.
type DBClusterStatus struct {
	// DBClusterARN is the Amazon Resource Name of the DB cluster.
	// +optional
	DBClusterARN string `json:"dbClusterArn,omitempty"`

	// Endpoint is the writer endpoint hostname.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// ReaderEndpoint is the reader endpoint hostname.
	// +optional
	ReaderEndpoint string `json:"readerEndpoint,omitempty"`

	// Status is the current cluster status (available, creating, modifying, deleting, etc.).
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
// +kubebuilder:printcolumn:name="Endpoint",type="string",JSONPath=".status.endpoint"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DBCluster is the Schema for managing Aurora DB Clusters.
type DBCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DBClusterSpec   `json:"spec,omitempty"`
	Status DBClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DBClusterList contains a list of DBCluster
type DBClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DBCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DBCluster{}, &DBClusterList{})
}
