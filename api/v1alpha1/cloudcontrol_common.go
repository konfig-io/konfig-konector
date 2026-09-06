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

// CloudControlStatus is embedded in the status of every generated Cloud
// Control-backed kind (hack/gen-cloudcontrol).
type CloudControlStatus struct {
	// AWSProvider confirms the account and region this resource was reconciled against.
	// +optional
	AWSProvider *ProviderStatus `json:"awsProvider,omitempty"`
	// Identifier is the Cloud Control primary identifier of the resource.
	// +optional
	Identifier string `json:"identifier,omitempty"`
	// RequestToken tracks an in-flight asynchronous operation.
	// +optional
	RequestToken string `json:"requestToken,omitempty"`
	// Operation is the in-flight operation (CREATE, UPDATE, DELETE).
	// +optional
	Operation string `json:"operation,omitempty"`
	// OperationStatus is the last reported operation status.
	// +optional
	OperationStatus string `json:"operationStatus,omitempty"`
	// Attempt counts failed create/delete requests; it salts the Cloud Control
	// idempotency token so a retry is not served the cached failure.
	// +optional
	Attempt int32 `json:"attempt,omitempty"`
	// Conditions describe the current state of the resource.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
}

// CFNTag is the CloudFormation Tag shape shared by generated kinds.
type CFNTag struct {
	// +kubebuilder:validation:MinLength=1
	Key   string `json:"key" cfn:"Key"`
	Value string `json:"value" cfn:"Value"`
}
