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

// LambdaS3Code references a deployment package stored in S3.
type LambdaS3Code struct {
	// S3Bucket is the S3 bucket containing the deployment package.
	S3Bucket string `json:"s3Bucket"`
	// S3Key is the S3 key of the deployment package.
	S3Key string `json:"s3Key"`
	// S3ObjectVersion is the S3 object version of the deployment package.
	// +optional
	S3ObjectVersion string `json:"s3ObjectVersion,omitempty"`
}

// LambdaCodeSource defines where the function code comes from.
// Exactly one of S3 or ImageURI must be set.
type LambdaCodeSource struct {
	// S3 references a zip deployment package in S3.
	// +optional
	S3 *LambdaS3Code `json:"s3,omitempty"`

	// ImageURI is an ECR image URI for container image functions.
	// +optional
	ImageURI string `json:"imageUri,omitempty"`
}

// LambdaVpcConfig configures the VPC networking for a Lambda function.
type LambdaVpcConfig struct {
	// SubnetRefs are the subnets the function runs in.
	SubnetRefs []SubnetRef `json:"subnetRefs"`
	// SecurityGroupRefs are the security groups attached to the function.
	SecurityGroupRefs []SecurityGroupRef `json:"securityGroupRefs"`
}

// LambdaDeadLetterConfig configures a dead-letter queue or SNS topic for failed async invocations.
type LambdaDeadLetterConfig struct {
	// TargetARN is the ARN of an SQS queue or SNS topic.
	TargetARN string `json:"targetARN"`
}

// LambdaTracingConfig configures AWS X-Ray tracing for the function.
type LambdaTracingConfig struct {
	// Mode is the tracing mode: PassThrough or Active.
	// +kubebuilder:validation:Enum=PassThrough;Active
	Mode string `json:"mode"`
}

// LambdaLoggingConfig configures CloudWatch Logs for the function.
type LambdaLoggingConfig struct {
	// LogFormat is the format for function logs: Text or JSON.
	// +kubebuilder:validation:Enum=Text;JSON
	// +optional
	LogFormat string `json:"logFormat,omitempty"`

	// LogGroup is the CloudWatch log group name. Defaults to /aws/lambda/{function-name}.
	// +optional
	LogGroup string `json:"logGroup,omitempty"`

	// SystemLogLevel is the log level for system logs: DEBUG, INFO, or WARN.
	// +kubebuilder:validation:Enum=DEBUG;INFO;WARN
	// +optional
	SystemLogLevel string `json:"systemLogLevel,omitempty"`

	// ApplicationLogLevel is the log level for application logs: TRACE, DEBUG, INFO, WARN, ERROR, or FATAL.
	// +kubebuilder:validation:Enum=TRACE;DEBUG;INFO;WARN;ERROR;FATAL
	// +optional
	ApplicationLogLevel string `json:"applicationLogLevel,omitempty"`
}

// LambdaFileSystemConfig mounts an EFS access point into the function.
type LambdaFileSystemConfig struct {
	// ARN is the ARN of the EFS access point.
	ARN string `json:"arn"`
	// LocalMountPath is the path where the file system is mounted (must start with /mnt/).
	LocalMountPath string `json:"localMountPath"`
}

// LambdaSnapStart configures SnapStart for supported runtimes (Java).
type LambdaSnapStart struct {
	// ApplyOn controls when SnapStart is applied: PublishedVersions or None.
	// +kubebuilder:validation:Enum=PublishedVersions;None
	ApplyOn string `json:"applyOn"`
}

// LambdaImageConfig overrides the container image configuration.
type LambdaImageConfig struct {
	// Command overrides the CMD in the Dockerfile.
	// +optional
	Command []string `json:"command,omitempty"`
	// EntryPoint overrides the ENTRYPOINT in the Dockerfile.
	// +optional
	EntryPoint []string `json:"entryPoint,omitempty"`
	// WorkingDirectory overrides the working directory.
	// +optional
	WorkingDirectory string `json:"workingDirectory,omitempty"`
}

// LambdaFunctionSpec defines the desired state of a Lambda Function.
type LambdaFunctionSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// FunctionName is the name of the Lambda function. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="functionName is immutable"
	FunctionName string `json:"functionName"`

	// RoleArn is the IAM execution role ARN for the function.
	// Either roleArn or roleRef must be set.
	// +optional
	RoleArn string `json:"roleArn,omitempty"`

	// RoleRef references an IAMRole CR in the same namespace.
	// +optional
	RoleRef *RoleRef `json:"roleRef,omitempty"`

	// Runtime is the function runtime identifier (e.g. provided.al2023, python3.12, nodejs20.x).
	// Required for zip deployments; omit for container image functions.
	// +optional
	Runtime string `json:"runtime,omitempty"`

	// Handler is the function entrypoint (e.g. index.handler). Required for zip deployments.
	// +optional
	Handler string `json:"handler,omitempty"`

	// Code defines where the function code lives.
	Code LambdaCodeSource `json:"code"`

	// Description is a human-readable description of the function.
	// +optional
	Description string `json:"description,omitempty"`

	// Timeout is the maximum execution time in seconds. Default: 3.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=900
	// +optional
	Timeout *int32 `json:"timeout,omitempty"`

	// MemorySize is the memory allocated to the function in MB. Default: 128.
	// +kubebuilder:validation:Minimum=128
	// +kubebuilder:validation:Maximum=10240
	// +optional
	MemorySize *int32 `json:"memorySize,omitempty"`

	// Environment variables for the function.
	// +optional
	Environment map[string]string `json:"environment,omitempty"`

	// VpcConfig places the function inside a VPC.
	// +optional
	VpcConfig *LambdaVpcConfig `json:"vpcConfig,omitempty"`

	// Architectures is the instruction set architecture. Default: x86_64.
	// +kubebuilder:validation:Enum=x86_64;arm64
	// +optional
	Architecture string `json:"architecture,omitempty"`

	// EphemeralStorageSize is the /tmp storage size in MB. Default: 512.
	// +optional
	EphemeralStorageSize *int32 `json:"ephemeralStorageSize,omitempty"`

	// Tags are AWS resource tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`

	// Layers is a list of Lambda layer ARNs to attach.
	// +optional
	Layers []string `json:"layers,omitempty"`

	// DeadLetterConfig configures a dead-letter queue or SNS topic for failed async invocations.
	// +optional
	DeadLetterConfig *LambdaDeadLetterConfig `json:"deadLetterConfig,omitempty"`

	// TracingConfig configures AWS X-Ray tracing.
	// +optional
	TracingConfig *LambdaTracingConfig `json:"tracingConfig,omitempty"`

	// LoggingConfig configures CloudWatch Logs for the function.
	// +optional
	LoggingConfig *LambdaLoggingConfig `json:"loggingConfig,omitempty"`

	// ReservedConcurrency sets the reserved concurrency limit. 0 throttles all invocations.
	// Set to -1 to explicitly remove a previously set reservation.
	// +optional
	ReservedConcurrency *int32 `json:"reservedConcurrency,omitempty"`

	// FileSystemConfigs mounts EFS access points into the function.
	// +optional
	FileSystemConfigs []LambdaFileSystemConfig `json:"fileSystemConfigs,omitempty"`

	// SnapStart configures SnapStart for supported runtimes (Java).
	// +optional
	SnapStart *LambdaSnapStart `json:"snapStart,omitempty"`

	// ImageConfig overrides the container image configuration.
	// +optional
	ImageConfig *LambdaImageConfig `json:"imageConfig,omitempty"`
}

// LambdaFunctionStatus defines the observed state of LambdaFunction.
type LambdaFunctionStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// FunctionARN is the ARN of the Lambda function.
	// +optional
	FunctionARN string `json:"functionArn,omitempty"`

	// State is the function state: Pending, Active, Inactive, Failed.
	// +optional
	State string `json:"state,omitempty"`

	// CodeSha256 is the SHA256 of the deployed code package.
	// +optional
	CodeSha256 string `json:"codeSha256,omitempty"`

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
// +kubebuilder:printcolumn:name="Function",type="string",JSONPath=".spec.functionName"
// +kubebuilder:printcolumn:name="Runtime",type="string",JSONPath=".spec.runtime"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// LambdaFunction is the Schema for managing AWS Lambda functions.
type LambdaFunction struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LambdaFunctionSpec   `json:"spec,omitempty"`
	Status LambdaFunctionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LambdaFunctionList contains a list of LambdaFunction.
type LambdaFunctionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LambdaFunction `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LambdaFunction{}, &LambdaFunctionList{})
}
