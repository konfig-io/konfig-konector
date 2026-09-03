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

// Package organizations provides helper functions for AWS Organizations operations.
package organizations

import (
	"errors"

	"github.com/aws/smithy-go"
)

// IsNotFound returns true when the error indicates the Organizations entity
// does not exist. Organizations uses a per-entity *NotFoundException family
// instead of a single code.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "AccountNotFoundException",
			"OrganizationalUnitNotFoundException",
			"PolicyNotFoundException",
			"PolicyNotAttachedException",
			"ParentNotFoundException",
			"ChildNotFoundException",
			"TargetNotFoundException",
			"RootNotFoundException",
			"CreateAccountStatusNotFoundException":
			return true
		}
	}
	return false
}

// IsDuplicateAttachment returns true when the policy is already attached to
// the target — a benign condition for the attachment reconciler.
func IsDuplicateAttachment(err error) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "DuplicatePolicyAttachmentException"
	}
	return false
}

// IsAccountAlreadyClosed returns true when CloseAccount reports the account
// is already closed or closing.
func IsAccountAlreadyClosed(err error) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "AccountAlreadyClosedException"
	}
	return false
}
