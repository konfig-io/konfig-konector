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

// Package athena provides helper functions for AWS Athena operations.
package athena

import (
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/athena/types"
	"github.com/aws/smithy-go"
)

// IsNotFound returns true when the error indicates the Athena resource does
// not exist. Athena has no dedicated not-found error for workgroups and named
// queries; it returns InvalidRequestException with a "not found"/"does not
// exist" message, so the message is inspected as well as the typed
// ResourceNotFoundException used by newer APIs (data catalogs, capacity
// reservations).
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var nf *types.ResourceNotFoundException
	if errors.As(err, &nf) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "ResourceNotFoundException":
			return true
		case "InvalidRequestException":
			msg := strings.ToLower(apiErr.ErrorMessage())
			return strings.Contains(msg, "not found") ||
				strings.Contains(msg, "does not exist") ||
				strings.Contains(msg, "was not found") ||
				strings.Contains(msg, "notfound")
		}
	}
	return false
}
