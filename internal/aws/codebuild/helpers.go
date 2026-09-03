package codebuild

import (
	"errors"

	"github.com/aws/smithy-go"
)

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
