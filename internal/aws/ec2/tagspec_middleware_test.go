package ec2

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestStripEmptyTagSpecs(t *testing.T) {
	in := &awsec2.CreateSubnetInput{TagSpecifications: []types.TagSpecification{{ResourceType: types.ResourceTypeSubnet}}}
	stripEmptyTagSpecs(in)
	if in.TagSpecifications != nil {
		t.Fatalf("expected empty tag specs removed, got %v", in.TagSpecifications)
	}
	in2 := &awsec2.CreateSubnetInput{TagSpecifications: []types.TagSpecification{
		{ResourceType: types.ResourceTypeSubnet},
		{ResourceType: types.ResourceTypeSubnet, Tags: []types.Tag{{Key: aws.String("Name"), Value: aws.String("x")}}},
	}}
	stripEmptyTagSpecs(in2)
	if len(in2.TagSpecifications) != 1 || len(in2.TagSpecifications[0].Tags) != 1 {
		t.Fatalf("expected one non-empty spec kept, got %v", in2.TagSpecifications)
	}
	stripEmptyTagSpecs(&awsec2.DescribeVpcsInput{}) // no field: no-op, no panic
}
