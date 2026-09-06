/*
Copyright 2024.

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

package route53

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

// RecordChangeAPI is the narrow Route53 client subset needed by
// UpsertRecordSet and DeleteRecordSet. It is satisfied by *multi.Route53.
type RecordChangeAPI interface {
	ChangeResourceRecordSets(ctx context.Context, params *route53.ChangeResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error)
}

// UpsertRecordSet creates or updates a DNS record set atomically.
// Returns the Route53 change ID so the caller can track propagation.
func UpsertRecordSet(ctx context.Context, client RecordChangeAPI, zoneID string, rrs *types.ResourceRecordSet) (string, error) {
	out, err := client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action:            types.ChangeActionUpsert,
					ResourceRecordSet: rrs,
				},
			},
		},
	})
	if err != nil {
		return "", err
	}
	return aws.ToString(out.ChangeInfo.Id), nil
}

// DeleteRecordSet removes a DNS record set. Tolerates not-found.
func DeleteRecordSet(ctx context.Context, client RecordChangeAPI, zoneID string, rrs *types.ResourceRecordSet) error {
	_, err := client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action:            types.ChangeActionDelete,
					ResourceRecordSet: rrs,
				},
			},
		},
	})
	if IsNotFound(err) {
		return nil
	}
	return err
}

// ChangeStatusAPI is the narrow Route53 client subset needed by
// GetChangeStatus. It is satisfied by *multi.Route53.
type ChangeStatusAPI interface {
	GetChange(ctx context.Context, params *route53.GetChangeInput, optFns ...func(*route53.Options)) (*route53.GetChangeOutput, error)
}

// GetChangeStatus checks whether a Route53 change has propagated (INSYNC / PENDING).
func GetChangeStatus(ctx context.Context, client ChangeStatusAPI, changeID string) (string, error) {
	out, err := client.GetChange(ctx, &route53.GetChangeInput{
		Id: aws.String(changeID),
	})
	if err != nil {
		return "", err
	}
	return string(out.ChangeInfo.Status), nil
}

// FindRecordSet searches for an existing record matching name/type/setIdentifier.
// Returns nil if not found.
func FindRecordSet(
	ctx context.Context,
	client route53.ListResourceRecordSetsAPIClient,
	zoneID, name, rrType, setIdentifier string,
) (*types.ResourceRecordSet, error) {
	paginator := route53.NewListResourceRecordSetsPaginator(client, &route53.ListResourceRecordSetsInput{
		HostedZoneId:    aws.String(zoneID),
		StartRecordName: aws.String(name),
		StartRecordType: types.RRType(rrType),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, rrs := range page.ResourceRecordSets {
			if aws.ToString(rrs.Name) != name {
				break
			}
			if string(rrs.Type) != rrType {
				continue
			}
			if setIdentifier != "" && aws.ToString(rrs.SetIdentifier) != setIdentifier {
				continue
			}
			return &rrs, nil
		}
	}
	return nil, nil
}
