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

// SCProductRef references either a managed SCProduct CR or a direct product ID.
type SCProductRef struct {
	// Name of an SCProduct CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// ProductID is a direct Service Catalog product ID (prod-...).
	// If set, Name is ignored.
	// +optional
	ProductID string `json:"productId,omitempty"`
}

// SCPortfolioRef references either a managed SCPortfolio CR or a direct
// portfolio ID.
type SCPortfolioRef struct {
	// Name of an SCPortfolio CR in the same namespace.
	// +optional
	Name string `json:"name,omitempty"`
	// PortfolioID is a direct Service Catalog portfolio ID (port-...).
	// If set, Name is ignored.
	// +optional
	PortfolioID string `json:"portfolioId,omitempty"`
}

// SCPortfolioProductAssociationSpec associates a Service Catalog product with
// a portfolio.
type SCPortfolioProductAssociationSpec struct {
	// ProductRef references the product to associate.
	ProductRef SCProductRef `json:"productRef"`

	// PortfolioRef references the portfolio to associate the product with.
	PortfolioRef SCPortfolioRef `json:"portfolioRef"`
}

// SCPortfolioProductAssociationStatus defines the observed state of
// SCPortfolioProductAssociation.
type SCPortfolioProductAssociationStatus struct {
	// ProductID is the resolved product ID.
	// +optional
	ProductID string `json:"productId,omitempty"`

	// PortfolioID is the resolved portfolio ID.
	// +optional
	PortfolioID string `json:"portfolioId,omitempty"`

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
// +kubebuilder:printcolumn:name="Product-ID",type="string",JSONPath=".status.productId"
// +kubebuilder:printcolumn:name="Portfolio-ID",type="string",JSONPath=".status.portfolioId"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SCPortfolioProductAssociation is the Schema for managing Service Catalog
// product-portfolio associations.
type SCPortfolioProductAssociation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SCPortfolioProductAssociationSpec   `json:"spec,omitempty"`
	Status SCPortfolioProductAssociationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SCPortfolioProductAssociationList contains a list of SCPortfolioProductAssociation
type SCPortfolioProductAssociationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SCPortfolioProductAssociation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SCPortfolioProductAssociation{}, &SCPortfolioProductAssociationList{})
}
