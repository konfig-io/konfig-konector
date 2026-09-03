// Package costexplorer provides small helpers over the Cost Explorer SDK.
package costexplorer

import (
	"errors"

	"github.com/aws/smithy-go"
)

// IsNotFound reports whether err indicates a missing Cost Explorer anomaly
// monitor or subscription.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var ae smithy.APIError
	if errors.As(err, &ae) {
		switch ae.ErrorCode() {
		case "UnknownMonitorException", "UnknownSubscriptionException", "ResourceNotFoundException":
			return true
		}
	}
	return false
}
