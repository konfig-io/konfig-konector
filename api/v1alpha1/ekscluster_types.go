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

// EKSClusterVpcConfig defines the VPC configuration for the EKS control plane.
type EKSClusterVpcConfig struct {
	// SubnetRefs are the subnets for the EKS control plane ENIs. At least 2 in different AZs required.
	// +kubebuilder:validation:MinItems=1
	SubnetRefs []SubnetRef `json:"subnetRefs"`

	// SecurityGroupRefs are additional security groups for the control plane ENIs.
	// +optional
	SecurityGroupRefs []SecurityGroupRef `json:"securityGroupRefs,omitempty"`

	// EndpointPublicAccess enables the public API server endpoint. Default: true.
	// +optional
	EndpointPublicAccess *bool `json:"endpointPublicAccess,omitempty"`

	// EndpointPrivateAccess enables the private API server endpoint. Default: false.
	// +optional
	EndpointPrivateAccess *bool `json:"endpointPrivateAccess,omitempty"`

	// PublicAccessCidrs restricts access to the public endpoint. Default: ["0.0.0.0/0"].
	// +optional
	PublicAccessCidrs []string `json:"publicAccessCidrs,omitempty"`
}

// EKSClusterLogging defines which control plane log types to enable.
type EKSClusterLogging struct {
	// EnabledTypes is the list of log types to enable.
	// Valid values: api, audit, authenticator, controllerManager, scheduler.
	// +optional
	EnabledTypes []string `json:"enabledTypes,omitempty"`
}

// EKSClusterEncryptionConfig configures KMS encryption for Kubernetes secrets.
type EKSClusterEncryptionConfig struct {
	// ProviderKeyArn is the KMS key ARN to use for encryption.
	ProviderKeyArn string `json:"providerKeyArn"`

	// Resources is the list of Kubernetes resources to encrypt (e.g. ["secrets"]).
	Resources []string `json:"resources"`
}

// EKSClusterAccessConfig configures the cluster authentication mode.
type EKSClusterAccessConfig struct {
	// AuthenticationMode controls how IAM principals are granted cluster access.
	// +kubebuilder:validation:Enum=API;API_AND_CONFIG_MAP;CONFIG_MAP
	// +optional
	AuthenticationMode string `json:"authenticationMode,omitempty"`

	// BootstrapClusterCreatorAdminPermissions grants the cluster creator admin access.
	// +optional
	BootstrapClusterCreatorAdminPermissions *bool `json:"bootstrapClusterCreatorAdminPermissions,omitempty"`
}

// EKSKubernetesNetworkConfig configures the Kubernetes network for the cluster.
type EKSKubernetesNetworkConfig struct {
	// ServiceIPv4CIDR is the CIDR block for Kubernetes service IPs. Immutable after creation.
	// +optional
	ServiceIPv4CIDR string `json:"serviceIpv4Cidr,omitempty"`
	// IPFamily is the IP family for the cluster: ipv4 or ipv6. Immutable after creation.
	// +kubebuilder:validation:Enum=ipv4;ipv6
	// +optional
	IPFamily string `json:"ipFamily,omitempty"`
}

// EKSUpgradePolicy configures the cluster upgrade support policy.
type EKSUpgradePolicy struct {
	// SupportType is STANDARD or EXTENDED.
	// +kubebuilder:validation:Enum=STANDARD;EXTENDED
	SupportType string `json:"supportType"`
}

// EKSClusterSpec defines the desired state of an EKS Cluster.
type EKSClusterSpec struct {
	// ClusterName is the name of the EKS cluster. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=100
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="clusterName is immutable"
	ClusterName string `json:"clusterName"`

	// Version is the Kubernetes version for the cluster (e.g. "1.30").
	// +optional
	Version string `json:"version,omitempty"`

	// RoleArn is the IAM role ARN for the EKS control plane.
	// Either roleArn or roleRef must be set.
	// +optional
	RoleArn string `json:"roleArn,omitempty"`

	// RoleRef is a reference to an IAMRole CR in the same namespace.
	// Either roleArn or roleRef must be set.
	// +optional
	RoleRef *RoleRef `json:"roleRef,omitempty"`

	// ResourcesVpcConfig is the VPC configuration for the cluster control plane.
	ResourcesVpcConfig EKSClusterVpcConfig `json:"resourcesVpcConfig"`

	// Logging configures control plane logging.
	// +optional
	Logging *EKSClusterLogging `json:"logging,omitempty"`

	// EncryptionConfig enables KMS encryption of Kubernetes secrets.
	// +optional
	EncryptionConfig *EKSClusterEncryptionConfig `json:"encryptionConfig,omitempty"`

	// AccessConfig configures the cluster authentication mode.
	// +optional
	AccessConfig *EKSClusterAccessConfig `json:"accessConfig,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`

	// KubernetesNetworkConfig configures the Kubernetes network. Immutable after creation.
	// +optional
	KubernetesNetworkConfig *EKSKubernetesNetworkConfig `json:"kubernetesNetworkConfig,omitempty"`

	// UpgradePolicy configures the cluster version support policy.
	// +optional
	UpgradePolicy *EKSUpgradePolicy `json:"upgradePolicy,omitempty"`

	// BootstrapSelfManagedAddons installs self-managed add-ons on cluster creation.
	// Immutable after creation.
	// +optional
	BootstrapSelfManagedAddons *bool `json:"bootstrapSelfManagedAddons,omitempty"`
}

// EKSClusterStatus defines the observed state of EKSCluster.
type EKSClusterStatus struct {
	// ClusterArn is the ARN of the EKS cluster.
	// +optional
	ClusterArn string `json:"clusterArn,omitempty"`

	// Endpoint is the Kubernetes API server endpoint.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// Version is the current Kubernetes version of the cluster.
	// +optional
	Version string `json:"version,omitempty"`

	// Status is the cluster status: CREATING, ACTIVE, DELETING, FAILED, UPDATING.
	// +optional
	Status string `json:"status,omitempty"`

	// CertificateAuthority is the base64-encoded certificate authority data.
	// +optional
	CertificateAuthority string `json:"certificateAuthority,omitempty"`

	// OIDCIssuer is the OIDC issuer URL for the cluster (used for IRSA).
	// +optional
	OIDCIssuer string `json:"oidcIssuer,omitempty"`

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
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".status.version"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EKSCluster is the Schema for managing EKS Clusters.
type EKSCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EKSClusterSpec   `json:"spec,omitempty"`
	Status EKSClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EKSClusterList contains a list of EKSCluster.
type EKSClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EKSCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EKSCluster{}, &EKSClusterList{})
}
