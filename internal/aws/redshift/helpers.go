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

// Package redshift provides helper functions for AWS Redshift operations.
package redshift

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/redshift/types"
	"github.com/aws/smithy-go"
)

// IsNotFound returns true when the error indicates the Redshift resource does
// not exist.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var nf1 *types.ClusterNotFoundFault
	var nf2 *types.ClusterSubnetGroupNotFoundFault
	var nf3 *types.ClusterParameterGroupNotFoundFault
	if errors.As(err, &nf1) || errors.As(err, &nf2) || errors.As(err, &nf3) {
		return true
	}
	// Fallback for untyped not-found errors from smithy.
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "ClusterNotFound", "ClusterNotFoundFault",
			"ClusterSubnetGroupNotFound", "ClusterSubnetGroupNotFoundFault",
			"ClusterParameterGroupNotFound", "ClusterParameterGroupNotFoundFault":
			return true
		}
	}
	return false
}

// IsTransientClusterStatus returns true for cluster states where AWS is busy
// and the controller should requeue instead of mutating.
func IsTransientClusterStatus(status string) bool {
	switch status {
	case "creating", "modifying", "rebooting", "renaming", "resizing",
		"rotating-keys", "storage-full", "updating-hsm", "recovering",
		"restoring", "resuming", "pausing", "maintenance":
		return true
	}
	return false
}
