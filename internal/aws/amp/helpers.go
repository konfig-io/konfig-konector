// Package amp provides small helpers over the Amazon Managed Service for
// Prometheus SDK.
package amp

import (
	"errors"

	"github.com/aws/smithy-go"
)

// IsNotFound reports whether err is the AMP resource-not-found error.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var ae smithy.APIError
	if errors.As(err, &ae) {
		return ae.ErrorCode() == "ResourceNotFoundException"
	}
	return false
}
