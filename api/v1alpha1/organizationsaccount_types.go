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

// OrganizationsAccountSpec defines the desired state of an AWS Organizations
// member account. Account creation is asynchronous: the controller polls
// DescribeCreateAccountStatus until the account exists, then optionally moves
// it under the requested parent.
//
// DELETION SEMANTICS: AWS accounts cannot be reliably deleted via API.
// CloseAccount exists but is heavily rate-limited (quota on closures per
// rolling 30 days) and IRREVERSIBLE after the 90-day post-closure period.
// When this CR is deleted:
//   - if spec.closeOnDelete is true, the controller calls CloseAccount;
//   - otherwise the AWS account is ABANDONED (left untouched) and a condition
//     message records that the account was not closed.
type OrganizationsAccountSpec struct {
	// Email is the email address of the account owner. Immutable; must not be
	// associated with any other AWS account.
	// +kubebuilder:validation:MinLength=6
	// +kubebuilder:validation:MaxLength=64
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="email is immutable"
	Email string `json:"email"`

	// AccountName is the friendly name of the member account. Immutable
	// (Organizations cannot rename accounts).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=50
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="accountName is immutable"
	AccountName string `json:"accountName"`

	// RoleName is the name of the IAM role created in the new account that
	// grants the management account administrator access. Defaults to
	// OrganizationAccountAccessRole when unset. Only used at creation.
	// +optional
	RoleName string `json:"roleName,omitempty"`

	// IAMUserAccessToBilling controls whether IAM users in the new account can
	// access billing information. Only used at creation.
	// +kubebuilder:validation:Enum=ALLOW;DENY
	// +optional
	IAMUserAccessToBilling string `json:"iamUserAccessToBilling,omitempty"`

	// ParentID is the root (r-...) or OU (ou-...) ID to move the account into
	// after creation via MoveAccount.
	// +optional
	ParentID string `json:"parentId,omitempty"`

	// ParentRef references an OrganizationsOU CR to move the account into
	// after creation.
	// +optional
	ParentRef *ResourceRef `json:"parentRef,omitempty"`

	// CloseOnDelete requests that the AWS account be closed (CloseAccount)
	// when this CR is deleted. Account closure is rate-limited and
	// irreversible after 90 days; the default (false) abandons the account
	// instead.
	// +optional
	CloseOnDelete bool `json:"closeOnDelete,omitempty"`

	// Tags are AWS resource tags to apply to the account.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// OrganizationsAccountStatus defines the observed state of OrganizationsAccount.
type OrganizationsAccountStatus struct {
	// AccountID is the 12-digit AWS account ID once creation succeeds.
	// +optional
	AccountID string `json:"accountId,omitempty"`

	// ARN is the Amazon Resource Name of the account.
	// +optional
	ARN string `json:"arn,omitempty"`

	// CreateAccountRequestID is the CreateAccount request ID (car-...) used to
	// poll DescribeCreateAccountStatus while creation is in progress.
	// +optional
	CreateAccountRequestID string `json:"createAccountRequestId,omitempty"`

	// State is the last observed CreateAccount state
	// (IN_PROGRESS, SUCCEEDED, or FAILED).
	// +optional
	State string `json:"state,omitempty"`

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
// +kubebuilder:printcolumn:name="Account-ID",type="string",JSONPath=".status.accountId"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OrganizationsAccount is the Schema for managing AWS Organizations member accounts.
type OrganizationsAccount struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OrganizationsAccountSpec   `json:"spec,omitempty"`
	Status OrganizationsAccountStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OrganizationsAccountList contains a list of OrganizationsAccount
type OrganizationsAccountList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OrganizationsAccount `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OrganizationsAccount{}, &OrganizationsAccountList{})
}
