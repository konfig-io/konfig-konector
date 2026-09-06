/*
Copyright 2024.

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

// ResourceRef is a reference to another konfig-konector managed resource.
type ResourceRef struct {
	// Name of the resource in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// Namespace of the resource. Defaults to the referring object's namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// PolicyRef references either a managed IAMPolicy CR or a direct AWS policy ARN.
type PolicyRef struct {
	// Name of an IAMPolicy CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// Namespace of the IAMPolicy CR. Defaults to the referring object's namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// ARN is a direct AWS policy ARN (e.g. for AWS managed policies).
	// If set, Name and Namespace are ignored.
	// +optional
	ARN string `json:"arn,omitempty"`
}

// RoleRef references either a managed IAMRole CR or a direct AWS role ARN.
type RoleRef struct {
	// Name of an IAMRole CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// Namespace of the IAMRole CR. Defaults to the referring object's namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// ARN is a direct AWS IAM role ARN.
	// If set, Name and Namespace are ignored.
	// +optional
	ARN string `json:"arn,omitempty"`
}

// VPCResourceRef references either a managed VPC CR or a direct VPC ID.
type VPCResourceRef struct {
	// Name of a VPC CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ID is a direct AWS VPC ID (e.g. vpc-0abc1234).
	// +optional
	ID string `json:"id,omitempty"`
}

// SubnetRef references either a managed Subnet CR or a direct Subnet ID.
type SubnetRef struct {
	// Name of a Subnet CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ID is a direct AWS Subnet ID (e.g. subnet-0abc1234).
	// +optional
	ID string `json:"id,omitempty"`
}

// SecurityGroupRef references either a managed SecurityGroup CR or a direct Security Group ID.
type SecurityGroupRef struct {
	// Name of a SecurityGroup CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ID is a direct AWS Security Group ID (e.g. sg-0abc1234).
	// +optional
	ID string `json:"id,omitempty"`
}

// SecretRef references a Kubernetes Secret and a key within it.
type SecretRef struct {
	// Name of the Kubernetes Secret.
	Name string `json:"name"`
	// Key is the key within the Secret whose value to use.
	Key string `json:"key"`
}

// EKSClusterRef references either a managed EKSCluster CR or a direct AWS cluster name.
type EKSClusterRef struct {
	// Name of an EKSCluster CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ClusterName is a direct AWS EKS cluster name, bypassing CR lookup.
	// +optional
	ClusterName string `json:"clusterName,omitempty"`
}

// EKSTaint defines a Kubernetes taint applied to EKS nodes.
type EKSTaint struct {
	// Key is the taint key.
	Key string `json:"key"`
	// Value is the taint value.
	// +optional
	Value string `json:"value,omitempty"`
	// Effect specifies the taint effect.
	// +kubebuilder:validation:Enum=NO_SCHEDULE;NO_EXECUTE;PREFER_NO_SCHEDULE
	Effect string `json:"effect"`
}

// LambdaFunctionRef references either a managed LambdaFunction CR or a direct function name/ARN.
type LambdaFunctionRef struct {
	// Name of a LambdaFunction CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// FunctionName is a direct AWS Lambda function name or ARN.
	// +optional
	FunctionName string `json:"functionName,omitempty"`
}

// ECSClusterRef references either a managed ECSCluster CR or a direct cluster name/ARN.
type ECSClusterRef struct {
	// Name of an ECSCluster CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ClusterName is a direct AWS ECS cluster name or ARN.
	// +optional
	ClusterName string `json:"clusterName,omitempty"`
}

// ECSTaskDefinitionRef references either a managed ECSTaskDefinition CR or a direct task definition ARN.
type ECSTaskDefinitionRef struct {
	// Name of an ECSTaskDefinition CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ARN is a direct AWS task definition ARN (e.g. arn:aws:ecs:...:task-definition/my-task:5).
	// +optional
	ARN string `json:"arn,omitempty"`
}

// GroupRef references either a managed IAMGroup CR or a direct AWS group name.
type GroupRef struct {
	// Name of an IAMGroup CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// GroupName is a direct AWS IAM group name, bypassing CR lookup.
	// +optional
	GroupName string `json:"groupName,omitempty"`
}

// UserRef references either a managed IAMUser CR or a direct AWS user name.
type UserRef struct {
	// Name of an IAMUser CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// UserName is a direct AWS IAM user name, bypassing CR lookup.
	// +optional
	UserName string `json:"userName,omitempty"`
}

// ConditionType constants used across all resources.
const (
	ConditionReady   = "Ready"
	ConditionSyncing = "Syncing"

	ReasonCreated     = "Created"
	ReasonUpdated     = "Updated"
	ReasonSynced      = "Synced"
	ReasonDeleting    = "Deleting"
	ReasonError       = "Error"
	ReasonNotFound    = "NotFound"
	ReasonRefNotReady = "ReferenceNotReady"

	FinalizerName = "aws.konfig.io/finalizer"

	// DeletionPolicyAnnotation controls what happens to the AWS resource when
	// the CR is deleted. Set to DeletionPolicyAbandon to keep the AWS resource.
	DeletionPolicyAnnotation = "aws.konfig.io/deletion-policy"
	DeletionPolicyAbandon    = "abandon"

	ReasonUpdateNotSupported = "UpdateNotSupported"
	// ReasonPendingAcceptance marks a two-sided resource (peering, attachment,
	// share invitation) that the other side has not accepted yet.
	ReasonPendingAcceptance = "PendingAcceptance"
	ReasonAbandoned          = "Abandoned"
)

// APIRef references a managed API Gateway API CR (APIGatewayV2API or RestAPI,
// depending on the referring kind) or a direct AWS API ID.
type APIRef struct {
	// Name of the API CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// APIID is a direct AWS API Gateway API ID, bypassing CR lookup.
	// +optional
	APIID string `json:"apiId,omitempty"`
}
