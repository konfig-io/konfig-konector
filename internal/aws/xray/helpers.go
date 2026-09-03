// Package xray provides small helpers over the X-Ray SDK.
package xray

import (
	"errors"

	"github.com/aws/smithy-go"
)

// IsNotFound reports whether err indicates a missing X-Ray resource.
// The X-Ray API has no dedicated not-found error for groups or sampling
// rules: deleting or updating a missing resource surfaces as
// ResourceNotFoundException in newer API paths and InvalidRequestException
// otherwise, so both are treated as not-found for delete idempotency.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var ae smithy.APIError
	if errors.As(err, &ae) {
		switch ae.ErrorCode() {
		case "ResourceNotFoundException", "InvalidRequestException":
			return true
		}
	}
	return false
}
