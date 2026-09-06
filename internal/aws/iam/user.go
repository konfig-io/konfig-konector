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

package iam

import (
	"context"

	"github.com/konfig-io/konfig-konector/internal/aws/multi"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// GetUser fetches the IAM user by name. Returns nil, nil if not found.
func GetUser(ctx context.Context, client *multi.IAM, userName string) (*types.User, error) {
	out, err := client.GetUser(ctx, &iam.GetUserInput{
		UserName: aws.String(userName),
	})
	if err != nil {
		if IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return out.User, nil
}

// CreateUser creates a new IAM user.
func CreateUser(ctx context.Context, client *multi.IAM, input *iam.CreateUserInput) (*types.User, error) {
	out, err := client.CreateUser(ctx, input)
	if err != nil {
		return nil, err
	}
	return out.User, nil
}

// UpdateUser updates the path and/or permissions boundary of a user.
func UpdateUser(ctx context.Context, client *multi.IAM, userName, newPath string) error {
	_, err := client.UpdateUser(ctx, &iam.UpdateUserInput{
		UserName:    aws.String(userName),
		NewPath:     aws.String(newPath),
		NewUserName: aws.String(userName),
	})
	return err
}

// DeleteUser deletes an IAM user. Returns nil if the user does not exist.
func DeleteUser(ctx context.Context, client *multi.IAM, userName string) error {
	_, err := client.DeleteUser(ctx, &iam.DeleteUserInput{
		UserName: aws.String(userName),
	})
	if IsNotFound(err) {
		return nil
	}
	return err
}

// SyncUserTags reconciles AWS tags on a user to match the desired set.
func SyncUserTags(ctx context.Context, client *multi.IAM, userName string, desired map[string]string) error {
	out, err := client.ListUserTags(ctx, &iam.ListUserTagsInput{UserName: aws.String(userName)})
	if err != nil {
		return err
	}

	current := make(map[string]string, len(out.Tags))
	for _, t := range out.Tags {
		current[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}

	var toAdd []types.Tag
	for k, v := range desired {
		if cur, ok := current[k]; !ok || cur != v {
			toAdd = append(toAdd, types.Tag{Key: aws.String(k), Value: aws.String(v)})
		}
	}
	if len(toAdd) > 0 {
		if _, err := client.TagUser(ctx, &iam.TagUserInput{
			UserName: aws.String(userName),
			Tags:     toAdd,
		}); err != nil {
			return err
		}
	}

	var toRemove []string
	for k := range current {
		if _, ok := desired[k]; !ok {
			toRemove = append(toRemove, k)
		}
	}
	if len(toRemove) > 0 {
		if _, err := client.UntagUser(ctx, &iam.UntagUserInput{
			UserName: aws.String(userName),
			TagKeys:  toRemove,
		}); err != nil {
			return err
		}
	}
	return nil
}
