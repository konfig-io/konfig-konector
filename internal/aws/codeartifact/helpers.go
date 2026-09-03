// Package codeartifact provides small helpers over the CodeArtifact SDK.
package codeartifact

import (
	"errors"

	"github.com/aws/smithy-go"
)

// IsNotFound reports whether err is the CodeArtifact resource-not-found error.
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
