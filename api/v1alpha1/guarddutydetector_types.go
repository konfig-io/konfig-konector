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

// GuardDutyFeature configures a single GuardDuty detector feature.
type GuardDutyFeature struct {
	// Name of the feature (e.g. S3_DATA_EVENTS, EKS_AUDIT_LOGS,
	// EBS_MALWARE_PROTECTION, RDS_LOGIN_EVENTS, LAMBDA_NETWORK_LOGS,
	// RUNTIME_MONITORING).
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Status of the feature.
	// +kubebuilder:validation:Enum=ENABLED;DISABLED
	Status string `json:"status"`
}

// GuardDutyDetectorSpec defines the desired state of a GuardDuty detector.
// GuardDuty supports one detector per account per region.
type GuardDutyDetectorSpec struct {
	// Enable specifies whether the detector is enabled.
	// +optional
	// +kubebuilder:default=true
	Enable *bool `json:"enable,omitempty"`

	// FindingPublishingFrequency is how frequently updated findings are
	// exported.
	// +kubebuilder:validation:Enum=FIFTEEN_MINUTES;ONE_HOUR;SIX_HOURS
	// +optional
	FindingPublishingFrequency string `json:"findingPublishingFrequency,omitempty"`

	// Features configures detector features.
	// +optional
	Features []GuardDutyFeature `json:"features,omitempty"`

	// Tags are AWS resource tags applied when the detector is created.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// GuardDutyDetectorStatus defines the observed state of GuardDutyDetector.
type GuardDutyDetectorStatus struct {
	// DetectorID is the unique ID of the detector.
	// +optional
	DetectorID string `json:"detectorId,omitempty"`

	// DetectorStatus is the ENABLED/DISABLED status reported by AWS.
	// +optional
	DetectorStatus string `json:"detectorStatus,omitempty"`

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
// +kubebuilder:printcolumn:name="Detector-ID",type="string",JSONPath=".status.detectorId"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.detectorStatus"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// GuardDutyDetector is the Schema for managing GuardDuty detectors.
type GuardDutyDetector struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GuardDutyDetectorSpec   `json:"spec,omitempty"`
	Status GuardDutyDetectorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GuardDutyDetectorList contains a list of GuardDutyDetector
type GuardDutyDetectorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GuardDutyDetector `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GuardDutyDetector{}, &GuardDutyDetectorList{})
}
