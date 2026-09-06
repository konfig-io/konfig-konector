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

// HostedZoneIDRef references a HostedZone CR or a direct hosted zone ID.
type HostedZoneIDRef struct {
	// Name of a HostedZone CR.
	// +optional
	Name string `json:"name,omitempty"`
	// Namespace of the HostedZone CR. Defaults to the referring object's namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// ID is a direct hosted zone ID (e.g. Z0123456789).
	// +optional
	ID string `json:"id,omitempty"`
}

// HostedZoneVPCAssociationSpec associates a VPC with a private hosted zone.
// The zone is owned by this resource's provider; the VPC may live in another
// account (vpcProviderRef), in which case the controller performs the full
// authorization handshake (CreateVPCAssociationAuthorization in the zone
// account, AssociateVPCWithHostedZone in the VPC account, then cleanup).
type HostedZoneVPCAssociationSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// HostedZoneRef is the private hosted zone.
	HostedZoneRef HostedZoneIDRef `json:"hostedZoneRef"`

	// VPCRef is the VPC to associate (CR name or direct ID).
	VPCRef VPCResourceRef `json:"vpcRef"`

	// VPCRegion is the region of the VPC.
	// +kubebuilder:validation:MinLength=1
	VPCRegion string `json:"vpcRegion"`

	// VPCProviderRef names the AWSProvider of the account owning the VPC when
	// it differs from the zone's account.
	// +optional
	VPCProviderRef *ProviderRef `json:"vpcProviderRef,omitempty"`
}

// HostedZoneVPCAssociationStatus defines the observed state.
type HostedZoneVPCAssociationStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// HostedZoneID and VPCID are the resolved identifiers.
	// +optional
	HostedZoneID string `json:"hostedZoneId,omitempty"`
	// +optional
	VPCID string `json:"vpcId,omitempty"`
	// Associated is true once AWS reports the VPC on the zone.
	// +optional
	Associated bool `json:"associated,omitempty"`
	// Conditions describe the current state of the resource.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Zone",type="string",JSONPath=".status.hostedZoneId"
// +kubebuilder:printcolumn:name="VPC",type="string",JSONPath=".status.vpcId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// HostedZoneVPCAssociation associates a VPC (optionally in another account)
// with a Route53 private hosted zone.
type HostedZoneVPCAssociation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HostedZoneVPCAssociationSpec   `json:"spec,omitempty"`
	Status HostedZoneVPCAssociationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HostedZoneVPCAssociationList contains a list of HostedZoneVPCAssociation.
type HostedZoneVPCAssociationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HostedZoneVPCAssociation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HostedZoneVPCAssociation{}, &HostedZoneVPCAssociationList{})
}
