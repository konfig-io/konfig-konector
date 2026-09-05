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

// SCPortfolioSpec defines the desired state of an AWS Service Catalog portfolio.
type SCPortfolioSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// DisplayName is the portfolio name shown to users.
	// +kubebuilder:validation:MinLength=1
	DisplayName string `json:"displayName"`

	// ProviderName is the name of the portfolio provider.
	// +kubebuilder:validation:MinLength=1
	ProviderName string `json:"providerName"`

	// Description of the portfolio.
	// +optional
	Description string `json:"description,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// SCPortfolioStatus defines the observed state of SCPortfolio.
type SCPortfolioStatus struct {
	// PortfolioID is the Service Catalog portfolio identifier (port-...).
	// +optional
	PortfolioID string `json:"portfolioId,omitempty"`

	// ARN is the Amazon Resource Name of the portfolio.
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
// +kubebuilder:printcolumn:name="Portfolio-ID",type="string",JSONPath=".status.portfolioId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SCPortfolio is the Schema for managing AWS Service Catalog portfolios.
type SCPortfolio struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SCPortfolioSpec   `json:"spec,omitempty"`
	Status SCPortfolioStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SCPortfolioList contains a list of SCPortfolio
type SCPortfolioList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SCPortfolio `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SCPortfolio{}, &SCPortfolioList{})
}
