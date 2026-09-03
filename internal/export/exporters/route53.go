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

package exporters

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsroute53 "github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	// Zones must be indexed before the records that live in them.
	export.Register(export.Exporter{Kind: "HostedZone", Service: "route53", Order: 15, Fn: exportHostedZones})
	export.Register(export.Exporter{Kind: "HealthCheck", Service: "route53", Order: 16, Fn: exportHealthChecks})
	export.Register(export.Exporter{Kind: "RecordSet", Service: "route53", Order: 17, Fn: exportRecordSets})
	export.Register(export.Exporter{Kind: "DelegationSignerRecord", Service: "route53", Order: 18, Fn: exportDelegationSignerRecords})
}

// route53TagMap converts Route53 tag slices to a spec tag map, dropping aws: tags.
func route53TagMap(tags []route53types.Tag) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return export.TagMap(m)
}

// zoneID strips the "/hostedzone/" prefix from a hosted zone Id field.
func zoneID(id string) string {
	return strings.TrimPrefix(id, "/hostedzone/")
}

func exportHostedZones(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsroute53.NewListHostedZonesPaginator(clients.Route53, &awsroute53.ListHostedZonesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list hosted zones: %w", err)
		}
		for _, z := range page.HostedZones {
			id := zoneID(aws.ToString(z.Id))
			name := aws.ToString(z.Name)
			cr := &awsv1alpha1.HostedZone{
				ObjectMeta: export.ObjectMeta(strings.TrimSuffix(name, "."), opts),
				Spec: awsv1alpha1.HostedZoneSpec{
					Name: name,
				},
			}
			if z.Config != nil {
				cr.Spec.Comment = aws.ToString(z.Config.Comment)
				cr.Spec.Private = z.Config.PrivateZone
			}
			// Private zones must carry the associated VPC; GetHostedZone
			// also returns the delegation set for public zones.
			if det, err := clients.Route53.GetHostedZone(ctx, &awsroute53.GetHostedZoneInput{Id: z.Id}); err == nil {
				if cr.Spec.Private && len(det.VPCs) > 0 {
					// The CRD models a single VPC association; additional
					// associated VPCs cannot round-trip.
					cr.Spec.VPCRef = &awsv1alpha1.VPCRef{
						ID:     aws.ToString(det.VPCs[0].VPCId),
						Region: string(det.VPCs[0].VPCRegion),
					}
				}
				if det.DelegationSet != nil && det.DelegationSet.Id != nil {
					cr.Spec.DelegationSetID = strings.TrimPrefix(aws.ToString(det.DelegationSet.Id), "/delegationset/")
				}
			}
			if tagsOut, err := clients.Route53.ListTagsForResource(ctx, &awsroute53.ListTagsForResourceInput{
				ResourceType: route53types.TagResourceTypeHostedzone,
				ResourceId:   aws.String(id),
			}); err == nil && tagsOut.ResourceTagSet != nil {
				cr.Spec.Tags = route53TagMap(tagsOut.ResourceTagSet.Tags)
			}
			opts.Index.Add(id, cr.Name)
			opts.Index.Add(aws.ToString(z.Id), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportHealthChecks(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsroute53.NewListHealthChecksPaginator(clients.Route53, &awsroute53.ListHealthChecksInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list health checks: %w", err)
		}
		for _, hc := range page.HealthChecks {
			id := aws.ToString(hc.Id)
			cfg := hc.HealthCheckConfig
			if cfg == nil {
				continue
			}
			cr := &awsv1alpha1.HealthCheck{
				ObjectMeta: export.ObjectMeta("healthcheck-"+id, opts),
				Spec: awsv1alpha1.HealthCheckSpec{
					Type:             string(cfg.Type),
					IPAddress:        aws.ToString(cfg.IPAddress),
					FQDN:             aws.ToString(cfg.FullyQualifiedDomainName),
					Port:             aws.ToInt32(cfg.Port),
					ResourcePath:     aws.ToString(cfg.ResourcePath),
					SearchString:     aws.ToString(cfg.SearchString),
					RequestInterval:  aws.ToInt32(cfg.RequestInterval),
					FailureThreshold: aws.ToInt32(cfg.FailureThreshold),
				},
			}
			if tagsOut, err := clients.Route53.ListTagsForResource(ctx, &awsroute53.ListTagsForResourceInput{
				ResourceType: route53types.TagResourceTypeHealthcheck,
				ResourceId:   aws.String(id),
			}); err == nil && tagsOut.ResourceTagSet != nil {
				cr.Spec.Tags = route53TagMap(tagsOut.ResourceTagSet.Tags)
				// Prefer the console-visible Name tag for the CR name.
				if n, ok := cr.Spec.Tags["Name"]; ok && n != "" {
					cr.ObjectMeta = export.ObjectMeta(n, opts)
				}
			}
			opts.Index.Add("healthcheck/"+id, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

// recordSetTypes is the set of record types the RecordSet CRD supports.
var recordSetTypes = map[route53types.RRType]bool{
	route53types.RRTypeA:     true,
	route53types.RRTypeAaaa:  true,
	route53types.RRTypeCname: true,
	route53types.RRTypeMx:    true,
	route53types.RRTypeTxt:   true,
	route53types.RRTypeNs:    true,
	route53types.RRTypeSoa:   true,
	route53types.RRTypeSrv:   true,
	route53types.RRTypeCaa:   true,
	route53types.RRTypePtr:   true,
}

// listAllRecordSets pages through ListResourceRecordSets, which has no SDK
// paginator, and calls fn for each record set.
func listAllRecordSets(ctx context.Context, clients *awsclient.Clients, id string, fn func(route53types.ResourceRecordSet)) error {
	input := &awsroute53.ListResourceRecordSetsInput{HostedZoneId: aws.String(id)}
	for {
		out, err := clients.Route53.ListResourceRecordSets(ctx, input)
		if err != nil {
			return err
		}
		for _, rr := range out.ResourceRecordSets {
			fn(rr)
		}
		if !out.IsTruncated {
			return nil
		}
		input.StartRecordName = out.NextRecordName
		input.StartRecordType = out.NextRecordType
		input.StartRecordIdentifier = out.NextRecordIdentifier
	}
}

func exportRecordSets(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsroute53.NewListHostedZonesPaginator(clients.Route53, &awsroute53.ListHostedZonesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list hosted zones: %w", err)
		}
		for _, z := range page.HostedZones {
			id := zoneID(aws.ToString(z.Id))
			zoneName := aws.ToString(z.Name)
			zoneCR, zoneExported := opts.Index.Lookup(id)
			err := listAllRecordSets(ctx, clients, id, func(rr route53types.ResourceRecordSet) {
				name := aws.ToString(rr.Name)
				// The zone's apex NS and SOA records are created and managed
				// by AWS with the zone itself — never export them.
				if name == zoneName && (rr.Type == route53types.RRTypeNs || rr.Type == route53types.RRTypeSoa) {
					return
				}
				if !recordSetTypes[rr.Type] {
					// Record type not modelled by the RecordSet CRD (e.g.
					// DS, TLSA, SSHFP); DS is exported separately.
					return
				}
				crName := name + "-" + strings.ToLower(string(rr.Type))
				if aws.ToString(rr.SetIdentifier) != "" {
					crName += "-" + aws.ToString(rr.SetIdentifier)
				}
				cr := &awsv1alpha1.RecordSet{
					ObjectMeta: export.ObjectMeta(crName, opts),
					Spec: awsv1alpha1.RecordSetSpec{
						Name:          name,
						Type:          string(rr.Type),
						TTL:           rr.TTL,
						Weight:        rr.Weight,
						SetIdentifier: aws.ToString(rr.SetIdentifier),
						Failover:      string(rr.Failover),
					},
				}
				if zoneExported {
					cr.Spec.HostedZoneRef = awsv1alpha1.HostedZoneRef{Name: zoneCR}
				} else {
					cr.Spec.HostedZoneRef = awsv1alpha1.HostedZoneRef{ID: id}
				}
				for _, v := range rr.ResourceRecords {
					cr.Spec.Records = append(cr.Spec.Records, aws.ToString(v.Value))
				}
				if rr.AliasTarget != nil {
					cr.Spec.Alias = &awsv1alpha1.AliasTarget{
						DNSName:              aws.ToString(rr.AliasTarget.DNSName),
						HostedZoneID:         aws.ToString(rr.AliasTarget.HostedZoneId),
						EvaluateTargetHealth: rr.AliasTarget.EvaluateTargetHealth,
					}
				}
				if hcID := aws.ToString(rr.HealthCheckId); hcID != "" {
					// HealthCheckRef is a ResourceRef (CR name only); the
					// association is dropped when the health check was not
					// exported in this run.
					if hcCR, ok := opts.Index.Lookup("healthcheck/" + hcID); ok {
						cr.Spec.HealthCheckRef = &awsv1alpha1.ResourceRef{Name: hcCR}
					}
				}
				objs = append(objs, cr)
			})
			if err != nil {
				// Per-zone failure (e.g. permissions): continue with the
				// next zone rather than abort the whole export.
				continue
			}
		}
	}
	return objs, nil
}

func exportDelegationSignerRecords(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsroute53.NewListHostedZonesPaginator(clients.Route53, &awsroute53.ListHostedZonesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list hosted zones: %w", err)
		}
		for _, z := range page.HostedZones {
			id := zoneID(aws.ToString(z.Id))
			err := listAllRecordSets(ctx, clients, id, func(rr route53types.ResourceRecordSet) {
				if rr.Type != route53types.RRTypeDs {
					return
				}
				child := aws.ToString(rr.Name)
				for i, v := range rr.ResourceRecords {
					crName := "ds-" + child
					if len(rr.ResourceRecords) > 1 {
						crName = fmt.Sprintf("%s-%d", crName, i)
					}
					cr := &awsv1alpha1.DelegationSignerRecord{
						ObjectMeta: export.ObjectMeta(crName, opts),
						Spec: awsv1alpha1.DelegationSignerRecordSpec{
							HostedZoneID:  id,
							ChildZoneName: child,
							DSRecord:      aws.ToString(v.Value),
							TTL:           rr.TTL,
						},
					}
					objs = append(objs, cr)
				}
			})
			if err != nil {
				// Per-zone failure: continue with the next zone.
				continue
			}
		}
	}
	return objs, nil
}
