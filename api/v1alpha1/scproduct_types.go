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

// SCProvisioningArtifact describes the initial provisioning artifact
// (version) of a Service Catalog product.
type SCProvisioningArtifact struct {
	// Name of the provisioning artifact (e.g. "v1").
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Description of the provisioning artifact.
	// +optional
	Description string `json:"description,omitempty"`

	// TemplateURL is the S3 (or GitHub) URL of the CloudFormation template,
	// passed as LoadTemplateFromURL.
	// +kubebuilder:validation:MinLength=1
	TemplateURL string `json:"templateUrl"`
}

// SCProductSpec defines the desired state of an AWS Service Catalog product.
type SCProductSpec struct {
	// Name of the product.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Owner of the product.
	// +kubebuilder:validation:MinLength=1
	Owner string `json:"owner"`

	// ProductType of the product. Immutable after creation.
	// +kubebuilder:validation:Enum=CLOUD_FORMATION_TEMPLATE
	// +kubebuilder:default=CLOUD_FORMATION_TEMPLATE
	// +optional
	ProductType string `json:"productType,omitempty"`

	// ProvisioningArtifact is the initial version of the product. Only used
	// at creation; add further versions outside this CR.
	ProvisioningArtifact SCProvisioningArtifact `json:"provisioningArtifact"`

	// Description of the product.
	// +optional
	Description string `json:"description,omitempty"`

	// Distributor of the product.
	// +optional
	Distributor string `json:"distributor,omitempty"`

	// SupportEmail is the contact email for product support.
	// +optional
	SupportEmail string `json:"supportEmail,omitempty"`

	// Tags are AWS resource tags to apply.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// SCProductStatus defines the observed state of SCProduct.
type SCProductStatus struct {
	// ProductID is the Service Catalog product identifier (prod-...).
	// +optional
	ProductID string `json:"productId,omitempty"`

	// ARN is the Amazon Resource Name of the product.
	// +optional
	ARN string `json:"arn,omitempty"`

	// ProvisioningArtifactID is the identifier of the initial provisioning
	// artifact created with the product.
	// +optional
	ProvisioningArtifactID string `json:"provisioningArtifactId,omitempty"`

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
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SCProduct is the Schema for managing AWS Service Catalog products.
type SCProduct struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SCProductSpec   `json:"spec,omitempty"`
	Status SCProductStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SCProductList contains a list of SCProduct
type SCProductList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SCProduct `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SCProduct{}, &SCProductList{})
}
