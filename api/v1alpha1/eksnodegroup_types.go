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

// EKSNodeGroupScalingConfig defines the scaling parameters for a node group.
type EKSNodeGroupScalingConfig struct {
	// MinSize is the minimum number of nodes.
	// +kubebuilder:validation:Minimum=0
	MinSize int32 `json:"minSize"`

	// MaxSize is the maximum number of nodes.
	// +kubebuilder:validation:Minimum=1
	MaxSize int32 `json:"maxSize"`

	// DesiredSize is the desired number of nodes.
	// +kubebuilder:validation:Minimum=0
	DesiredSize int32 `json:"desiredSize"`
}

// EKSNodeGroupUpdateConfig defines rolling update behavior.
type EKSNodeGroupUpdateConfig struct {
	// MaxUnavailable is the maximum number of nodes unavailable during an update.
	// +optional
	MaxUnavailable *int32 `json:"maxUnavailable,omitempty"`

	// MaxUnavailablePercentage is the maximum percentage of nodes unavailable during an update.
	// +optional
	MaxUnavailablePercentage *int32 `json:"maxUnavailablePercentage,omitempty"`
}

// EKSNodeGroupRemoteAccess configures SSH access to nodes.
type EKSNodeGroupRemoteAccess struct {
	// EC2SshKey is the name of the EC2 key pair for SSH access.
	// +optional
	EC2SshKey string `json:"ec2SshKey,omitempty"`

	// SourceSecurityGroupRefs restricts SSH access to these security groups.
	// +optional
	SourceSecurityGroupRefs []SecurityGroupRef `json:"sourceSecurityGroupRefs,omitempty"`
}

// EKSNodeGroupLaunchTemplate references an EC2 Launch Template for the node group.
type EKSNodeGroupLaunchTemplate struct {
	// Name is a LaunchTemplate CR name in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`

	// ID is a direct AWS Launch Template ID.
	// +optional
	ID string `json:"id,omitempty"`

	// Version is the launch template version. Defaults to $Latest.
	// +optional
	Version string `json:"version,omitempty"`
}

// EKSNodeRepairConfig configures automatic node repair for the node group.
type EKSNodeRepairConfig struct {
	// Enabled enables automatic node repair.
	Enabled bool `json:"enabled"`
}

// EKSNodeGroupSpec defines the desired state of an EKS Managed Node Group.
type EKSNodeGroupSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ClusterName is the EKS cluster name. Either clusterName or clusterRef must be set.
	// +optional
	ClusterName string `json:"clusterName,omitempty"`

	// ClusterRef references an EKSCluster CR in the same namespace.
	// +optional
	ClusterRef *EKSClusterRef `json:"clusterRef,omitempty"`

	// NodegroupName is the name of the node group. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="nodegroupName is immutable"
	NodegroupName string `json:"nodegroupName"`

	// NodeRoleArn is the IAM role ARN for the node instances.
	// Either nodeRoleArn or nodeRoleRef must be set.
	// +optional
	NodeRoleArn string `json:"nodeRoleArn,omitempty"`

	// NodeRoleRef references an IAMRole CR in the same namespace.
	// +optional
	NodeRoleRef *RoleRef `json:"nodeRoleRef,omitempty"`

	// SubnetRefs are the subnets to launch nodes in.
	// +kubebuilder:validation:MinItems=1
	SubnetRefs []SubnetRef `json:"subnetRefs"`

	// ScalingConfig defines the min/max/desired node counts.
	ScalingConfig EKSNodeGroupScalingConfig `json:"scalingConfig"`

	// InstanceTypes is the list of EC2 instance types. Defaults to t3.medium.
	// +optional
	InstanceTypes []string `json:"instanceTypes,omitempty"`

	// AmiType is the AMI type for nodes.
	// +kubebuilder:validation:Enum=AL2_x86_64;AL2_x86_64_GPU;AL2_ARM_64;AL2023_x86_64_STANDARD;AL2023_ARM_64_STANDARD;BOTTLEROCKET_x86_64;BOTTLEROCKET_ARM_64;CUSTOM
	// +optional
	AmiType string `json:"amiType,omitempty"`

	// CapacityType is the capacity type: ON_DEMAND or SPOT.
	// +kubebuilder:validation:Enum=ON_DEMAND;SPOT
	// +optional
	CapacityType string `json:"capacityType,omitempty"`

	// DiskSize is the root EBS volume size in GiB.
	// +optional
	DiskSize *int32 `json:"diskSize,omitempty"`

	// Labels are Kubernetes labels applied to nodes.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Taints are Kubernetes taints applied to nodes.
	// +optional
	Taints []EKSTaint `json:"taints,omitempty"`

	// UpdateConfig defines rolling update behavior.
	// +optional
	UpdateConfig *EKSNodeGroupUpdateConfig `json:"updateConfig,omitempty"`

	// LaunchTemplate is an optional custom launch template.
	// +optional
	LaunchTemplate *EKSNodeGroupLaunchTemplate `json:"launchTemplate,omitempty"`

	// ReleaseVersion is the AMI release version. Leave empty for latest.
	// +optional
	ReleaseVersion string `json:"releaseVersion,omitempty"`

	// Version is the Kubernetes version for the node group. Defaults to the cluster version.
	// +optional
	Version string `json:"version,omitempty"`

	// RemoteAccess configures SSH access to nodes.
	// +optional
	RemoteAccess *EKSNodeGroupRemoteAccess `json:"remoteAccess,omitempty"`

	// Tags are AWS resource tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`

	// NodeRepairConfig configures automatic node repair.
	// +optional
	NodeRepairConfig *EKSNodeRepairConfig `json:"nodeRepairConfig,omitempty"`
}

// EKSNodeGroupStatus defines the observed state of EKSNodeGroup.
type EKSNodeGroupStatus struct {
	// NodegroupArn is the ARN of the node group.
	// +optional
	NodegroupArn string `json:"nodegroupArn,omitempty"`

	// Status is the node group status: CREATING, ACTIVE, UPDATING, DELETING, DEGRADED.
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
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.clusterName"
// +kubebuilder:printcolumn:name="Nodegroup",type="string",JSONPath=".spec.nodegroupName"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EKSNodeGroup is the Schema for managing EKS Managed Node Groups.
type EKSNodeGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EKSNodeGroupSpec   `json:"spec,omitempty"`
	Status EKSNodeGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EKSNodeGroupList contains a list of EKSNodeGroup.
type EKSNodeGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EKSNodeGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EKSNodeGroup{}, &EKSNodeGroupList{})
}
