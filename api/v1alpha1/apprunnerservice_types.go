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

// AppRunnerImageConfiguration configures how the image is run.
type AppRunnerImageConfiguration struct {
	// Port the application listens on in the container. Default 8080.
	// +optional
	Port string `json:"port,omitempty"`

	// RuntimeEnvironmentVariables available to the running service.
	// +optional
	RuntimeEnvironmentVariables map[string]string `json:"runtimeEnvironmentVariables,omitempty"`

	// StartCommand overrides the image's default start command.
	// +optional
	StartCommand string `json:"startCommand,omitempty"`
}

// AppRunnerImageRepository describes a source image repository.
type AppRunnerImageRepository struct {
	// ImageIdentifier is the image name (ECR) or full URI (ECR Public).
	// +kubebuilder:validation:MinLength=1
	ImageIdentifier string `json:"imageIdentifier"`

	// ImageRepositoryType is the repository provider.
	// +kubebuilder:validation:Enum=ECR;ECR_PUBLIC
	ImageRepositoryType string `json:"imageRepositoryType"`

	// ImageConfiguration configures how the image runs.
	// +optional
	ImageConfiguration *AppRunnerImageConfiguration `json:"imageConfiguration,omitempty"`
}

// AppRunnerAuthenticationConfiguration configures access to the source.
type AppRunnerAuthenticationConfiguration struct {
	// AccessRoleArn is a direct IAM role ARN granting App Runner access to
	// the ECR image (required for private ECR).
	// +optional
	AccessRoleArn string `json:"accessRoleArn,omitempty"`

	// AccessRoleRef references an IAMRole CR for the access role.
	// +optional
	AccessRoleRef *RoleRef `json:"accessRoleRef,omitempty"`
}

// AppRunnerSourceConfiguration describes the source deployed to the service.
type AppRunnerSourceConfiguration struct {
	// ImageRepository describes the source image repository.
	ImageRepository AppRunnerImageRepository `json:"imageRepository"`

	// AutoDeploymentsEnabled starts a deployment on each image change.
	// +optional
	AutoDeploymentsEnabled *bool `json:"autoDeploymentsEnabled,omitempty"`

	// AuthenticationConfiguration configures source access.
	// +optional
	AuthenticationConfiguration *AppRunnerAuthenticationConfiguration `json:"authenticationConfiguration,omitempty"`
}

// AppRunnerInstanceConfiguration configures service instances.
type AppRunnerInstanceConfiguration struct {
	// CPU units reserved for each instance (e.g. "1024", "1 vCPU").
	// +optional
	CPU string `json:"cpu,omitempty"`

	// Memory reserved for each instance (e.g. "2048", "2 GB").
	// +optional
	Memory string `json:"memory,omitempty"`

	// InstanceRoleArn is a direct IAM role ARN the service instances assume.
	// +optional
	InstanceRoleArn string `json:"instanceRoleArn,omitempty"`

	// InstanceRoleRef references an IAMRole CR for the instance role.
	// +optional
	InstanceRoleRef *RoleRef `json:"instanceRoleRef,omitempty"`
}

// AppRunnerHealthCheckConfiguration configures the service health check.
type AppRunnerHealthCheckConfiguration struct {
	// Protocol used for health checks (TCP or HTTP).
	// +kubebuilder:validation:Enum=TCP;HTTP
	// +optional
	Protocol string `json:"protocol,omitempty"`

	// Path health check requests are sent to (HTTP only).
	// +optional
	Path string `json:"path,omitempty"`

	// Interval between health checks in seconds.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Interval int32 `json:"interval,omitempty"`

	// Timeout for a health check response in seconds.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Timeout int32 `json:"timeout,omitempty"`

	// HealthyThreshold is consecutive successes before healthy.
	// +kubebuilder:validation:Minimum=1
	// +optional
	HealthyThreshold int32 `json:"healthyThreshold,omitempty"`

	// UnhealthyThreshold is consecutive failures before unhealthy.
	// +kubebuilder:validation:Minimum=1
	// +optional
	UnhealthyThreshold int32 `json:"unhealthyThreshold,omitempty"`
}

// AppRunnerServiceSpec defines the desired state of an App Runner service.
type AppRunnerServiceSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ServiceName is the name of the service. Immutable after creation.
	// +kubebuilder:validation:MinLength=4
	// +kubebuilder:validation:MaxLength=40
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serviceName is immutable"
	ServiceName string `json:"serviceName"`

	// SourceConfiguration describes the source deployed to the service.
	SourceConfiguration AppRunnerSourceConfiguration `json:"sourceConfiguration"`

	// InstanceConfiguration configures service instances.
	// +optional
	InstanceConfiguration *AppRunnerInstanceConfiguration `json:"instanceConfiguration,omitempty"`

	// HealthCheckConfiguration configures the service health check.
	// +optional
	HealthCheckConfiguration *AppRunnerHealthCheckConfiguration `json:"healthCheckConfiguration,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// AppRunnerServiceStatus defines the observed state of AppRunnerService.
type AppRunnerServiceStatus struct {
	// ServiceARN is the ARN of the App Runner service.
	// +optional
	ServiceARN string `json:"serviceArn,omitempty"`

	// ServiceID is the App Runner generated ID of the service.
	// +optional
	ServiceID string `json:"serviceId,omitempty"`

	// ServiceURL is the subdomain URL of the running service.
	// +optional
	ServiceURL string `json:"serviceUrl,omitempty"`

	// Status is the current service state (e.g. RUNNING).
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
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.serviceUrl"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AppRunnerService is the Schema for managing AWS App Runner services.
type AppRunnerService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppRunnerServiceSpec   `json:"spec,omitempty"`
	Status AppRunnerServiceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AppRunnerServiceList contains a list of AppRunnerService
type AppRunnerServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AppRunnerService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AppRunnerService{}, &AppRunnerServiceList{})
}
