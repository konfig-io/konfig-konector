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

package ec2

import (
	"context"
	"reflect"

	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go/middleware"
)

// StripEmptyTagSpecifications is an EC2 client middleware that removes
// TagSpecification entries with no tags from any Create* input. EC2 rejects
// them with "Tag specification must have at least one tag", and dozens of
// controllers build the specification unconditionally from spec.tags.
func StripEmptyTagSpecifications(stack *middleware.Stack) error {
	return stack.Initialize.Add(middleware.InitializeMiddlewareFunc("konfigStripEmptyTagSpecs",
		func(ctx context.Context, in middleware.InitializeInput, next middleware.InitializeHandler) (middleware.InitializeOutput, middleware.Metadata, error) {
			stripEmptyTagSpecs(in.Parameters)
			return next.HandleInitialize(ctx, in)
		}), middleware.Before)
}

var tagSpecSliceType = reflect.TypeOf([]types.TagSpecification{})

func stripEmptyTagSpecs(params interface{}) {
	v := reflect.ValueOf(params)
	if v.Kind() != reflect.Ptr || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return
	}
	f := v.Elem().FieldByName("TagSpecifications")
	if !f.IsValid() || f.Type() != tagSpecSliceType || f.Len() == 0 {
		return
	}
	specs := f.Interface().([]types.TagSpecification)
	kept := make([]types.TagSpecification, 0, len(specs))
	for _, s := range specs {
		if len(s.Tags) > 0 {
			kept = append(kept, s)
		}
	}
	if len(kept) == 0 {
		f.Set(reflect.Zero(f.Type()))
		return
	}
	f.Set(reflect.ValueOf(kept))
}
