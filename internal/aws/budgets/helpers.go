// Package budgets provides small helpers over the AWS Budgets SDK.
package budgets

import (
	"errors"

	"github.com/aws/smithy-go"
)

// IsNotFound reports whether err is the Budgets not-found error.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var ae smithy.APIError
	if errors.As(err, &ae) {
		return ae.ErrorCode() == "NotFoundException"
	}
	return false
}
