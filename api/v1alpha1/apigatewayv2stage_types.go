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

// APIGatewayV2RouteSettings configures throttling for a stage's routes.
type APIGatewayV2RouteSettings struct {
	// ThrottlingBurstLimit is the throttling burst limit.
	// +optional
	ThrottlingBurstLimit *int32 `json:"throttlingBurstLimit,omitempty"`

	// ThrottlingRateLimit is the throttling rate limit (requests per second).
	// +optional
	ThrottlingRateLimit *float64 `json:"throttlingRateLimit,omitempty"`

	// DetailedMetricsEnabled enables detailed CloudWatch metrics.
	// +optional
	DetailedMetricsEnabled *bool `json:"detailedMetricsEnabled,omitempty"`
}

// APIGatewayV2StageSpec defines the desired state of an API Gateway v2 stage.
type APIGatewayV2StageSpec struct {
	// APIRef references the APIGatewayV2API this stage belongs to.
	APIRef APIRef `json:"apiRef"`

	// StageName is the name of the stage (e.g. "$default", "prod").
	// Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="stageName is immutable"
	StageName string `json:"stageName"`

	// AutoDeploy enables automatic deployments on API updates.
	// +optional
	AutoDeploy bool `json:"autoDeploy,omitempty"`

	// Description of the stage.
	// +optional
	Description string `json:"description,omitempty"`

	// StageVariables are stage variables (name/value pairs).
	// +optional
	StageVariables map[string]string `json:"stageVariables,omitempty"`

	// DefaultRouteSettings is the default route throttling for the stage.
	// +optional
	DefaultRouteSettings *APIGatewayV2RouteSettings `json:"defaultRouteSettings,omitempty"`

	// Tags are AWS resource tags to apply on creation.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// APIGatewayV2StageStatus defines the observed state of APIGatewayV2Stage.
type APIGatewayV2StageStatus struct {
	// StageName is the stage name in AWS (also the primary identifier
	// together with the API ID).
	// +optional
	StageName string `json:"stageName,omitempty"`

	// APIID is the resolved API identifier the stage was created in.
	// +optional
	APIID string `json:"apiId,omitempty"`

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
// +kubebuilder:printcolumn:name="Stage",type="string",JSONPath=".status.stageName"
// +kubebuilder:printcolumn:name="API-ID",type="string",JSONPath=".status.apiId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// APIGatewayV2Stage is the Schema for managing API Gateway v2 stages.
type APIGatewayV2Stage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   APIGatewayV2StageSpec   `json:"spec,omitempty"`
	Status APIGatewayV2StageStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// APIGatewayV2StageList contains a list of APIGatewayV2Stage
type APIGatewayV2StageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []APIGatewayV2Stage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&APIGatewayV2Stage{}, &APIGatewayV2StageList{})
}
