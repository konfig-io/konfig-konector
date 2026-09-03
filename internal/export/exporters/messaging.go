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
	awseventbridge "github.com/aws/aws-sdk-go-v2/service/eventbridge"
	awsfirehose "github.com/aws/aws-sdk-go-v2/service/firehose"
	awskafka "github.com/aws/aws-sdk-go-v2/service/kafka"
	kafkatypes "github.com/aws/aws-sdk-go-v2/service/kafka/types"
	awskinesis "github.com/aws/aws-sdk-go-v2/service/kinesis"
	kinesistypes "github.com/aws/aws-sdk-go-v2/service/kinesis/types"
	awspipes "github.com/aws/aws-sdk-go-v2/service/pipes"
	awssesv2 "github.com/aws/aws-sdk-go-v2/service/sesv2"
	sesv2types "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	awssns "github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	// Topics and buses before subscriptions/rules/targets that reference them.
	export.Register(export.Exporter{Kind: "SNSTopic", Service: "sns", Order: 40, Fn: exportSNSTopics})
	export.Register(export.Exporter{Kind: "SNSSubscription", Service: "sns", Order: 41, Fn: exportSNSSubscriptions})
	export.Register(export.Exporter{Kind: "EventBus", Service: "events", Order: 40, Fn: exportEventBuses})
	export.Register(export.Exporter{Kind: "EventRule", Service: "events", Order: 41, Fn: exportEventRules})
	export.Register(export.Exporter{Kind: "EventTarget", Service: "events", Order: 42, Fn: exportEventTargets})
	export.Register(export.Exporter{Kind: "EventBridgePipe", Service: "pipes", Order: 43, Fn: exportEventBridgePipes})
	export.Register(export.Exporter{Kind: "KinesisStream", Service: "kinesis", Order: 40, Fn: exportKinesisStreams})
	export.Register(export.Exporter{Kind: "KinesisStreamConsumer", Service: "kinesis", Order: 41, Fn: exportKinesisStreamConsumers})
	export.Register(export.Exporter{Kind: "FirehoseDeliveryStream", Service: "firehose", Order: 42, Fn: exportFirehoseDeliveryStreams})
	export.Register(export.Exporter{Kind: "MSKConfiguration", Service: "msk", Order: 40, Fn: exportMSKConfigurations})
	export.Register(export.Exporter{Kind: "MSKCluster", Service: "msk", Order: 41, Fn: exportMSKClusters})
	export.Register(export.Exporter{Kind: "MSKServerlessCluster", Service: "msk", Order: 41, Fn: exportMSKServerlessClusters})
	export.Register(export.Exporter{Kind: "SESConfigurationSet", Service: "ses", Order: 40, Fn: exportSESConfigurationSets})
	export.Register(export.Exporter{Kind: "SESEmailIdentity", Service: "ses", Order: 41, Fn: exportSESEmailIdentities})
}

func snsTagMap(tags []snstypes.Tag) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return export.TagMap(m)
}

func exportSNSTopics(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awssns.NewListTopicsPaginator(clients.SNS, &awssns.ListTopicsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list topics: %w", err)
		}
		for _, t := range page.Topics {
			arn := aws.ToString(t.TopicArn)
			name := arn[strings.LastIndex(arn, ":")+1:]
			attrsOut, err := clients.SNS.GetTopicAttributes(ctx, &awssns.GetTopicAttributesInput{TopicArn: t.TopicArn})
			if err != nil {
				continue
			}
			a := attrsOut.Attributes
			topic := &awsv1alpha1.SNSTopic{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.SNSTopicSpec{
					TopicName:                 name,
					FIFO:                      a["FifoTopic"] == "true",
					ContentBasedDeduplication: a["ContentBasedDeduplication"] == "true",
					KMSKeyID:                  a["KmsMasterKeyId"],
					Policy:                    a["Policy"],
				},
			}
			tagsOut, err := clients.SNS.ListTagsForResource(ctx, &awssns.ListTagsForResourceInput{ResourceArn: t.TopicArn})
			if err == nil {
				topic.Spec.Tags = snsTagMap(tagsOut.Tags)
			}
			opts.Index.Add(arn, topic.Name)
			objs = append(objs, topic)
		}
	}
	return objs, nil
}

func exportSNSSubscriptions(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awssns.NewListSubscriptionsPaginator(clients.SNS, &awssns.ListSubscriptionsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list subscriptions: %w", err)
		}
		for _, s := range page.Subscriptions {
			subArn := aws.ToString(s.SubscriptionArn)
			// Unconfirmed subscriptions have no real ARN ("PendingConfirmation")
			// and cannot be adopted.
			if !strings.HasPrefix(subArn, "arn:") {
				continue
			}
			topicArn := aws.ToString(s.TopicArn)
			topicName := topicArn[strings.LastIndex(topicArn, ":")+1:]
			// Subscription ARN suffix (UUID) keeps CR names unique per topic.
			suffix := subArn[strings.LastIndex(subArn, ":")+1:]
			if len(suffix) > 8 {
				suffix = suffix[:8]
			}
			sub := &awsv1alpha1.SNSSubscription{
				ObjectMeta: export.ObjectMeta(topicName+"-"+aws.ToString(s.Protocol)+"-"+suffix, opts),
				Spec: awsv1alpha1.SNSSubscriptionSpec{
					Protocol: aws.ToString(s.Protocol),
					Endpoint: aws.ToString(s.Endpoint),
				},
			}
			if crName, ok := opts.Index.Lookup(topicArn); ok {
				sub.Spec.TopicRef = awsv1alpha1.SNSTopicRef{Name: crName}
			} else {
				sub.Spec.TopicRef = awsv1alpha1.SNSTopicRef{ARN: topicArn}
			}
			attrsOut, err := clients.SNS.GetSubscriptionAttributes(ctx, &awssns.GetSubscriptionAttributesInput{
				SubscriptionArn: s.SubscriptionArn,
			})
			if err == nil {
				sub.Spec.FilterPolicy = attrsOut.Attributes["FilterPolicy"]
				sub.Spec.RawMessageDelivery = attrsOut.Attributes["RawMessageDelivery"] == "true"
			}
			opts.Index.Add(subArn, sub.Name)
			objs = append(objs, sub)
		}
	}
	return objs, nil
}

func exportEventBuses(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	// ListEventBuses has no SDK paginator; page manually.
	input := &awseventbridge.ListEventBusesInput{}
	for {
		out, err := clients.EventBridge.ListEventBuses(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("list event buses: %w", err)
		}
		for _, b := range out.EventBuses {
			name := aws.ToString(b.Name)
			// The default bus always exists and cannot be created/deleted.
			if name == "default" {
				continue
			}
			bus := &awsv1alpha1.EventBus{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec:       awsv1alpha1.EventBusSpec{EventBusName: name},
			}
			tagsOut, err := clients.EventBridge.ListTagsForResource(ctx, &awseventbridge.ListTagsForResourceInput{
				ResourceARN: b.Arn,
			})
			if err == nil && len(tagsOut.Tags) > 0 {
				m := make(map[string]string, len(tagsOut.Tags))
				for _, t := range tagsOut.Tags {
					m[aws.ToString(t.Key)] = aws.ToString(t.Value)
				}
				bus.Spec.Tags = export.TagMap(m)
			}
			opts.Index.Add(aws.ToString(b.Arn), bus.Name)
			opts.Index.Add(name, bus.Name)
			objs = append(objs, bus)
		}
		if aws.ToString(out.NextToken) == "" {
			break
		}
		input.NextToken = out.NextToken
	}
	return objs, nil
}

// eventBusNames lists all event bus names including "default".
func eventBusNames(ctx context.Context, clients *awsclient.Clients) ([]string, error) {
	var names []string
	input := &awseventbridge.ListEventBusesInput{}
	for {
		out, err := clients.EventBridge.ListEventBuses(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("list event buses: %w", err)
		}
		for _, b := range out.EventBuses {
			names = append(names, aws.ToString(b.Name))
		}
		if aws.ToString(out.NextToken) == "" {
			break
		}
		input.NextToken = out.NextToken
	}
	return names, nil
}

// eventBusRefFor builds the optional bus ref: nil for the default bus,
// CR-name ref when the bus was exported, raw name otherwise.
func eventBusRefFor(busName string, opts *export.Options) *awsv1alpha1.EventBusRef {
	if busName == "" || busName == "default" {
		return nil
	}
	if crName, ok := opts.Index.Lookup(busName); ok {
		return &awsv1alpha1.EventBusRef{Name: crName}
	}
	return &awsv1alpha1.EventBusRef{EventBusName: busName}
}

// skipEventRule reports whether a rule is AWS-managed and must not be exported.
func skipEventRule(name, managedBy string) bool {
	return managedBy != "" || strings.HasPrefix(name, "AWS")
}

func exportEventRules(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	buses, err := eventBusNames(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	for _, bus := range buses {
		// ListRules has no SDK paginator; page manually.
		input := &awseventbridge.ListRulesInput{EventBusName: aws.String(bus)}
		for {
			out, err := clients.EventBridge.ListRules(ctx, input)
			if err != nil {
				// Per-bus failure: keep exporting other buses.
				break
			}
			for _, r := range out.Rules {
				name := aws.ToString(r.Name)
				if skipEventRule(name, aws.ToString(r.ManagedBy)) {
					continue
				}
				crName := name
				if bus != "default" {
					crName = bus + "-" + name
				}
				rule := &awsv1alpha1.EventRule{
					ObjectMeta: export.ObjectMeta(crName, opts),
					Spec: awsv1alpha1.EventRuleSpec{
						EventBusRef:        eventBusRefFor(bus, opts),
						RuleName:           name,
						Description:        aws.ToString(r.Description),
						EventPattern:       aws.ToString(r.EventPattern),
						ScheduleExpression: aws.ToString(r.ScheduleExpression),
						State:              string(r.State),
						RoleARN:            aws.ToString(r.RoleArn),
					},
				}
				tagsOut, err := clients.EventBridge.ListTagsForResource(ctx, &awseventbridge.ListTagsForResourceInput{
					ResourceARN: r.Arn,
				})
				if err == nil && len(tagsOut.Tags) > 0 {
					m := make(map[string]string, len(tagsOut.Tags))
					for _, t := range tagsOut.Tags {
						m[aws.ToString(t.Key)] = aws.ToString(t.Value)
					}
					rule.Spec.Tags = export.TagMap(m)
				}
				opts.Index.Add(aws.ToString(r.Arn), rule.Name)
				opts.Index.Add(bus+"/"+name, rule.Name)
				objs = append(objs, rule)
			}
			if aws.ToString(out.NextToken) == "" {
				break
			}
			input.NextToken = out.NextToken
		}
	}
	return objs, nil
}

func exportEventTargets(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	buses, err := eventBusNames(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	for _, bus := range buses {
		input := &awseventbridge.ListRulesInput{EventBusName: aws.String(bus)}
		for {
			out, err := clients.EventBridge.ListRules(ctx, input)
			if err != nil {
				break
			}
			for _, r := range out.Rules {
				ruleName := aws.ToString(r.Name)
				if skipEventRule(ruleName, aws.ToString(r.ManagedBy)) {
					continue
				}
				// One EventTarget CR aggregates all targets of a rule.
				var entries []awsv1alpha1.EventTargetEntry
				tinput := &awseventbridge.ListTargetsByRuleInput{
					Rule:         r.Name,
					EventBusName: aws.String(bus),
				}
				for {
					tout, err := clients.EventBridge.ListTargetsByRule(ctx, tinput)
					if err != nil {
						break
					}
					for _, t := range tout.Targets {
						entries = append(entries, awsv1alpha1.EventTargetEntry{
							ID:        aws.ToString(t.Id),
							ARN:       aws.ToString(t.Arn),
							RoleARN:   aws.ToString(t.RoleArn),
							Input:     aws.ToString(t.Input),
							InputPath: aws.ToString(t.InputPath),
						})
					}
					if aws.ToString(tout.NextToken) == "" {
						break
					}
					tinput.NextToken = tout.NextToken
				}
				if len(entries) == 0 {
					continue
				}
				crName := ruleName + "-targets"
				if bus != "default" {
					crName = bus + "-" + crName
				}
				et := &awsv1alpha1.EventTarget{
					ObjectMeta: export.ObjectMeta(crName, opts),
					Spec: awsv1alpha1.EventTargetSpec{
						EventBusRef: eventBusRefFor(bus, opts),
						Targets:     entries,
					},
				}
				if ruleCR, ok := opts.Index.Lookup(bus + "/" + ruleName); ok {
					et.Spec.EventRuleRef = awsv1alpha1.EventRuleRef{Name: ruleCR}
				} else {
					et.Spec.EventRuleRef = awsv1alpha1.EventRuleRef{RuleName: ruleName}
				}
				objs = append(objs, et)
			}
			if aws.ToString(out.NextToken) == "" {
				break
			}
			input.NextToken = out.NextToken
		}
	}
	return objs, nil
}

func exportEventBridgePipes(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awspipes.NewListPipesPaginator(clients.Pipes, &awspipes.ListPipesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list pipes: %w", err)
		}
		for _, summary := range page.Pipes {
			name := aws.ToString(summary.Name)
			// DescribePipe fills in role ARN, description, and tags.
			out, err := clients.Pipes.DescribePipe(ctx, &awspipes.DescribePipeInput{Name: summary.Name})
			if err != nil {
				continue
			}
			pipe := &awsv1alpha1.EventBridgePipe{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.EventBridgePipeSpec{
					Name:         name,
					RoleARN:      aws.ToString(out.RoleArn),
					Source:       aws.ToString(out.Source),
					Target:       aws.ToString(out.Target),
					Description:  aws.ToString(out.Description),
					DesiredState: string(out.DesiredState),
					Enrichment:   aws.ToString(out.Enrichment),
					Tags:         export.TagMap(out.Tags),
				},
			}
			opts.Index.Add(aws.ToString(out.Arn), pipe.Name)
			objs = append(objs, pipe)
		}
	}
	return objs, nil
}

func exportKinesisStreams(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awskinesis.NewListStreamsPaginator(clients.Kinesis, &awskinesis.ListStreamsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list streams: %w", err)
		}
		for _, summary := range page.StreamSummaries {
			if summary.StreamStatus != kinesistypes.StreamStatusActive {
				continue
			}
			name := aws.ToString(summary.StreamName)
			out, err := clients.Kinesis.DescribeStreamSummary(ctx, &awskinesis.DescribeStreamSummaryInput{
				StreamARN: summary.StreamARN,
			})
			if err != nil || out.StreamDescriptionSummary == nil {
				continue
			}
			d := out.StreamDescriptionSummary
			cr := &awsv1alpha1.KinesisStream{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.KinesisStreamSpec{
					StreamName:           name,
					RetentionPeriodHours: d.RetentionPeriodHours,
				},
			}
			if d.StreamModeDetails != nil {
				cr.Spec.StreamMode = string(d.StreamModeDetails.StreamMode)
			}
			if cr.Spec.StreamMode != string(kinesistypes.StreamModeOnDemand) {
				cr.Spec.ShardCount = d.OpenShardCount
			}
			tagsOut, err := clients.Kinesis.ListTagsForStream(ctx, &awskinesis.ListTagsForStreamInput{
				StreamName: summary.StreamName,
			})
			if err == nil && len(tagsOut.Tags) > 0 {
				m := make(map[string]string, len(tagsOut.Tags))
				for _, t := range tagsOut.Tags {
					m[aws.ToString(t.Key)] = aws.ToString(t.Value)
				}
				cr.Spec.Tags = export.TagMap(m)
			}
			opts.Index.Add(aws.ToString(d.StreamARN), cr.Name)
			opts.Index.Add(name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportKinesisStreamConsumers(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	// Consumers are listed per stream ARN.
	var streamARNs []string
	sp := awskinesis.NewListStreamsPaginator(clients.Kinesis, &awskinesis.ListStreamsInput{})
	for sp.HasMorePages() {
		page, err := sp.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list streams: %w", err)
		}
		for _, s := range page.StreamSummaries {
			streamARNs = append(streamARNs, aws.ToString(s.StreamARN))
		}
	}

	var objs []client.Object
	for _, streamARN := range streamARNs {
		streamName := streamARN[strings.LastIndex(streamARN, "/")+1:]
		cp := awskinesis.NewListStreamConsumersPaginator(clients.Kinesis, &awskinesis.ListStreamConsumersInput{
			StreamARN: aws.String(streamARN),
		})
		for cp.HasMorePages() {
			page, err := cp.NextPage(ctx)
			if err != nil {
				// Per-stream failure: keep exporting other streams.
				break
			}
			for _, c := range page.Consumers {
				cr := &awsv1alpha1.KinesisStreamConsumer{
					ObjectMeta: export.ObjectMeta(streamName+"-"+aws.ToString(c.ConsumerName), opts),
					Spec: awsv1alpha1.KinesisStreamConsumerSpec{
						ConsumerName: aws.ToString(c.ConsumerName),
						StreamARN:    streamARN,
					},
				}
				opts.Index.Add(aws.ToString(c.ConsumerARN), cr.Name)
				objs = append(objs, cr)
			}
		}
	}
	return objs, nil
}

func exportFirehoseDeliveryStreams(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	// ListDeliveryStreams has no SDK paginator; page manually.
	var names []string
	input := &awsfirehose.ListDeliveryStreamsInput{}
	for {
		out, err := clients.Firehose.ListDeliveryStreams(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("list delivery streams: %w", err)
		}
		names = append(names, out.DeliveryStreamNames...)
		if !aws.ToBool(out.HasMoreDeliveryStreams) || len(out.DeliveryStreamNames) == 0 {
			break
		}
		input.ExclusiveStartDeliveryStreamName = aws.String(out.DeliveryStreamNames[len(out.DeliveryStreamNames)-1])
	}

	var objs []client.Object
	for _, name := range names {
		out, err := clients.Firehose.DescribeDeliveryStream(ctx, &awsfirehose.DescribeDeliveryStreamInput{
			DeliveryStreamName: aws.String(name),
		})
		if err != nil || out.DeliveryStreamDescription == nil {
			continue
		}
		d := out.DeliveryStreamDescription
		cr := &awsv1alpha1.FirehoseDeliveryStream{
			ObjectMeta: export.ObjectMeta(name, opts),
			Spec: awsv1alpha1.FirehoseDeliveryStreamSpec{
				DeliveryStreamName: name,
				DeliveryStreamType: string(d.DeliveryStreamType),
			},
		}
		// The CRD only models an S3 destination; ExtendedS3 (console default)
		// carries the same base fields.
		for _, dest := range d.Destinations {
			if s3 := dest.ExtendedS3DestinationDescription; s3 != nil {
				spec := &awsv1alpha1.FirehoseS3Destination{
					BucketARN: aws.ToString(s3.BucketARN),
					RoleARN:   aws.ToString(s3.RoleARN),
					Prefix:    aws.ToString(s3.Prefix),
				}
				if s3.BufferingHints != nil {
					spec.BufferingIntervalSeconds = s3.BufferingHints.IntervalInSeconds
					spec.BufferingSizeMBs = s3.BufferingHints.SizeInMBs
				}
				cr.Spec.S3DestinationConfiguration = spec
				break
			}
			if s3 := dest.S3DestinationDescription; s3 != nil {
				spec := &awsv1alpha1.FirehoseS3Destination{
					BucketARN: aws.ToString(s3.BucketARN),
					RoleARN:   aws.ToString(s3.RoleARN),
					Prefix:    aws.ToString(s3.Prefix),
				}
				if s3.BufferingHints != nil {
					spec.BufferingIntervalSeconds = s3.BufferingHints.IntervalInSeconds
					spec.BufferingSizeMBs = s3.BufferingHints.SizeInMBs
				}
				cr.Spec.S3DestinationConfiguration = spec
				break
			}
		}
		tagsOut, err := clients.Firehose.ListTagsForDeliveryStream(ctx, &awsfirehose.ListTagsForDeliveryStreamInput{
			DeliveryStreamName: aws.String(name),
		})
		if err == nil && len(tagsOut.Tags) > 0 {
			m := make(map[string]string, len(tagsOut.Tags))
			for _, t := range tagsOut.Tags {
				m[aws.ToString(t.Key)] = aws.ToString(t.Value)
			}
			cr.Spec.Tags = export.TagMap(m)
		}
		opts.Index.Add(aws.ToString(d.DeliveryStreamARN), cr.Name)
		objs = append(objs, cr)
	}
	return objs, nil
}

func exportMSKConfigurations(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awskafka.NewListConfigurationsPaginator(clients.Kafka, &awskafka.ListConfigurationsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list msk configurations: %w", err)
		}
		for _, c := range page.Configurations {
			if c.State != kafkatypes.ConfigurationStateActive {
				continue
			}
			name := aws.ToString(c.Name)
			cr := &awsv1alpha1.MSKConfiguration{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.MSKConfigurationSpec{
					Name:          name,
					KafkaVersions: c.KafkaVersions,
					Description:   aws.ToString(c.Description),
				},
			}
			// ServerProperties requires a revision-level describe call.
			if c.LatestRevision != nil {
				revOut, err := clients.Kafka.DescribeConfigurationRevision(ctx, &awskafka.DescribeConfigurationRevisionInput{
					Arn:      c.Arn,
					Revision: c.LatestRevision.Revision,
				})
				if err == nil {
					cr.Spec.ServerProperties = string(revOut.ServerProperties)
				}
			}
			opts.Index.Add(aws.ToString(c.Arn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportMSKClusters(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awskafka.NewListClustersV2Paginator(clients.Kafka, &awskafka.ListClustersV2Input{
		ClusterTypeFilter: aws.String("PROVISIONED"),
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list msk clusters: %w", err)
		}
		for _, c := range page.ClusterInfoList {
			if c.State != kafkatypes.ClusterStateActive || c.Provisioned == nil {
				continue
			}
			name := aws.ToString(c.ClusterName)
			prov := c.Provisioned
			cr := &awsv1alpha1.MSKCluster{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.MSKClusterSpec{
					ClusterName:         name,
					NumberOfBrokerNodes: aws.ToInt32(prov.NumberOfBrokerNodes),
					Tags:                export.TagMap(c.Tags),
				},
			}
			if prov.CurrentBrokerSoftwareInfo != nil {
				cr.Spec.KafkaVersion = aws.ToString(prov.CurrentBrokerSoftwareInfo.KafkaVersion)
			}
			if bng := prov.BrokerNodeGroupInfo; bng != nil {
				spec := awsv1alpha1.MSKBrokerNodeGroupInfo{
					InstanceType:   aws.ToString(bng.InstanceType),
					ClientSubnets:  bng.ClientSubnets,
					SecurityGroups: bng.SecurityGroups,
				}
				if bng.StorageInfo != nil && bng.StorageInfo.EbsStorageInfo != nil {
					spec.StorageVolumeSizeGiB = bng.StorageInfo.EbsStorageInfo.VolumeSize
				}
				cr.Spec.BrokerNodeGroupInfo = spec
			}
			opts.Index.Add(aws.ToString(c.ClusterArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportMSKServerlessClusters(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awskafka.NewListClustersV2Paginator(clients.Kafka, &awskafka.ListClustersV2Input{
		ClusterTypeFilter: aws.String("SERVERLESS"),
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list msk serverless clusters: %w", err)
		}
		for _, c := range page.ClusterInfoList {
			if c.State != kafkatypes.ClusterStateActive || c.Serverless == nil {
				continue
			}
			name := aws.ToString(c.ClusterName)
			cr := &awsv1alpha1.MSKServerlessCluster{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.MSKServerlessClusterSpec{
					ClusterName: name,
					Tags:        export.TagMap(c.Tags),
				},
			}
			for _, vc := range c.Serverless.VpcConfigs {
				cr.Spec.VpcConfigs = append(cr.Spec.VpcConfigs, awsv1alpha1.MSKServerlessVpcConfig{
					SubnetIDs:        vc.SubnetIds,
					SecurityGroupIDs: vc.SecurityGroupIds,
				})
			}
			opts.Index.Add(aws.ToString(c.ClusterArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func sesTagMap(tags []sesv2types.Tag) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return export.TagMap(m)
}

func exportSESConfigurationSets(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awssesv2.NewListConfigurationSetsPaginator(clients.SESv2, &awssesv2.ListConfigurationSetsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list configuration sets: %w", err)
		}
		for _, name := range page.ConfigurationSets {
			cr := &awsv1alpha1.SESConfigurationSet{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec:       awsv1alpha1.SESConfigurationSetSpec{ConfigurationSetName: name},
			}
			out, err := clients.SESv2.GetConfigurationSet(ctx, &awssesv2.GetConfigurationSetInput{
				ConfigurationSetName: aws.String(name),
			})
			if err == nil {
				if out.SendingOptions != nil {
					cr.Spec.SendingEnabled = aws.Bool(out.SendingOptions.SendingEnabled)
				}
				if out.ReputationOptions != nil {
					cr.Spec.ReputationMetricsEnabled = aws.Bool(out.ReputationOptions.ReputationMetricsEnabled)
				}
				cr.Spec.Tags = sesTagMap(out.Tags)
			}
			opts.Index.Add(name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportSESEmailIdentities(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awssesv2.NewListEmailIdentitiesPaginator(clients.SESv2, &awssesv2.ListEmailIdentitiesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list email identities: %w", err)
		}
		for _, id := range page.EmailIdentities {
			name := aws.ToString(id.IdentityName)
			cr := &awsv1alpha1.SESEmailIdentity{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec:       awsv1alpha1.SESEmailIdentitySpec{EmailIdentity: name},
			}
			out, err := clients.SESv2.GetEmailIdentity(ctx, &awssesv2.GetEmailIdentityInput{
				EmailIdentity: id.IdentityName,
			})
			if err == nil {
				cr.Spec.ConfigurationSetName = aws.ToString(out.ConfigurationSetName)
				cr.Spec.Tags = sesTagMap(out.Tags)
			}
			opts.Index.Add(name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}
