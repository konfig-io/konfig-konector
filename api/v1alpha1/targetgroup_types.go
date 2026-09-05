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

// TargetGroupHealthCheck defines health check settings for a target group.
type TargetGroupHealthCheck struct {
	// Protocol is the health check protocol.
	// +kubebuilder:validation:Enum=HTTP;HTTPS;TCP;TLS;UDP;TCP_UDP
	// +optional
	Protocol string `json:"protocol,omitempty"`

	// Port is the health check port (default "traffic-port").
	// +optional
	Port string `json:"port,omitempty"`

	// Path is the health check path (for HTTP/HTTPS).
	// +optional
	Path string `json:"path,omitempty"`

	// HealthyThreshold is the number of consecutive successes.
	// +kubebuilder:validation:Minimum=2
	// +kubebuilder:validation:Maximum=10
	// +optional
	HealthyThreshold int32 `json:"healthyThreshold,omitempty"`

	// UnhealthyThreshold is the number of consecutive failures.
	// +kubebuilder:validation:Minimum=2
	// +kubebuilder:validation:Maximum=10
	// +optional
	UnhealthyThreshold int32 `json:"unhealthyThreshold,omitempty"`

	// IntervalSeconds is the health check interval.
	// +kubebuilder:validation:Minimum=5
	// +kubebuilder:validation:Maximum=300
	// +optional
	IntervalSeconds int32 `json:"intervalSeconds,omitempty"`
}

// TargetGroupSpec defines the desired state of a Target Group.
type TargetGroupSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the name of the target group. Immutable.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=32
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Protocol is the routing protocol.
	// +kubebuilder:validation:Enum=HTTP;HTTPS;TCP;TLS;UDP;TCP_UDP;GENEVE
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="protocol is immutable"
	Protocol string `json:"protocol"`

	// Port is the port on which targets receive traffic. Immutable.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="port is immutable"
	Port int32 `json:"port"`

	// TargetType is the target type.
	// +kubebuilder:validation:Enum=instance;ip;lambda;alb
	// +optional
	TargetType string `json:"targetType,omitempty"`

	// VPCRef is required for instance, ip, or alb target types.
	// +optional
	VPCRef *VPCResourceRef `json:"vpcRef,omitempty"`

	// HealthCheck defines the health check configuration.
	// +optional
	HealthCheck *TargetGroupHealthCheck `json:"healthCheck,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// TargetGroupStatus defines the observed state of TargetGroup.
type TargetGroupStatus struct {
	// ARN is the ARN of the target group.
	// +optional
	ARN string `json:"arn,omitempty"`

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
// +kubebuilder:printcolumn:name="Protocol",type="string",JSONPath=".spec.protocol"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// TargetGroup is the Schema for managing ELBv2 Target Groups.
type TargetGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TargetGroupSpec   `json:"spec,omitempty"`
	Status TargetGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TargetGroupList contains a list of TargetGroup
type TargetGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TargetGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TargetGroup{}, &TargetGroupList{})
}
