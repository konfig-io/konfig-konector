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

// Package ec2 provides helper functions for AWS EC2 operations.
package ec2

import (
	"context"
	"errors"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

// TagsAPI is the narrow EC2 client surface needed for tag synchronization.
// *ec2.Client satisfies it, so callers passing a real client are unaffected.
type TagsAPI interface {
	CreateTags(ctx context.Context, params *awsec2.CreateTagsInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateTagsOutput, error)
	DeleteTags(ctx context.Context, params *awsec2.DeleteTagsInput, optFns ...func(*awsec2.Options)) (*awsec2.DeleteTagsOutput, error)
	DescribeTags(ctx context.Context, params *awsec2.DescribeTagsInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeTagsOutput, error)
}

// DescribeVpcsAPI is the narrow EC2 client surface needed to look up VPCs.
// *ec2.Client satisfies it.
type DescribeVpcsAPI interface {
	DescribeVpcs(ctx context.Context, params *awsec2.DescribeVpcsInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeVpcsOutput, error)
}

// IsNotFound returns true when the AWS error indicates the resource does not exist.
// EC2 returns InvalidXxx.NotFound error codes for missing resources.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		switch code {
		case "InvalidVpcID.NotFound",
			"InvalidSubnetID.NotFound",
			"InvalidInternetGatewayID.NotFound",
			"InvalidRouteTableID.NotFound",
			"InvalidNatGatewayID.NotFound",
			"InvalidGroup.NotFound",
			"InvalidGroupId.NotFound",
			"InvalidVpcEndpointId.NotFound",
			"InvalidKeyPair.NotFound",
			"InvalidLaunchTemplateId.NotFound",
			"InvalidLaunchTemplateName.NotFoundException",
			"InvalidAllocationID.NotFound",
			"InvalidAssociationID.NotFound",
			"InvalidNetworkAclID.NotFound",
			"InvalidPlacementGroup.Unknown",
			"InvalidTransitGatewayID.NotFound",
			"InvalidTransitGatewayAttachmentID.NotFound",
			"InvalidVpcPeeringConnectionID.NotFound",
			"InvalidEgressOnlyInternetGatewayId.NotFound",
			"InvalidFlowLogId.NotFound",
			"InvalidVolumeID.NotFound",
			"InvalidAMIID.NotFound",
			"InvalidSpotFleetRequestId.NotFound",
			"InvalidCustomerGatewayID.NotFound",
			"InvalidVpnGatewayID.NotFound",
			"InvalidVpnConnectionID.NotFound",
			"InvalidRoute.NotFound",
			"InvalidPrefixListID.NotFound",
			"InvalidPrefixListId.NotFound",
			"InvalidCapacityReservationId.NotFound",
			"InvalidCapacityReservationId.NotFoundException":
			return true
		}
	}
	return false
}

// TagsFromMap converts a map of string key/value pairs to EC2 Tag slice.
func TagsFromMap(m map[string]string) []types.Tag {
	tags := make([]types.Tag, 0, len(m))
	for k, v := range m {
		k, v := k, v
		tags = append(tags, types.Tag{Key: &k, Value: &v})
	}
	return tags
}
