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

// konfig-export walks an AWS account (one region) and renders every supported
// resource as konfig-konector CR YAML — the equivalent of GCP Config
// Connector's `config-connector export`.
//
// Usage:
//
//	konfig-export --namespace konfig-system > account.yaml
//	konfig-export --services ec2,iam,s3 --output ./export/
//	konfig-export --no-abandon   # exported CRs delete AWS resources when removed (dangerous)
//
// Credentials/region come from the standard AWS SDK chain (AWS_PROFILE,
// AWS_REGION, SSO, etc.). The command is read-only: it only calls
// List/Describe/Get APIs.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
	_ "github.com/konfig-io/konfig-konector/internal/export/exporters"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

func main() {
	var (
		namespace string
		services  string
		kinds     string
		output    string
		noAbandon bool
		list      bool
	)
	flag.StringVar(&namespace, "namespace", "konfig-system", "namespace for exported CRs")
	flag.StringVar(&services, "services", "", "comma-separated service filter (e.g. ec2,iam,s3); empty = all")
	flag.StringVar(&kinds, "kinds", "", "comma-separated kind filter (e.g. VPC,SQSQueue); empty = all")
	flag.StringVar(&output, "output", "-", "output: '-' for stdout, a file path, or a directory (one file per service)")
	flag.BoolVar(&noAbandon, "no-abandon", false, "omit the deletion-policy: abandon annotation (deleting an exported CR will then DELETE the AWS resource)")
	flag.BoolVar(&list, "list", false, "list supported services/kinds and exit")
	flag.Parse()

	if list {
		for _, e := range export.All() {
			fmt.Printf("%-24s %s\n", e.Service, e.Kind)
		}
		return
	}

	ctx := context.Background()
	clients, err := awsclient.NewClients(ctx)
	if err != nil {
		fatal("initialise AWS clients: %v", err)
	}

	svcFilter := toSet(services)
	kindFilter := toSet(kinds)

	opts := &export.Options{
		Namespace: namespace,
		Abandon:   !noAbandon,
		Index:     export.NewRefIndex(),
	}

	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		fatal("build scheme: %v", err)
	}

	type result struct {
		service string
		kind    string
		objs    []client.Object
	}
	var results []result
	var errs int

	// Sequential by Order: later exporters resolve refs against the index
	// populated by earlier ones (VPC before Subnet, IAMRole before attachments).
	for _, e := range export.All() {
		if len(svcFilter) > 0 && !svcFilter[e.Service] {
			continue
		}
		if len(kindFilter) > 0 && !kindFilter[strings.ToLower(e.Kind)] {
			continue
		}
		objs, err := e.Fn(ctx, clients, opts)
		if err != nil {
			// Missing permissions or unsubscribed services shouldn't abort the
			// whole account export; report and continue.
			fmt.Fprintf(os.Stderr, "WARN %s/%s: %v\n", e.Service, e.Kind, err)
			errs++
			continue
		}
		if len(objs) > 0 {
			results = append(results, result{e.Service, e.Kind, objs})
			fmt.Fprintf(os.Stderr, "%-16s %-32s %d\n", e.Service, e.Kind, len(objs))
		}
	}

	render := func(objs []client.Object) ([]byte, error) {
		var buf []byte
		for _, obj := range objs {
			gvks, _, err := scheme.ObjectKinds(obj)
			if err != nil || len(gvks) == 0 {
				return nil, fmt.Errorf("unknown object kind %T: %v", obj, err)
			}
			obj.GetObjectKind().SetGroupVersionKind(gvks[0])
			b, err := yaml.Marshal(obj)
			if err != nil {
				return nil, err
			}
			buf = append(buf, []byte("---\n")...)
			buf = append(buf, b...)
		}
		return buf, nil
	}

	total := 0
	for _, r := range results {
		total += len(r.objs)
	}

	switch {
	case output == "-" || output == "":
		for _, r := range results {
			b, err := render(r.objs)
			if err != nil {
				fatal("render %s: %v", r.kind, err)
			}
			os.Stdout.Write(b)
		}
	case isDir(output):
		byService := map[string][]client.Object{}
		for _, r := range results {
			byService[r.service] = append(byService[r.service], r.objs...)
		}
		svcs := make([]string, 0, len(byService))
		for s := range byService {
			svcs = append(svcs, s)
		}
		sort.Strings(svcs)
		for _, s := range svcs {
			b, err := render(byService[s])
			if err != nil {
				fatal("render %s: %v", s, err)
			}
			path := filepath.Join(output, s+".yaml")
			if err := os.WriteFile(path, b, 0o644); err != nil {
				fatal("write %s: %v", path, err)
			}
		}
	default:
		var all []byte
		for _, r := range results {
			b, err := render(r.objs)
			if err != nil {
				fatal("render %s: %v", r.kind, err)
			}
			all = append(all, b...)
		}
		if err := os.WriteFile(output, all, 0o644); err != nil {
			fatal("write %s: %v", output, err)
		}
	}

	fmt.Fprintf(os.Stderr, "\nexported %d resources", total)
	if errs > 0 {
		fmt.Fprintf(os.Stderr, " (%d exporters failed — see warnings above)", errs)
	}
	fmt.Fprintln(os.Stderr)
}

func toSet(csv string) map[string]bool {
	if csv == "" {
		return nil
	}
	m := map[string]bool{}
	for _, s := range strings.Split(csv, ",") {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" {
			m[s] = true
		}
	}
	return m
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ERROR: "+format+"\n", args...)
	os.Exit(1)
}
