package codedeploy

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
		code := ae.ErrorCode()
		return code == "ApplicationDoesNotExistException" ||
			code == "DeploymentGroupDoesNotExistException"
	}
	return false
}
