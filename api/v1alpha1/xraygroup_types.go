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

// XRayInsightsConfiguration configures X-Ray insights for a group.
type XRayInsightsConfiguration struct {
	// Enabled turns on insights for the group.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// NotificationsEnabled turns on insights notifications. Notifications can
	// only be enabled when insights are enabled.
	// +optional
	NotificationsEnabled bool `json:"notificationsEnabled,omitempty"`
}

// XRayGroupSpec defines the desired state of an X-Ray group.
type XRayGroupSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// GroupName is the case-sensitive name of the group. Immutable.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=32
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="groupName is immutable"
	GroupName string `json:"groupName"`

	// FilterExpression defines criteria by which to group traces. AWS requires
	// one, e.g. service("api") or fault = true.
	// +kubebuilder:validation:MinLength=1
	// +optional
	FilterExpression string `json:"filterExpression"`

	// InsightsConfiguration configures insights and insight notifications.
	// +optional
	InsightsConfiguration *XRayInsightsConfiguration `json:"insightsConfiguration,omitempty"`

	// Tags are AWS resource tags to apply at creation time.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// XRayGroupStatus defines the observed state of XRayGroup.
type XRayGroupStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// ARN is the ARN of the group.
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
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// XRayGroup is the Schema for managing X-Ray groups.
type XRayGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   XRayGroupSpec   `json:"spec,omitempty"`
	Status XRayGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// XRayGroupList contains a list of XRayGroup
type XRayGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []XRayGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&XRayGroup{}, &XRayGroupList{})
}
