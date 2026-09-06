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

// S3BucketCORSSpec defines the desired state of S3BucketCORS.
type S3BucketCORSSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// BucketName is the name of the S3 bucket.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="bucketName is immutable"
	BucketName string `json:"bucketName"`

	// CORSRules are the CORS rules to apply.
	// +kubebuilder:validation:MinItems=1
	CORSRules []S3CORSRule `json:"corsRules"`
}

// S3BucketCORSStatus defines the observed state of S3BucketCORS.
type S3BucketCORSStatus struct {
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
// +kubebuilder:printcolumn:name="Bucket",type="string",JSONPath=".spec.bucketName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// S3BucketCORS is the Schema for managing S3 bucket CORS configurations.
type S3BucketCORS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   S3BucketCORSSpec   `json:"spec,omitempty"`
	Status S3BucketCORSStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// S3BucketCORSList contains a list of S3BucketCORS.
type S3BucketCORSList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []S3BucketCORS `json:"items"`
}

func init() {
	SchemeBuilder.Register(&S3BucketCORS{}, &S3BucketCORSList{})
}
