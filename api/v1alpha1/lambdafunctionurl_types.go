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

// LambdaURLCORSConfig defines CORS for a Lambda function URL.
type LambdaURLCORSConfig struct {
	// AllowCredentials indicates whether to include cookies in CORS responses.
	// +optional
	AllowCredentials bool `json:"allowCredentials,omitempty"`

	// AllowHeaders lists headers allowed in preflight requests.
	// +optional
	AllowHeaders []string `json:"allowHeaders,omitempty"`

	// AllowMethods lists HTTP methods allowed.
	// +optional
	AllowMethods []string `json:"allowMethods,omitempty"`

	// AllowOrigins lists origins that can access the function URL.
	// +optional
	AllowOrigins []string `json:"allowOrigins,omitempty"`

	// ExposeHeaders lists headers accessible to the browser.
	// +optional
	ExposeHeaders []string `json:"exposeHeaders,omitempty"`

	// MaxAge is the time in seconds the browser caches preflight results.
	// +optional
	MaxAge *int32 `json:"maxAge,omitempty"`
}

// LambdaFunctionURLSpec defines the desired state of a Lambda Function URL.
type LambdaFunctionURLSpec struct {
	// ProviderRef selects the AWSProvider (account/region) this resource is
	// reconciled against. Defaults to the namespace annotation, then the
	// operator's own credentials.
	// +optional
	ProviderRef *ProviderRef `json:"providerRef,omitempty"`

	// FunctionName is the name or ARN of the Lambda function.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="functionName is immutable"
	FunctionName string `json:"functionName"`

	// AuthType controls authentication for the function URL.
	// +kubebuilder:validation:Enum=NONE;AWS_IAM
	AuthType string `json:"authType"`

	// Qualifier is the function version or alias (optional).
	// +optional
	Qualifier string `json:"qualifier,omitempty"`

	// CORS defines CORS settings for the function URL.
	// +optional
	CORS *LambdaURLCORSConfig `json:"cors,omitempty"`

	// InvokeMode controls how the function is invoked.
	// +kubebuilder:validation:Enum=BUFFERED;RESPONSE_STREAM
	// +optional
	InvokeMode string `json:"invokeMode,omitempty"`
}

// LambdaFunctionURLStatus defines the observed state of LambdaFunctionURL.
type LambdaFunctionURLStatus struct {
	// FunctionURL is the URL for the function.
	// +optional
	FunctionURL string `json:"functionUrl,omitempty"`

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
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.functionUrl"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// LambdaFunctionURL is the Schema for managing Lambda function URLs.
type LambdaFunctionURL struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LambdaFunctionURLSpec   `json:"spec,omitempty"`
	Status LambdaFunctionURLStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// LambdaFunctionURLList contains a list of LambdaFunctionURL.
type LambdaFunctionURLList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LambdaFunctionURL `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LambdaFunctionURL{}, &LambdaFunctionURLList{})
}
