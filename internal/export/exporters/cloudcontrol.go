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
	awscc "github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/cfn"
	"github.com/konfig-io/konfig-konector/internal/export"
)

// registerCloudControlExporter wires a generated Cloud Control-backed kind
// into konfig-export: ListResources for the type, GetResource for each, and
// the live properties mapped back into the typed spec. Runs after every
// native exporter so identifiers of native kinds are already indexed.
func registerCloudControlExporter(kind, typeName, service string, newObj func() cfn.CloudControlObject) {
	export.Register(export.Exporter{
		Kind: kind, Service: service, Order: 900,
		Fn: func(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
			var objs []client.Object
			p := awscc.NewListResourcesPaginator(clients.CloudControl, &awscc.ListResourcesInput{TypeName: aws.String(typeName)})
			for p.HasMorePages() {
				page, err := p.NextPage(ctx)
				if err != nil {
					// Types that need extra list parameters (parent identifiers)
					// cannot be enumerated generically; report and move on.
					return objs, fmt.Errorf("list %s: %w", typeName, err)
				}
				for _, rd := range page.ResourceDescriptions {
					obj := newObj()
					if err := cfn.FromProperties([]byte(aws.ToString(rd.Properties)), obj.CloudControlSpec()); err != nil {
						continue
					}
					id := aws.ToString(rd.Identifier)
					name := strings.NewReplacer("|", "-", "/", "-", ":", "-", "_", "-").Replace(id)
					meta := export.ObjectMeta(name, opts)
					obj.SetName(meta.Name)
					obj.SetNamespace(meta.Namespace)
					opts.Index.Add(id, obj.GetName())
					objs = append(objs, obj)
				}
			}
			return objs, nil
		},
	})
}
