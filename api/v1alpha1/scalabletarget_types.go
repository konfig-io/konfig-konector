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

// ScalableTargetSpec defines the desired state of an Application Auto Scaling
// scalable target.
type ScalableTargetSpec struct {
	// ServiceNamespace is the AWS service namespace of the scalable resource.
	// Immutable after creation.
	// +kubebuilder:validation:Enum=ecs;elasticmapreduce;ec2;appstream;dynamodb;rds;sagemaker;custom-resource;comprehend;lambda;cassandra;kafka;elasticache;neptune;workspaces
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serviceNamespace is immutable"
	ServiceNamespace string `json:"serviceNamespace"`

	// ResourceID identifies the resource, e.g. service/my-cluster/my-service
	// or table/my-table. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="resourceId is immutable"
	ResourceID string `json:"resourceId"`

	// ScalableDimension is the scalable property of the resource, e.g.
	// ecs:service:DesiredCount. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="scalableDimension is immutable"
	ScalableDimension string `json:"scalableDimension"`

	// MinCapacity is the minimum value that Application Auto Scaling can scale to.
	// +kubebuilder:validation:Minimum=0
	MinCapacity int32 `json:"minCapacity"`

	// MaxCapacity is the maximum value that Application Auto Scaling can scale to.
	// +kubebuilder:validation:Minimum=0
	MaxCapacity int32 `json:"maxCapacity"`

	// RoleARN is the ARN of the IAM role that allows Application Auto Scaling
	// to modify the scalable target. Most services use a service-linked role
	// and this can be omitted.
	// +optional
	RoleARN string `json:"roleArn,omitempty"`

	// Tags are AWS resource tags to apply on registration.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ScalableTargetStatus defines the observed state of ScalableTarget.
type ScalableTargetStatus struct {
	// ScalableTargetARN is the ARN of the scalable target.
	// +optional
	ScalableTargetARN string `json:"scalableTargetArn,omitempty"`

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
// +kubebuilder:printcolumn:name="Resource-ID",type="string",JSONPath=".spec.resourceId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ScalableTarget is the Schema for managing Application Auto Scaling scalable targets.
type ScalableTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScalableTargetSpec   `json:"spec,omitempty"`
	Status ScalableTargetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ScalableTargetList contains a list of ScalableTarget
type ScalableTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScalableTarget `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScalableTarget{}, &ScalableTargetList{})
}
