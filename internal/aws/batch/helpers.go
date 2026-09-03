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

// Package batch provides helper functions for AWS Batch operations.
package batch

import (
	"errors"
	"strings"

	"github.com/aws/smithy-go"
)

// IsNotFound returns true when the error indicates the Batch resource does
// not exist. AWS Batch has no dedicated not-found error type: it reports a
// ClientException whose message says the resource "does not exist".
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "ClientException" &&
			strings.Contains(strings.ToLower(apiErr.ErrorMessage()), "does not exist")
	}
	return false
}
