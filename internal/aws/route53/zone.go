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

// Package route53 provides helper functions for Route53 operations.
package route53

import (
	"context"
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

// HostedZoneGetAPI is the narrow Route53 client subset needed by GetHostedZone.
// It is satisfied by *route53.Client.
type HostedZoneGetAPI interface {
	GetHostedZone(ctx context.Context, params *route53.GetHostedZoneInput, optFns ...func(*route53.Options)) (*route53.GetHostedZoneOutput, error)
}

// HostedZoneDeleteAPI is the narrow Route53 client subset needed by
// DeleteHostedZone. It is satisfied by *route53.Client.
type HostedZoneDeleteAPI interface {
	route53.ListResourceRecordSetsAPIClient
	ChangeResourceRecordSets(ctx context.Context, params *route53.ChangeResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error)
	DeleteHostedZone(ctx context.Context, params *route53.DeleteHostedZoneInput, optFns ...func(*route53.Options)) (*route53.DeleteHostedZoneOutput, error)
}

// TagSyncAPI is the narrow Route53 client subset needed by SyncTags.
// It is satisfied by *route53.Client.
type TagSyncAPI interface {
	ListTagsForResource(ctx context.Context, params *route53.ListTagsForResourceInput, optFns ...func(*route53.Options)) (*route53.ListTagsForResourceOutput, error)
	ChangeTagsForResource(ctx context.Context, params *route53.ChangeTagsForResourceInput, optFns ...func(*route53.Options)) (*route53.ChangeTagsForResourceOutput, error)
}

// IsNotFound returns true when the AWS error indicates the resource does not exist.
func IsNotFound(err error) bool {
	var nhe *types.NoSuchHostedZone
	var nhc *types.NoSuchHealthCheck
	return errors.As(err, &nhe) || errors.As(err, &nhc)
}

// StripZonePrefix removes the "/hostedzone/" prefix that Route53 returns.
func StripZonePrefix(id string) string {
	return strings.TrimPrefix(id, "/hostedzone/")
}

// GetHostedZone fetches a hosted zone by its bare ID. Returns nil, nil if not found.
func GetHostedZone(ctx context.Context, client HostedZoneGetAPI, zoneID string) (*types.HostedZone, error) {
	out, err := client.GetHostedZone(ctx, &route53.GetHostedZoneInput{
		Id: aws.String(zoneID),
	})
	if err != nil {
		if IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return out.HostedZone, nil
}

// DeleteHostedZone purges all non-SOA/NS records from the zone, then deletes it.
// Route53 requires the zone to be empty (only SOA + NS) before deletion.
func DeleteHostedZone(ctx context.Context, client HostedZoneDeleteAPI, zoneID string) error {
	// List all records and delete any that aren't SOA or NS at the zone apex.
	paginator := route53.NewListResourceRecordSetsPaginator(client, &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
	})
	var toDelete []types.ResourceRecordSet
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			if IsNotFound(err) {
				return nil
			}
			return err
		}
		for _, rrs := range page.ResourceRecordSets {
			if rrs.Type == types.RRTypeNs || rrs.Type == types.RRTypeSoa {
				continue
			}
			toDelete = append(toDelete, rrs)
		}
	}

	if len(toDelete) > 0 {
		var changes []types.Change
		for _, rrs := range toDelete {
			r := rrs
			changes = append(changes, types.Change{
				Action:            types.ChangeActionDelete,
				ResourceRecordSet: &r,
			})
		}
		_, err := client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
			HostedZoneId: aws.String(zoneID),
			ChangeBatch:  &types.ChangeBatch{Changes: changes},
		})
		if err != nil && !IsNotFound(err) {
			return err
		}
	}

	_, err := client.DeleteHostedZone(ctx, &route53.DeleteHostedZoneInput{
		Id: aws.String(zoneID),
	})
	if IsNotFound(err) {
		return nil
	}
	return err
}

// SyncTags synchronises Route53 tags on a hosted zone.
func SyncTags(ctx context.Context, client TagSyncAPI, zoneID string, desired map[string]string) error {
	out, err := client.ListTagsForResource(ctx, &route53.ListTagsForResourceInput{
		ResourceType: types.TagResourceTypeHostedzone,
		ResourceId:   aws.String(zoneID),
	})
	if err != nil {
		return err
	}

	current := make(map[string]string)
	for _, t := range out.ResourceTagSet.Tags {
		current[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}

	var toAdd []types.Tag
	for k, v := range desired {
		if cur, ok := current[k]; !ok || cur != v {
			toAdd = append(toAdd, types.Tag{Key: aws.String(k), Value: aws.String(v)})
		}
	}

	var toRemove []string
	for k := range current {
		if _, ok := desired[k]; !ok {
			toRemove = append(toRemove, k)
		}
	}

	if len(toAdd) == 0 && len(toRemove) == 0 {
		return nil
	}

	_, err = client.ChangeTagsForResource(ctx, &route53.ChangeTagsForResourceInput{
		ResourceType:  types.TagResourceTypeHostedzone,
		ResourceId:    aws.String(zoneID),
		AddTags:       toAdd,
		RemoveTagKeys: toRemove,
	})
	return err
}
