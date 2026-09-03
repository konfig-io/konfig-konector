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

// Package securityhub provides helper functions for AWS Security Hub operations.
package securityhub

import (
	"errors"

	"github.com/aws/smithy-go"
)

// IsNotFound returns true when the error indicates the hub resource does not
// exist. Security Hub returns ResourceNotFoundException for missing
// resources, and InvalidAccessException when the account is not subscribed
// to Security Hub at all — both mean "not enabled" for reconcile purposes.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "ResourceNotFoundException", "InvalidAccessException":
			return true
		}
	}
	return false
}
