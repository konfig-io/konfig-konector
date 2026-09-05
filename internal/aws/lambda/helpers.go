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

// Package lambda provides helper functions for AWS Lambda operations.
package lambda

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/konfig-io/konfig-konector/internal/aws/multi"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/smithy-go"
)

// IsNotFound returns true when the error indicates the resource does not exist.
func IsNotFound(err error) bool {
	var nfe *types.ResourceNotFoundException
	if errors.As(err, &nfe) {
		return true
	}
	var pcnfe *types.ProvisionedConcurrencyConfigNotFoundException
	if errors.As(err, &pcnfe) {
		return true
	}
	var ae smithy.APIError
	if errors.As(err, &ae) {
		switch ae.ErrorCode() {
		case "ResourceNotFoundException", "ProvisionedConcurrencyConfigNotFoundException":
			return true
		}
	}
	return false
}

// IsConflict returns true when a resource already exists (AddPermission duplicate).
func IsConflict(err error) bool {
	var rce *types.ResourceConflictException
	return errors.As(err, &rce)
}

// FunctionAPI is the narrow subset of the Lambda SDK client used by the
// function lifecycle helpers. *multi.Lambda satisfies it, so existing
// callers keep compiling.
type FunctionAPI interface {
	GetFunction(ctx context.Context, params *lambda.GetFunctionInput, optFns ...func(*lambda.Options)) (*lambda.GetFunctionOutput, error)
	CreateFunction(ctx context.Context, params *lambda.CreateFunctionInput, optFns ...func(*lambda.Options)) (*lambda.CreateFunctionOutput, error)
	UpdateFunctionConfiguration(ctx context.Context, params *lambda.UpdateFunctionConfigurationInput, optFns ...func(*lambda.Options)) (*lambda.UpdateFunctionConfigurationOutput, error)
	UpdateFunctionCode(ctx context.Context, params *lambda.UpdateFunctionCodeInput, optFns ...func(*lambda.Options)) (*lambda.UpdateFunctionCodeOutput, error)
	PutFunctionConcurrency(ctx context.Context, params *lambda.PutFunctionConcurrencyInput, optFns ...func(*lambda.Options)) (*lambda.PutFunctionConcurrencyOutput, error)
	DeleteFunctionConcurrency(ctx context.Context, params *lambda.DeleteFunctionConcurrencyInput, optFns ...func(*lambda.Options)) (*lambda.DeleteFunctionConcurrencyOutput, error)
	DeleteFunction(ctx context.Context, params *lambda.DeleteFunctionInput, optFns ...func(*lambda.Options)) (*lambda.DeleteFunctionOutput, error)
}

func GetFunction(ctx context.Context, c FunctionAPI, name string) (*types.FunctionConfiguration, error) {
	out, err := c.GetFunction(ctx, &lambda.GetFunctionInput{FunctionName: aws.String(name)})
	if err != nil {
		return nil, err
	}
	return out.Configuration, nil
}

// LambdaFileSystemConfig is an EFS access point mount for a Lambda function.
type LambdaFileSystemConfig struct {
	ARN            string
	LocalMountPath string
}

type CreateFunctionInput struct {
	FunctionName       string
	RoleArn            string
	Runtime            types.Runtime
	Handler            string
	S3Bucket           string
	S3Key              string
	S3ObjectVersion    string
	ImageURI           string
	Description        string
	Timeout            *int32
	MemorySize         *int32
	Environment        map[string]string
	SubnetIDs          []string
	SecurityGroupIDs   []string
	Architecture       types.Architecture
	EphemeralStorageMB *int32
	Tags               map[string]string

	Layers              []string
	DeadLetterTargetARN string
	TracingMode         string
	LogFormat           string
	LogGroup            string
	SystemLogLevel      string
	ApplicationLogLevel string
	FileSystemConfigs   []LambdaFileSystemConfig
	SnapStartApplyOn    string
	ImageCommand        []string
	ImageEntryPoint     []string
	ImageWorkingDir     string
}

func CreateFunction(ctx context.Context, c FunctionAPI, in CreateFunctionInput) (*lambda.CreateFunctionOutput, error) {
	input := &lambda.CreateFunctionInput{
		FunctionName: aws.String(in.FunctionName),
		Role:         aws.String(in.RoleArn),
		Tags:         in.Tags,
	}
	if in.ImageURI != "" {
		input.Code = &types.FunctionCode{ImageUri: aws.String(in.ImageURI)}
		input.PackageType = types.PackageTypeImage
	} else {
		input.Code = &types.FunctionCode{
			S3Bucket: aws.String(in.S3Bucket),
			S3Key:    aws.String(in.S3Key),
		}
		if in.S3ObjectVersion != "" {
			input.Code.S3ObjectVersion = aws.String(in.S3ObjectVersion)
		}
		input.Runtime = in.Runtime
		if in.Handler != "" {
			input.Handler = aws.String(in.Handler)
		}
		input.PackageType = types.PackageTypeZip
	}
	if in.Description != "" {
		input.Description = aws.String(in.Description)
	}
	if in.Timeout != nil {
		input.Timeout = in.Timeout
	}
	if in.MemorySize != nil {
		input.MemorySize = in.MemorySize
	}
	if len(in.Environment) > 0 {
		input.Environment = &types.Environment{Variables: in.Environment}
	}
	if len(in.SubnetIDs) > 0 {
		input.VpcConfig = &types.VpcConfig{
			SubnetIds:        in.SubnetIDs,
			SecurityGroupIds: in.SecurityGroupIDs,
		}
	}
	if in.Architecture != "" {
		input.Architectures = []types.Architecture{in.Architecture}
	}
	if in.EphemeralStorageMB != nil {
		input.EphemeralStorage = &types.EphemeralStorage{Size: in.EphemeralStorageMB}
	}
	if len(in.Layers) > 0 {
		input.Layers = in.Layers
	}
	if in.DeadLetterTargetARN != "" {
		input.DeadLetterConfig = &types.DeadLetterConfig{TargetArn: aws.String(in.DeadLetterTargetARN)}
	}
	if in.TracingMode != "" {
		input.TracingConfig = &types.TracingConfig{Mode: types.TracingMode(in.TracingMode)}
	}
	if in.LogFormat != "" || in.LogGroup != "" || in.SystemLogLevel != "" || in.ApplicationLogLevel != "" {
		lc := &types.LoggingConfig{}
		if in.LogFormat != "" {
			lc.LogFormat = types.LogFormat(in.LogFormat)
		}
		if in.LogGroup != "" {
			lc.LogGroup = aws.String(in.LogGroup)
		}
		if in.SystemLogLevel != "" {
			lc.SystemLogLevel = types.SystemLogLevel(in.SystemLogLevel)
		}
		if in.ApplicationLogLevel != "" {
			lc.ApplicationLogLevel = types.ApplicationLogLevel(in.ApplicationLogLevel)
		}
		input.LoggingConfig = lc
	}
	if len(in.FileSystemConfigs) > 0 {
		fscs := make([]types.FileSystemConfig, 0, len(in.FileSystemConfigs))
		for _, fsc := range in.FileSystemConfigs {
			fscs = append(fscs, types.FileSystemConfig{
				Arn:            aws.String(fsc.ARN),
				LocalMountPath: aws.String(fsc.LocalMountPath),
			})
		}
		input.FileSystemConfigs = fscs
	}
	if in.SnapStartApplyOn != "" {
		input.SnapStart = &types.SnapStart{ApplyOn: types.SnapStartApplyOn(in.SnapStartApplyOn)}
	}
	if in.ImageURI != "" && (len(in.ImageCommand) > 0 || len(in.ImageEntryPoint) > 0 || in.ImageWorkingDir != "") {
		ic := &types.ImageConfig{}
		if len(in.ImageCommand) > 0 {
			ic.Command = in.ImageCommand
		}
		if len(in.ImageEntryPoint) > 0 {
			ic.EntryPoint = in.ImageEntryPoint
		}
		if in.ImageWorkingDir != "" {
			ic.WorkingDirectory = aws.String(in.ImageWorkingDir)
		}
		input.ImageConfig = ic
	}
	out, err := c.CreateFunction(ctx, input)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func UpdateFunctionConfiguration(ctx context.Context, c FunctionAPI, in CreateFunctionInput) error {
	input := &lambda.UpdateFunctionConfigurationInput{
		FunctionName: aws.String(in.FunctionName),
		Role:         aws.String(in.RoleArn),
	}
	if in.Runtime != "" {
		input.Runtime = in.Runtime
	}
	if in.Handler != "" {
		input.Handler = aws.String(in.Handler)
	}
	if in.Description != "" {
		input.Description = aws.String(in.Description)
	}
	if in.Timeout != nil {
		input.Timeout = in.Timeout
	}
	if in.MemorySize != nil {
		input.MemorySize = in.MemorySize
	}
	if len(in.Environment) > 0 {
		input.Environment = &types.Environment{Variables: in.Environment}
	}
	if len(in.SubnetIDs) > 0 {
		input.VpcConfig = &types.VpcConfig{
			SubnetIds:        in.SubnetIDs,
			SecurityGroupIds: in.SecurityGroupIDs,
		}
	}
	if in.EphemeralStorageMB != nil {
		input.EphemeralStorage = &types.EphemeralStorage{Size: in.EphemeralStorageMB}
	}
	if len(in.Layers) > 0 {
		input.Layers = in.Layers
	}
	if in.DeadLetterTargetARN != "" {
		input.DeadLetterConfig = &types.DeadLetterConfig{TargetArn: aws.String(in.DeadLetterTargetARN)}
	}
	if in.TracingMode != "" {
		input.TracingConfig = &types.TracingConfig{Mode: types.TracingMode(in.TracingMode)}
	}
	if in.LogFormat != "" || in.LogGroup != "" || in.SystemLogLevel != "" || in.ApplicationLogLevel != "" {
		lc := &types.LoggingConfig{}
		if in.LogFormat != "" {
			lc.LogFormat = types.LogFormat(in.LogFormat)
		}
		if in.LogGroup != "" {
			lc.LogGroup = aws.String(in.LogGroup)
		}
		if in.SystemLogLevel != "" {
			lc.SystemLogLevel = types.SystemLogLevel(in.SystemLogLevel)
		}
		if in.ApplicationLogLevel != "" {
			lc.ApplicationLogLevel = types.ApplicationLogLevel(in.ApplicationLogLevel)
		}
		input.LoggingConfig = lc
	}
	if len(in.FileSystemConfigs) > 0 {
		fscs := make([]types.FileSystemConfig, 0, len(in.FileSystemConfigs))
		for _, fsc := range in.FileSystemConfigs {
			fscs = append(fscs, types.FileSystemConfig{
				Arn:            aws.String(fsc.ARN),
				LocalMountPath: aws.String(fsc.LocalMountPath),
			})
		}
		input.FileSystemConfigs = fscs
	}
	if in.SnapStartApplyOn != "" {
		input.SnapStart = &types.SnapStart{ApplyOn: types.SnapStartApplyOn(in.SnapStartApplyOn)}
	}
	if in.ImageURI != "" && (len(in.ImageCommand) > 0 || len(in.ImageEntryPoint) > 0 || in.ImageWorkingDir != "") {
		ic := &types.ImageConfig{}
		if len(in.ImageCommand) > 0 {
			ic.Command = in.ImageCommand
		}
		if len(in.ImageEntryPoint) > 0 {
			ic.EntryPoint = in.ImageEntryPoint
		}
		if in.ImageWorkingDir != "" {
			ic.WorkingDirectory = aws.String(in.ImageWorkingDir)
		}
		input.ImageConfig = ic
	}
	_, err := c.UpdateFunctionConfiguration(ctx, input)
	return err
}

func SetReservedConcurrency(ctx context.Context, c FunctionAPI, functionName string, concurrency int32) error {
	_, err := c.PutFunctionConcurrency(ctx, &lambda.PutFunctionConcurrencyInput{
		FunctionName:                 aws.String(functionName),
		ReservedConcurrentExecutions: aws.Int32(concurrency),
	})
	return err
}

func DeleteReservedConcurrency(ctx context.Context, c FunctionAPI, functionName string) error {
	_, err := c.DeleteFunctionConcurrency(ctx, &lambda.DeleteFunctionConcurrencyInput{
		FunctionName: aws.String(functionName),
	})
	if IsNotFound(err) {
		return nil
	}
	return err
}

func UpdateFunctionCode(ctx context.Context, c FunctionAPI, in CreateFunctionInput) error {
	input := &lambda.UpdateFunctionCodeInput{
		FunctionName: aws.String(in.FunctionName),
	}
	if in.ImageURI != "" {
		input.ImageUri = aws.String(in.ImageURI)
	} else {
		input.S3Bucket = aws.String(in.S3Bucket)
		input.S3Key = aws.String(in.S3Key)
		if in.S3ObjectVersion != "" {
			input.S3ObjectVersion = aws.String(in.S3ObjectVersion)
		}
	}
	_, err := c.UpdateFunctionCode(ctx, input)
	return err
}

func DeleteFunction(ctx context.Context, c FunctionAPI, name string) error {
	_, err := c.DeleteFunction(ctx, &lambda.DeleteFunctionInput{FunctionName: aws.String(name)})
	return err
}

// GetEventSourceMapping fetches an event source mapping by UUID.
func GetEventSourceMapping(ctx context.Context, c *multi.Lambda, uuid string) (*lambda.GetEventSourceMappingOutput, error) {
	out, err := c.GetEventSourceMapping(ctx, &lambda.GetEventSourceMappingInput{UUID: aws.String(uuid)})
	if err != nil {
		return nil, err
	}
	return out, nil
}

type EventSourceMappingInput struct {
	FunctionArn                    string
	EventSourceArn                 string
	BatchSize                      *int32
	Enabled                        *bool
	StartingPosition               types.EventSourcePosition
	MaximumBatchingWindowInSeconds *int32
	FilterPatterns                 []string
}

func CreateEventSourceMapping(ctx context.Context, c *multi.Lambda, in EventSourceMappingInput) (*lambda.CreateEventSourceMappingOutput, error) {
	input := &lambda.CreateEventSourceMappingInput{
		FunctionName:   aws.String(in.FunctionArn),
		EventSourceArn: aws.String(in.EventSourceArn),
	}
	if in.BatchSize != nil {
		input.BatchSize = in.BatchSize
	}
	if in.Enabled != nil {
		input.Enabled = in.Enabled
	}
	if in.StartingPosition != "" {
		input.StartingPosition = in.StartingPosition
	}
	if in.MaximumBatchingWindowInSeconds != nil {
		input.MaximumBatchingWindowInSeconds = in.MaximumBatchingWindowInSeconds
	}
	if len(in.FilterPatterns) > 0 {
		filters := make([]types.Filter, 0, len(in.FilterPatterns))
		for _, p := range in.FilterPatterns {
			pattern := p
			filters = append(filters, types.Filter{Pattern: &pattern})
		}
		input.FilterCriteria = &types.FilterCriteria{Filters: filters}
	}
	out, err := c.CreateEventSourceMapping(ctx, input)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func UpdateEventSourceMapping(ctx context.Context, c *multi.Lambda, uuid string, in EventSourceMappingInput) error {
	input := &lambda.UpdateEventSourceMappingInput{
		UUID:         aws.String(uuid),
		FunctionName: aws.String(in.FunctionArn),
	}
	if in.BatchSize != nil {
		input.BatchSize = in.BatchSize
	}
	if in.Enabled != nil {
		input.Enabled = in.Enabled
	}
	if in.MaximumBatchingWindowInSeconds != nil {
		input.MaximumBatchingWindowInSeconds = in.MaximumBatchingWindowInSeconds
	}
	_, err := c.UpdateEventSourceMapping(ctx, input)
	return err
}

func DeleteEventSourceMapping(ctx context.Context, c *multi.Lambda, uuid string) error {
	_, err := c.DeleteEventSourceMapping(ctx, &lambda.DeleteEventSourceMappingInput{UUID: aws.String(uuid)})
	return err
}

// PolicyStatement is used to parse the Lambda resource policy JSON.
type PolicyStatement struct {
	Sid string `json:"Sid"`
}

type policyDoc struct {
	Statement []PolicyStatement `json:"Statement"`
}

// StatementExists checks whether a given Sid is in the function's resource policy.
func StatementExists(ctx context.Context, c *multi.Lambda, functionName, statementId string) (bool, error) {
	out, err := c.GetPolicy(ctx, &lambda.GetPolicyInput{FunctionName: aws.String(functionName)})
	if IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if out.Policy == nil {
		return false, nil
	}
	var doc policyDoc
	if err := json.Unmarshal([]byte(*out.Policy), &doc); err != nil {
		return false, fmt.Errorf("parse policy: %w", err)
	}
	for _, s := range doc.Statement {
		if s.Sid == statementId {
			return true, nil
		}
	}
	return false, nil
}

type AddPermissionInput struct {
	FunctionName  string
	StatementId   string
	Action        string
	Principal     string
	SourceArn     string
	SourceAccount string
}

func AddPermission(ctx context.Context, c *multi.Lambda, in AddPermissionInput) error {
	input := &lambda.AddPermissionInput{
		FunctionName: aws.String(in.FunctionName),
		StatementId:  aws.String(in.StatementId),
		Action:       aws.String(in.Action),
		Principal:    aws.String(in.Principal),
	}
	if in.SourceArn != "" {
		input.SourceArn = aws.String(in.SourceArn)
	}
	if in.SourceAccount != "" {
		input.SourceAccount = aws.String(in.SourceAccount)
	}
	_, err := c.AddPermission(ctx, input)
	if IsConflict(err) {
		return nil // already exists, idempotent
	}
	return err
}

func RemovePermission(ctx context.Context, c *multi.Lambda, functionName, statementId string) error {
	_, err := c.RemovePermission(ctx, &lambda.RemovePermissionInput{
		FunctionName: aws.String(functionName),
		StatementId:  aws.String(statementId),
	})
	if IsNotFound(err) {
		return nil
	}
	return err
}
