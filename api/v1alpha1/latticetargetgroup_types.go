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

// LatticeHealthCheck configures target health checks for a VPC Lattice target group.
type LatticeHealthCheck struct {
	// Path is the destination for health checks on the targets.
	// +optional
	Path string `json:"path,omitempty"`
	// Protocol used for health checks (HTTP or HTTPS).
	// +kubebuilder:validation:Enum=HTTP;HTTPS
	// +optional
	Protocol string `json:"protocol,omitempty"`
}

// LatticeTargetGroupConfig holds the protocol/port/VPC configuration for
// non-Lambda target groups.
type LatticeTargetGroupConfig struct {
	// Port on which the targets listen.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`

	// Protocol to use for routing traffic to the targets.
	// +kubebuilder:validation:Enum=HTTP;HTTPS;TCP
	// +optional
	Protocol string `json:"protocol,omitempty"`

	// VPCRef references the VPC of the targets.
	// +optional
	VPCRef *VPCResourceRef `json:"vpcRef,omitempty"`

	// HealthCheck configures target health checks.
	// +optional
	HealthCheck *LatticeHealthCheck `json:"healthCheck,omitempty"`
}

// LatticeTargetGroupSpec defines the desired state of a VPC Lattice target group.
type LatticeTargetGroupSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name of the target group. Immutable after creation.
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Type of target group. Immutable after creation.
	// +kubebuilder:validation:Enum=INSTANCE;IP;LAMBDA;ALB
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="type is immutable"
	Type string `json:"type"`

	// Config holds the protocol/port/VPC configuration. Required for all
	// types except LAMBDA.
	// +optional
	Config *LatticeTargetGroupConfig `json:"config,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// LatticeTargetGroupStatus defines the observed state of LatticeTargetGroup.
type LatticeTargetGroupStatus struct {
	// ARN is the Amazon Resource Name of the target group.
	// +optional
	ARN string `json:"arn,omitempty"`

	// ID is the unique identifier of the target group.
	// +optional
	ID string `json:"id,omitempty"`

	// Status is the lifecycle status reported by AWS.
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
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.id"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// LatticeTargetGroup is the Schema for managing VPC Lattice target groups.
type LatticeTargetGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LatticeTargetGroupSpec   `json:"spec,omitempty"`
	Status LatticeTargetGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// LatticeTargetGroupList contains a list of LatticeTargetGroup
type LatticeTargetGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LatticeTargetGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LatticeTargetGroup{}, &LatticeTargetGroupList{})
}
