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

// APIGatewayV2DomainNameSpec defines the desired state of an API Gateway v2 domain name.
type APIGatewayV2DomainNameSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// DomainName is the custom domain name (e.g. api.example.com).
	// Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="domainName is immutable"
	DomainName string `json:"domainName"`

	// CertificateARN is the ACM certificate ARN for the domain.
	CertificateARN string `json:"certificateArn"`

	// EndpointType is the endpoint type.
	// +kubebuilder:validation:Enum=REGIONAL;EDGE
	// +optional
	EndpointType string `json:"endpointType,omitempty"`

	// SecurityPolicy is the TLS security policy.
	// +kubebuilder:validation:Enum=TLS_1_0;TLS_1_2
	// +optional
	SecurityPolicy string `json:"securityPolicy,omitempty"`

	// Tags are AWS resource tags to apply on creation.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// APIGatewayV2DomainNameStatus defines the observed state of APIGatewayV2DomainName.
type APIGatewayV2DomainNameStatus struct {
	// DomainName is the domain name in AWS (also the primary identifier).
	// +optional
	DomainName string `json:"domainName,omitempty"`

	// APIGatewayDomainName is the AWS-managed regional domain name to CNAME to.
	// +optional
	APIGatewayDomainName string `json:"apiGatewayDomainName,omitempty"`

	// HostedZoneID is the Route53 hosted zone ID of the regional endpoint.
	// +optional
	HostedZoneID string `json:"hostedZoneId,omitempty"`

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
// +kubebuilder:printcolumn:name="Domain",type="string",JSONPath=".status.domainName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// APIGatewayV2DomainName is the Schema for managing API Gateway v2 custom domain names.
type APIGatewayV2DomainName struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   APIGatewayV2DomainNameSpec   `json:"spec,omitempty"`
	Status APIGatewayV2DomainNameStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// APIGatewayV2DomainNameList contains a list of APIGatewayV2DomainName
type APIGatewayV2DomainNameList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []APIGatewayV2DomainName `json:"items"`
}

func init() {
	SchemeBuilder.Register(&APIGatewayV2DomainName{}, &APIGatewayV2DomainNameList{})
}
