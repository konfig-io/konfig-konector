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

// LambdaLayerContent defines the content for a Lambda layer.
type LambdaLayerContent struct {
	// S3Bucket is the S3 bucket containing the layer archive.
	// +optional
	S3Bucket string `json:"s3Bucket,omitempty"`

	// S3Key is the S3 object key for the layer archive.
	// +optional
	S3Key string `json:"s3Key,omitempty"`

	// S3ObjectVersion is the S3 object version.
	// +optional
	S3ObjectVersion string `json:"s3ObjectVersion,omitempty"`
}

// LambdaLayerVersionSpec defines the desired state of a Lambda Layer Version.
type LambdaLayerVersionSpec struct {
	// LayerName is the name of the layer.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="layerName is immutable"
	LayerName string `json:"layerName"`

	// Content is the layer archive source.
	Content LambdaLayerContent `json:"content"`

	// Description is an optional description.
	// +optional
	Description string `json:"description,omitempty"`

	// CompatibleRuntimes lists the runtimes compatible with this layer.
	// +optional
	CompatibleRuntimes []string `json:"compatibleRuntimes,omitempty"`

	// CompatibleArchitectures lists the architectures compatible with this layer.
	// +optional
	CompatibleArchitectures []string `json:"compatibleArchitectures,omitempty"`

	// LicenseInfo provides license details.
	// +optional
	LicenseInfo string `json:"licenseInfo,omitempty"`
}

// LambdaLayerVersionStatus defines the observed state of LambdaLayerVersion.
type LambdaLayerVersionStatus struct {
	// LayerVersionARN is the ARN of the layer version.
	// +optional
	LayerVersionARN string `json:"layerVersionArn,omitempty"`

	// Version is the version number.
	// +optional
	Version int64 `json:"version,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.layerVersionArn"
// +kubebuilder:printcolumn:name="Version",type="integer",JSONPath=".status.version"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// LambdaLayerVersion is the Schema for managing Lambda layer versions.
type LambdaLayerVersion struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LambdaLayerVersionSpec   `json:"spec,omitempty"`
	Status LambdaLayerVersionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// LambdaLayerVersionList contains a list of LambdaLayerVersion.
type LambdaLayerVersionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LambdaLayerVersion `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LambdaLayerVersion{}, &LambdaLayerVersionList{})
}
