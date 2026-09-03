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

// Package glue provides helper functions for AWS Glue operations.
package glue

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/glue/types"
	"github.com/aws/smithy-go"
)

// IsNotFound returns true when the error indicates the Glue entity does not exist.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var nf *types.EntityNotFoundException
	if errors.As(err, &nf) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "EntityNotFoundException"
	}
	return false
}

// IsAlreadyExists returns true when the error indicates the Glue entity
// already exists.
func IsAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	var ae *types.AlreadyExistsException
	if errors.As(err, &ae) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "AlreadyExistsException"
	}
	return false
}
