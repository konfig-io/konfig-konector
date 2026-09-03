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

// DBInstanceSpec defines the desired state of an RDS DB Instance.
type DBInstanceSpec struct {
	// DBInstanceIdentifier is the unique name for the DB instance. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="dbInstanceIdentifier is immutable"
	DBInstanceIdentifier string `json:"dbInstanceIdentifier"`

	// DBInstanceClass is the compute and memory capacity (e.g. db.t3.micro).
	// +kubebuilder:validation:MinLength=1
	DBInstanceClass string `json:"dbInstanceClass"`

	// Engine is the database engine (e.g. mysql, postgres, mariadb).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="engine is immutable"
	Engine string `json:"engine"`

	// EngineVersion is the version of the database engine.
	// +optional
	EngineVersion string `json:"engineVersion,omitempty"`

	// MasterUsername is the login name for the master user. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="masterUsername is immutable"
	MasterUsername string `json:"masterUsername"`

	// MasterUserPasswordRef references a Kubernetes Secret containing the master password.
	MasterUserPasswordRef SecretRef `json:"masterUserPasswordRef"`

	// DBName is the name of the initial database. Immutable after creation.
	// +optional
	DBName string `json:"dbName,omitempty"`

	// AllocatedStorage is the storage size in GiB.
	// +kubebuilder:validation:Minimum=5
	AllocatedStorage int32 `json:"allocatedStorage"`

	// StorageType is the storage type: gp2, gp3, or io1.
	// +kubebuilder:validation:Enum=gp2;gp3;io1;standard
	// +optional
	StorageType string `json:"storageType,omitempty"`

	// StorageEncrypted enables encryption at rest.
	// +optional
	StorageEncrypted bool `json:"storageEncrypted,omitempty"`

	// KMSKeyID is the KMS key ARN/ID for storage encryption.
	// +optional
	KMSKeyID string `json:"kmsKeyId,omitempty"`

	// MultiAZ enables a Multi-AZ deployment.
	// +optional
	MultiAZ bool `json:"multiAZ,omitempty"`

	// PubliclyAccessible controls whether the DB is publicly accessible.
	// +optional
	PubliclyAccessible bool `json:"publiclyAccessible,omitempty"`

	// DBSubnetGroupRef is the name of a DBSubnetGroup CR in the same namespace.
	// +optional
	DBSubnetGroupRef string `json:"dbSubnetGroupRef,omitempty"`

	// DBParameterGroupRef is the name of a DBParameterGroup CR in the same namespace.
	// +optional
	DBParameterGroupRef string `json:"dbParameterGroupRef,omitempty"`

	// VPCSecurityGroupRefs is the list of VPC security group references.
	// +optional
	VPCSecurityGroupRefs []SecurityGroupRef `json:"vpcSecurityGroupRefs,omitempty"`

	// Port is the port the DB listens on.
	// +optional
	Port int32 `json:"port,omitempty"`

	// BackupRetentionPeriod is the number of days to retain automated backups (0 to disable).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=35
	// +optional
	BackupRetentionPeriod int32 `json:"backupRetentionPeriod,omitempty"`

	// DeletionProtection prevents the DB instance from being deleted.
	// +optional
	DeletionProtection bool `json:"deletionProtection,omitempty"`

	// SkipFinalSnapshot skips the final snapshot when the instance is deleted.
	// +optional
	SkipFinalSnapshot bool `json:"skipFinalSnapshot,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`

	// MaxAllocatedStorage enables storage autoscaling up to this size in GiB.
	// +optional
	MaxAllocatedStorage *int32 `json:"maxAllocatedStorage,omitempty"`

	// PreferredBackupWindow is the daily time range for automated backups (e.g. "02:00-03:00").
	// +optional
	PreferredBackupWindow string `json:"preferredBackupWindow,omitempty"`

	// PreferredMaintenanceWindow is the weekly time range for maintenance (e.g. "sun:05:00-sun:06:00").
	// +optional
	PreferredMaintenanceWindow string `json:"preferredMaintenanceWindow,omitempty"`

	// EnablePerformanceInsights enables Performance Insights for the DB instance.
	// +optional
	EnablePerformanceInsights *bool `json:"enablePerformanceInsights,omitempty"`

	// PerformanceInsightsKMSKeyID is the KMS key ARN/ID for Performance Insights encryption.
	// +optional
	PerformanceInsightsKMSKeyID string `json:"performanceInsightsKmsKeyId,omitempty"`

	// PerformanceInsightsRetentionPeriod is the number of days to retain Performance Insights data (7 or 731).
	// +optional
	PerformanceInsightsRetentionPeriod *int32 `json:"performanceInsightsRetentionPeriod,omitempty"`

	// MonitoringInterval is the interval in seconds for Enhanced Monitoring metrics (0, 1, 5, 10, 15, 30, 60).
	// +optional
	MonitoringInterval *int32 `json:"monitoringInterval,omitempty"`

	// MonitoringRoleARN is the IAM role ARN for Enhanced Monitoring.
	// +optional
	MonitoringRoleARN string `json:"monitoringRoleArn,omitempty"`

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

	// Iops is the I/O operations per second for io1/io2 storage.
	// +optional
	Iops *int32 `json:"iops,omitempty"`

	// StorageThroughput is the storage throughput in MiB/s for gp3 storage.
	// +optional
	StorageThroughput *int32 `json:"storageThroughput,omitempty"`
}

// DBInstanceStatus defines the observed state of DBInstance.
type DBInstanceStatus struct {
	// DBInstanceARN is the Amazon Resource Name of the DB instance.
	// +optional
	DBInstanceARN string `json:"dbInstanceArn,omitempty"`

	// Endpoint is the connection endpoint hostname.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// Port is the connection port.
	// +optional
	Port int32 `json:"port,omitempty"`

	// DBInstanceStatus is the current status (available, creating, modifying, deleting, etc.).
	// +optional
	DBInstanceStatus string `json:"dbInstanceStatus,omitempty"`

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
// +kubebuilder:printcolumn:name="DB-Status",type="string",JSONPath=".status.dbInstanceStatus"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DBInstance is the Schema for managing RDS DB Instances.
type DBInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DBInstanceSpec   `json:"spec,omitempty"`
	Status DBInstanceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DBInstanceList contains a list of DBInstance
type DBInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DBInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DBInstance{}, &DBInstanceList{})
}
