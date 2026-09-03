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

// Package cloudformation provides helper functions for AWS CloudFormation operations.
package cloudformation

import (
	"errors"
	"strings"

	"github.com/aws/smithy-go"
)

// IsNotFound returns true when the error indicates the stack does not exist.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	// CloudFormation returns a 400 ValidationError with "does not exist" for nonexistent stacks.
	if strings.Contains(err.Error(), "does not exist") {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "ValidationError", "ResourceNotFoundException":
			return true
		}
	}
	return false
}
