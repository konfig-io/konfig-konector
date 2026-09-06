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

// ECSNetworkConfiguration defines the VPC networking for an ECS service or task.
type ECSNetworkConfiguration struct {
	// SubnetRefs are the subnets to launch tasks in.
	SubnetRefs []SubnetRef `json:"subnetRefs"`
	// SecurityGroupRefs are the security groups to attach to tasks.
	// +optional
	SecurityGroupRefs []SecurityGroupRef `json:"securityGroupRefs,omitempty"`
	// AssignPublicIP controls whether tasks get a public IP. Default: DISABLED.
	// +kubebuilder:validation:Enum=ENABLED;DISABLED
	// +optional
	AssignPublicIP string `json:"assignPublicIp,omitempty"`
}

// ECSLoadBalancer configures an ALB/NLB target group for service load balancing.
type ECSLoadBalancer struct {
	// TargetGroupArn is the ARN of the ALB/NLB target group.
	TargetGroupArn string `json:"targetGroupArn"`
	// ContainerName is the name of the container to register with the target group.
	ContainerName string `json:"containerName"`
	// ContainerPort is the container port to route traffic to.
	ContainerPort int32 `json:"containerPort"`
}

// ECSServiceSpec defines the desired state of an ECS Service.
type ECSServiceSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// ClusterRef references the ECS cluster to run the service in.
	// Either clusterRef or clusterName must be set.
	// +optional
	ClusterRef *ECSClusterRef `json:"clusterRef,omitempty"`

	// ClusterName is a direct ECS cluster name or ARN.
	// +optional
	ClusterName string `json:"clusterName,omitempty"`

	// ServiceName is the name of the ECS service. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serviceName is immutable"
	ServiceName string `json:"serviceName"`

	// TaskDefinitionRef references the task definition to run.
	// Either taskDefinitionRef or taskDefinitionArn must be set.
	// +optional
	TaskDefinitionRef *ECSTaskDefinitionRef `json:"taskDefinitionRef,omitempty"`

	// TaskDefinitionArn is a direct task definition ARN (with revision).
	// +optional
	TaskDefinitionArn string `json:"taskDefinitionArn,omitempty"`

	// DesiredCount is the number of task instances to run.
	// +kubebuilder:validation:Minimum=0
	DesiredCount int32 `json:"desiredCount"`

	// LaunchType is the launch type for tasks.
	// +kubebuilder:validation:Enum=FARGATE;EC2;EXTERNAL
	// +optional
	LaunchType string `json:"launchType,omitempty"`

	// NetworkConfiguration is required for awsvpc network mode tasks.
	// +optional
	NetworkConfiguration *ECSNetworkConfiguration `json:"networkConfiguration,omitempty"`

	// LoadBalancers connect the service to ALB/NLB target groups.
	// +optional
	LoadBalancers []ECSLoadBalancer `json:"loadBalancers,omitempty"`

	// HealthCheckGracePeriodSeconds is the grace period before health check starts.
	// +optional
	HealthCheckGracePeriodSeconds *int32 `json:"healthCheckGracePeriodSeconds,omitempty"`

	// EnableExecuteCommand enables ECS Exec on tasks.
	// +optional
	EnableExecuteCommand *bool `json:"enableExecuteCommand,omitempty"`

	// Tags are AWS resource tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ECSServiceStatus defines the observed state of ECSService.
type ECSServiceStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ServiceARN is the ARN of the ECS service.
	// +optional
	ServiceARN string `json:"serviceArn,omitempty"`

	// Status is the service status: ACTIVE, DRAINING, INACTIVE.
	// +optional
	Status string `json:"status,omitempty"`

	// RunningCount is the number of currently running task instances.
	// +optional
	RunningCount int32 `json:"runningCount,omitempty"`

	// PendingCount is the number of tasks in pending state.
	// +optional
	PendingCount int32 `json:"pendingCount,omitempty"`

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
// +kubebuilder:printcolumn:name="Service",type="string",JSONPath=".spec.serviceName"
// +kubebuilder:printcolumn:name="Desired",type="integer",JSONPath=".spec.desiredCount"
// +kubebuilder:printcolumn:name="Running",type="integer",JSONPath=".status.runningCount"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ECSService is the Schema for managing ECS Services.
type ECSService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ECSServiceSpec   `json:"spec,omitempty"`
	Status ECSServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ECSServiceList contains a list of ECSService.
type ECSServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ECSService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ECSService{}, &ECSServiceList{})
}
