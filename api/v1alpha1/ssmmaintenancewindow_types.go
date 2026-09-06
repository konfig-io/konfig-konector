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

// SSMMaintenanceWindowSpec defines the desired state of an SSM maintenance window.
type SSMMaintenanceWindowSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// Name is the name of the maintenance window.
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=128
	Name string `json:"name"`

	// Schedule is a cron or rate expression, e.g. cron(0 4 ? * SUN *).
	// +kubebuilder:validation:MinLength=1
	Schedule string `json:"schedule"`

	// Duration of the maintenance window in hours.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=24
	Duration int32 `json:"duration"`

	// Cutoff is the number of hours before the end of the window that new
	// tasks stop being scheduled.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=23
	Cutoff int32 `json:"cutoff"`

	// AllowUnassociatedTargets permits running tasks on targets not
	// registered with the window.
	// +optional
	AllowUnassociatedTargets bool `json:"allowUnassociatedTargets,omitempty"`

	// Timezone in IANA format, e.g. America/Los_Angeles.
	// +optional
	Timezone string `json:"timezone,omitempty"`

	// Description of the maintenance window.
	// +optional
	Description string `json:"description,omitempty"`

	// Tags are AWS resource tags to apply on creation.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// SSMMaintenanceWindowStatus defines the observed state of SSMMaintenanceWindow.
type SSMMaintenanceWindowStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// WindowID is the AWS maintenance window ID (mw-...).
	// +optional
	WindowID string `json:"windowId,omitempty"`

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
// +kubebuilder:printcolumn:name="Window-ID",type="string",JSONPath=".status.windowId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SSMMaintenanceWindow is the Schema for managing SSM maintenance windows.
type SSMMaintenanceWindow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SSMMaintenanceWindowSpec   `json:"spec,omitempty"`
	Status SSMMaintenanceWindowStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SSMMaintenanceWindowList contains a list of SSMMaintenanceWindow
type SSMMaintenanceWindowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SSMMaintenanceWindow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SSMMaintenanceWindow{}, &SSMMaintenanceWindowList{})
}
