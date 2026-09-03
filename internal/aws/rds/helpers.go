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

// Package rds provides helper functions for AWS RDS operations.
package rds

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/smithy-go"
)

// IsNotFound returns true when the error indicates the resource does not exist.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var nf1 *types.DBInstanceNotFoundFault
	var nf2 *types.DBClusterNotFoundFault
	var nf3 *types.DBSubnetGroupNotFoundFault
	var nf4 *types.DBParameterGroupNotFoundFault
	var nf5 *types.DBClusterParameterGroupNotFoundFault
	var nf6 *types.DBSnapshotNotFoundFault
	var nf7 *types.DBProxyNotFoundFault
	var nf8 *types.OptionGroupNotFoundFault
	var nf9 *types.GlobalClusterNotFoundFault
	var nf10 *types.SubscriptionNotFoundFault
	if errors.As(err, &nf1) || errors.As(err, &nf2) || errors.As(err, &nf3) ||
		errors.As(err, &nf4) || errors.As(err, &nf5) || errors.As(err, &nf6) ||
		errors.As(err, &nf7) || errors.As(err, &nf8) || errors.As(err, &nf9) ||
		errors.As(err, &nf10) {
		return true
	}
	// Fallback for any untyped not-found from smithy
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		switch code {
		case "DBInstanceNotFound", "DBClusterNotFound",
			"DBSubnetGroupNotFound", "DBParameterGroupNotFound",
			"DBClusterParameterGroupNotFound", "DBSnapshotNotFound",
			"DBProxyNotFoundFault", "OptionGroupNotFoundFault",
			"GlobalClusterNotFoundFault", "SubscriptionNotFound":
			return true
		}
	}
	return false
}

// IsTransient returns true for states where AWS is busy and we should just requeue.
func IsTransient(status string) bool {
	switch status {
	case "creating", "modifying", "backing-up", "rebooting",
		"starting", "stopping", "upgrading", "migrating",
		"maintenance", "renaming":
		return true
	}
	return false
}
