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

// S3BucketRef references a bucket by CR name or direct bucket name.
type S3BucketRef struct {
	// Name is the name of an S3Bucket CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`

	// BucketName is a direct S3 bucket name (bypasses CR lookup).
	// +optional
	BucketName string `json:"bucketName,omitempty"`
}

// S3BucketPolicySpec defines the desired state of an S3 Bucket Policy.
type S3BucketPolicySpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// BucketRef references the target bucket.
	BucketRef S3BucketRef `json:"bucketRef"`

	// PolicyDocument is the JSON IAM resource policy to attach to the bucket.
	// +kubebuilder:validation:MinLength=1
	PolicyDocument string `json:"policyDocument"`
}

// S3BucketPolicyStatus defines the observed state of S3BucketPolicy.
type S3BucketPolicyStatus struct {
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

// S3BucketPolicy is the Schema for managing S3 Bucket Policies.
type S3BucketPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   S3BucketPolicySpec   `json:"spec,omitempty"`
	Status S3BucketPolicyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// S3BucketPolicyList contains a list of S3BucketPolicy
type S3BucketPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []S3BucketPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&S3BucketPolicy{}, &S3BucketPolicyList{})
}
