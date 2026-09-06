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

// DBSnapshotSpec defines the desired state of a DB Snapshot.
type DBSnapshotSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// DBInstanceIdentifier is the identifier of the RDS instance to snapshot.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="dbInstanceIdentifier is immutable"
	DBInstanceIdentifier string `json:"dbInstanceIdentifier"`

	// DBSnapshotIdentifier is the identifier for this snapshot.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="dbSnapshotIdentifier is immutable"
	DBSnapshotIdentifier string `json:"dbSnapshotIdentifier"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// DBSnapshotStatus defines the observed state of DBSnapshot.
type DBSnapshotStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// SnapshotARN is the ARN of the snapshot.
	// +optional
	SnapshotARN string `json:"snapshotArn,omitempty"`

	// SnapshotStatus is the current state of the snapshot.
	// +optional
	SnapshotStatus string `json:"snapshotStatus,omitempty"`

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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.snapshotArn"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.snapshotStatus"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DBSnapshot is the Schema for managing RDS DB Snapshots.
type DBSnapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DBSnapshotSpec   `json:"spec,omitempty"`
	Status DBSnapshotStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DBSnapshotList contains a list of DBSnapshot.
type DBSnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DBSnapshot `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DBSnapshot{}, &DBSnapshotList{})
}
