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

// LaunchTemplateRef references a LaunchTemplate CR or uses a direct template ID/name.
type LaunchTemplateRef struct {
	// Name is the name of a LaunchTemplate CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`

	// ID is a direct AWS Launch Template ID.
	// +optional
	ID string `json:"id,omitempty"`

	// Version is the launch template version (e.g. "$Latest", "$Default", or a number).
	// +optional
	Version string `json:"version,omitempty"`
}

// AutoScalingGroupSpec defines the desired state of an EC2 Auto Scaling Group.
type AutoScalingGroupSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// AutoScalingGroupName is the name of the Auto Scaling group. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="autoScalingGroupName is immutable"
	AutoScalingGroupName string `json:"autoScalingGroupName"`

	// LaunchTemplateRef references the launch template to use.
	LaunchTemplateRef LaunchTemplateRef `json:"launchTemplateRef"`

	// MinSize is the minimum number of instances.
	// +kubebuilder:validation:Minimum=0
	MinSize int32 `json:"minSize"`

	// MaxSize is the maximum number of instances.
	// +kubebuilder:validation:Minimum=0
	MaxSize int32 `json:"maxSize"`

	// DesiredCapacity is the desired number of instances.
	// +optional
	DesiredCapacity *int32 `json:"desiredCapacity,omitempty"`

	// VPCZoneIdentifier is the list of subnet references for the ASG.
	// +optional
	VPCZoneIdentifier []SubnetRef `json:"vpcZoneIdentifier,omitempty"`

	// TargetGroupARNs is the list of ALB/NLB target group ARNs to attach.
	// +optional
	TargetGroupARNs []string `json:"targetGroupArns,omitempty"`

	// Tags are AWS resource tags to apply (also propagated to instances).
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// AutoScalingGroupStatus defines the observed state of AutoScalingGroup.
type AutoScalingGroupStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
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
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AutoScalingGroup is the Schema for managing EC2 Auto Scaling Groups.
type AutoScalingGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AutoScalingGroupSpec   `json:"spec,omitempty"`
	Status AutoScalingGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AutoScalingGroupList contains a list of AutoScalingGroup
type AutoScalingGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AutoScalingGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AutoScalingGroup{}, &AutoScalingGroupList{})
}
