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

// AthenaEncryptionConfiguration configures query result encryption.
type AthenaEncryptionConfiguration struct {
	// EncryptionOption is SSE_S3, SSE_KMS, or CSE_KMS.
	// +kubebuilder:validation:Enum=SSE_S3;SSE_KMS;CSE_KMS
	EncryptionOption string `json:"encryptionOption"`

	// KMSKey is the KMS key ARN or ID for SSE_KMS and CSE_KMS.
	// +optional
	KMSKey string `json:"kmsKey,omitempty"`
}

// AthenaResultConfiguration configures where query results are stored.
type AthenaResultConfiguration struct {
	// OutputLocation is the S3 location where query results are stored,
	// e.g. s3://bucket/prefix/.
	// +optional
	OutputLocation string `json:"outputLocation,omitempty"`

	// EncryptionConfiguration configures result encryption.
	// +optional
	EncryptionConfiguration *AthenaEncryptionConfiguration `json:"encryptionConfiguration,omitempty"`
}

// AthenaWorkGroupSpec defines the desired state of an Athena workgroup.
type AthenaWorkGroupSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the workgroup name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Description is a description of the workgroup.
	// +optional
	Description string `json:"description,omitempty"`

	// ResultConfiguration configures where and how query results are stored.
	// +optional
	ResultConfiguration *AthenaResultConfiguration `json:"resultConfiguration,omitempty"`

	// EnforceWorkGroupConfiguration makes workgroup settings override
	// client-side settings.
	// +optional
	EnforceWorkGroupConfiguration *bool `json:"enforceWorkGroupConfiguration,omitempty"`

	// PublishCloudWatchMetricsEnabled publishes query metrics to CloudWatch.
	// +optional
	PublishCloudWatchMetricsEnabled *bool `json:"publishCloudWatchMetricsEnabled,omitempty"`

	// BytesScannedCutoffPerQuery is the per-query data scan limit in bytes.
	// +kubebuilder:validation:Minimum=10000000
	// +optional
	BytesScannedCutoffPerQuery *int64 `json:"bytesScannedCutoffPerQuery,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// AthenaWorkGroupStatus defines the observed state of AthenaWorkGroup.
type AthenaWorkGroupStatus struct {
	// WorkGroupName is the name of the workgroup in AWS.
	// +optional
	WorkGroupName string `json:"workGroupName,omitempty"`

	// State is the workgroup state (ENABLED/DISABLED).
	// +optional
	State string `json:"state,omitempty"`

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
// +kubebuilder:printcolumn:name="WorkGroup",type="string",JSONPath=".status.workGroupName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AthenaWorkGroup is the Schema for managing Athena workgroups.
type AthenaWorkGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AthenaWorkGroupSpec   `json:"spec,omitempty"`
	Status AthenaWorkGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AthenaWorkGroupList contains a list of AthenaWorkGroup
type AthenaWorkGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AthenaWorkGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AthenaWorkGroup{}, &AthenaWorkGroupList{})
}
