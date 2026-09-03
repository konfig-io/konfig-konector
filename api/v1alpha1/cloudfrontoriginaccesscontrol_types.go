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

// CloudFrontOriginAccessControlSpec defines the desired state of a CloudFront Origin Access Control.
type CloudFrontOriginAccessControlSpec struct {
	// Name is the name for the origin access control.
	Name string `json:"name"`

	// OriginAccessControlOriginType is the type of origin this OAC is for.
	// +kubebuilder:validation:Enum=s3;mediastore;mediapackagev2;lambda
	OriginAccessControlOriginType string `json:"originAccessControlOriginType"`

	// SigningBehavior specifies which requests CloudFront signs.
	// +kubebuilder:validation:Enum=always;never;no-override
	SigningBehavior string `json:"signingBehavior"`

	// SigningProtocol is the signing protocol.
	// +kubebuilder:validation:Enum=sigv4
	// +optional
	SigningProtocol string `json:"signingProtocol,omitempty"`

	// Description is an optional description.
	// +optional
	Description string `json:"description,omitempty"`
}

// CloudFrontOriginAccessControlStatus defines the observed state of CloudFrontOriginAccessControl.
type CloudFrontOriginAccessControlStatus struct {
	// ID is the ID of the origin access control.
	// +optional
	ID string `json:"id,omitempty"`

	// ETag is the ETag for the origin access control.
	// +optional
	ETag string `json:"etag,omitempty"`

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

// CloudFrontOriginAccessControl is the Schema for managing CloudFront origin access controls.
type CloudFrontOriginAccessControl struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudFrontOriginAccessControlSpec   `json:"spec,omitempty"`
	Status CloudFrontOriginAccessControlStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CloudFrontOriginAccessControlList contains a list of CloudFrontOriginAccessControl.
type CloudFrontOriginAccessControlList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudFrontOriginAccessControl `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudFrontOriginAccessControl{}, &CloudFrontOriginAccessControlList{})
}
