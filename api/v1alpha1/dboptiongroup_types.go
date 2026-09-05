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

// DBOptionGroupOption defines a single option in the option group.
type DBOptionGroupOption struct {
	// OptionName is the name of the option (e.g. "MARIADB_AUDIT_PLUGIN").
	OptionName string `json:"optionName"`

	// Port is the port number for the option.
	// +optional
	Port int32 `json:"port,omitempty"`

	// OptionSettings are additional settings for the option.
	// +optional
	OptionSettings []DBOptionSetting `json:"optionSettings,omitempty"`
}

// DBOptionSetting defines a name/value setting for an option.
type DBOptionSetting struct {
	// Name of the setting.
	Name string `json:"name"`
	// Value of the setting.
	Value string `json:"value"`
}

// DBOptionGroupSpec defines the desired state of a DB Option Group.
type DBOptionGroupSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// OptionGroupName is the name of the option group. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="optionGroupName is immutable"
	OptionGroupName string `json:"optionGroupName"`

	// OptionGroupDescription is a description for the option group. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="optionGroupDescription is immutable"
	OptionGroupDescription string `json:"optionGroupDescription"`

	// EngineName is the DB engine (e.g. "mysql", "oracle-ee"). Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="engineName is immutable"
	EngineName string `json:"engineName"`

	// MajorEngineVersion is the engine version (e.g. "8.0"). Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="majorEngineVersion is immutable"
	MajorEngineVersion string `json:"majorEngineVersion"`

	// Options are the options to include in the group.
	// +optional
	Options []DBOptionGroupOption `json:"options,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// DBOptionGroupStatus defines the observed state of DBOptionGroup.
type DBOptionGroupStatus struct {
	// ARN is the ARN of the option group.
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
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.arn"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DBOptionGroup is the Schema for managing RDS DB Option Groups.
type DBOptionGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DBOptionGroupSpec   `json:"spec,omitempty"`
	Status DBOptionGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DBOptionGroupList contains a list of DBOptionGroup
type DBOptionGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DBOptionGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DBOptionGroup{}, &DBOptionGroupList{})
}
