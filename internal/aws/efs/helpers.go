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

// Package efs provides helper functions for AWS EFS operations.
package efs

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/aws/smithy-go"
)

// IsNotFound returns true when the error indicates the resource does not exist.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var nf1 *types.FileSystemNotFound
	var nf2 *types.MountTargetNotFound
	var nf3 *types.AccessPointNotFound
	if errors.As(err, &nf1) || errors.As(err, &nf2) || errors.As(err, &nf3) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "FileSystemNotFound", "MountTargetNotFound", "AccessPointNotFound":
			return true
		}
	}
	return false
}
