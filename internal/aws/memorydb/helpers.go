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

// Package memorydb provides helper functions for AWS MemoryDB operations.
package memorydb

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/memorydb/types"
	"github.com/aws/smithy-go"
)

// IsNotFound returns true when the error indicates the resource does not exist.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var nf1 *types.ClusterNotFoundFault
	var nf2 *types.SnapshotNotFoundFault
	if errors.As(err, &nf1) || errors.As(err, &nf2) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "ClusterNotFoundFault", "SnapshotNotFoundFault":
			return true
		}
	}
	return false
}
