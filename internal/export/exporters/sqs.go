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

// Package exporters registers per-service export functions. Each file covers
// one AWS service and follows the same shape: paginate the List/Describe API,
// map live state into the CR spec (spec fields only — never status), record
// identifiers in the RefIndex, and resolve cross-resource refs via the index.
package exporters

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	export.Register(export.Exporter{
		Kind: "SQSQueue", Service: "sqs", Order: 40,
		Fn: exportSQSQueues,
	})
}

func exportSQSQueues(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awssqs.NewListQueuesPaginator(clients.SQS, &awssqs.ListQueuesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list queues: %w", err)
		}
		for _, url := range page.QueueUrls {
			attrsOut, err := clients.SQS.GetQueueAttributes(ctx, &awssqs.GetQueueAttributesInput{
				QueueUrl:       aws.String(url),
				AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameAll},
			})
			if err != nil {
				return nil, fmt.Errorf("get attributes %s: %w", url, err)
			}
			a := attrsOut.Attributes

			name := url[strings.LastIndex(url, "/")+1:]
			q := &awsv1alpha1.SQSQueue{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.SQSQueueSpec{
					QueueName:              name,
					FIFO:                   a[string(sqstypes.QueueAttributeNameFifoQueue)] == "true",
					VisibilityTimeout:      atoi32(a[string(sqstypes.QueueAttributeNameVisibilityTimeout)]),
					MessageRetentionPeriod: atoi32(a[string(sqstypes.QueueAttributeNameMessageRetentionPeriod)]),
					DelaySeconds:           atoi32(a[string(sqstypes.QueueAttributeNameDelaySeconds)]),
					ReceiveMessageWaitTime: atoi32(a[string(sqstypes.QueueAttributeNameReceiveMessageWaitTimeSeconds)]),
					KMSKeyID:               a["KmsMasterKeyId"],
					Policy:                 a[string(sqstypes.QueueAttributeNamePolicy)],
				},
			}

			// Redrive policy: reference the DLQ by CR name. All queues are
			// listed in this loop, so the DLQ's CR name is derivable from
			// its ARN without ordering concerns.
			if rp := a[string(sqstypes.QueueAttributeNameRedrivePolicy)]; rp != "" {
				var parsed struct {
					DeadLetterTargetArn string `json:"deadLetterTargetArn"`
					MaxReceiveCount     int32  `json:"maxReceiveCount"`
				}
				if err := json.Unmarshal([]byte(rp), &parsed); err == nil && parsed.DeadLetterTargetArn != "" {
					dlqName := parsed.DeadLetterTargetArn[strings.LastIndex(parsed.DeadLetterTargetArn, ":")+1:]
					q.Spec.RedrivePolicy = &awsv1alpha1.SQSRedrivePolicy{
						DeadLetterQueueRef: export.CRName(dlqName),
						MaxReceiveCount:    parsed.MaxReceiveCount,
					}
				}
			}

			tagsOut, err := clients.SQS.ListQueueTags(ctx, &awssqs.ListQueueTagsInput{QueueUrl: aws.String(url)})
			if err == nil {
				q.Spec.Tags = export.TagMap(tagsOut.Tags)
			}

			opts.Index.Add(a[string(sqstypes.QueueAttributeNameQueueArn)], q.Name)
			opts.Index.Add(url, q.Name)
			objs = append(objs, q)
		}
	}
	return objs, nil
}

func atoi32(s string) int32 {
	n, _ := strconv.Atoi(s)
	return int32(n)
}
