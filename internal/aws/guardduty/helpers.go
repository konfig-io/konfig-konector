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

// Package guardduty provides helper functions for AWS GuardDuty operations.
package guardduty

import (
	"errors"
	"strings"

	"github.com/aws/smithy-go"
)

// IsNotFound returns true when the error indicates the detector does not
// exist. GuardDuty has no dedicated not-found exception: a request against a
// missing detector returns BadRequestException with a message identifying the
// detector as non-existent, so match on the message pattern.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		if apiErr.ErrorCode() == "BadRequestException" {
			msg := strings.ToLower(apiErr.ErrorMessage())
			return strings.Contains(msg, "detector does not exist") ||
				strings.Contains(msg, "detectorid is not owned") ||
				strings.Contains(msg, "not exist") ||
				strings.Contains(msg, "not found")
		}
	}
	return false
}
