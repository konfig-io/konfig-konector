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

// S3AccessPointPublicAccessBlock configures public access blocking for an
// access point.
type S3AccessPointPublicAccessBlock struct {
	// BlockPublicAcls blocks public ACLs.
	// +optional
	BlockPublicAcls bool `json:"blockPublicAcls,omitempty"`

	// BlockPublicPolicy blocks public bucket policies.
	// +optional
	BlockPublicPolicy bool `json:"blockPublicPolicy,omitempty"`

	// IgnorePublicAcls ignores public ACLs.
	// +optional
	IgnorePublicAcls bool `json:"ignorePublicAcls,omitempty"`

	// RestrictPublicBuckets restricts public bucket policies.
	// +optional
	RestrictPublicBuckets bool `json:"restrictPublicBuckets,omitempty"`
}

// S3AccessPointVPCConfiguration restricts the access point to a VPC.
type S3AccessPointVPCConfiguration struct {
	// VPCRef references the VPC (by VPC CR name or direct VPC ID).
	VPCRef VPCResourceRef `json:"vpcRef"`
}

// S3AccessPointSpec defines the desired state of an S3 access point.
type S3AccessPointSpec struct {
	// Name is the name of the access point. Immutable after creation.
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=50
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// AccountID is the AWS account ID that owns the bucket. Required on all
	// S3 Control API calls. Immutable after creation.
	// +kubebuilder:validation:Pattern=`^[0-9]{12}$`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="accountId is immutable"
	AccountID string `json:"accountId"`

	// BucketRef references the bucket the access point is attached to
	// (by S3Bucket CR name or direct bucket name).
	BucketRef S3BucketRef `json:"bucketRef"`

	// VPCConfiguration restricts access to the given VPC. Immutable in AWS;
	// changing it requires recreating the access point.
	// +optional
	VPCConfiguration *S3AccessPointVPCConfiguration `json:"vpcConfiguration,omitempty"`

	// PublicAccessBlock configures public access blocking.
	// +optional
	PublicAccessBlock *S3AccessPointPublicAccessBlock `json:"publicAccessBlock,omitempty"`
}

// S3AccessPointStatus defines the observed state of S3AccessPoint.
type S3AccessPointStatus struct {
	// ARN is the ARN of the access point.
	// +optional
	ARN string `json:"arn,omitempty"`

	// Alias is the bucket-style alias of the access point.
	// +optional
	Alias string `json:"alias,omitempty"`

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
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// S3AccessPoint is the Schema for managing S3 access points.
type S3AccessPoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   S3AccessPointSpec   `json:"spec,omitempty"`
	Status S3AccessPointStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// S3AccessPointList contains a list of S3AccessPoint
type S3AccessPointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []S3AccessPoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&S3AccessPoint{}, &S3AccessPointList{})
}
