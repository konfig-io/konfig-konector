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

// Package export discovers existing AWS resources and renders them as
// konfig-konector CRs — the equivalent of GCP Config Connector's
// `config-connector export` for an entire account/region.
package export

import (
	"context"
	"sort"
	"sync"

	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Options configures an export run.
type Options struct {
	// Namespace to set on every exported CR.
	Namespace string
	// Abandon controls whether exported CRs carry the
	// aws.konfig.io/deletion-policy: abandon annotation. Default true: a
	// freshly imported account must never be deletable by deleting CRs.
	Abandon bool
	// Index maps AWS identifiers to exported CR names so exporters can emit
	// name-based cross-resource refs instead of raw IDs.
	Index *RefIndex
}

// Exporter lists one kind of AWS resource and returns the equivalent CRs.
type Exporter struct {
	// Kind is the CRD kind this exporter produces (e.g. "SQSQueue").
	Kind string
	// Service groups exporters for --services filtering (e.g. "sqs", "ec2").
	Service string
	// Order within the whole run: exporters with lower Order run first so
	// that referenced resources (VPCs) are indexed before referrers (subnets).
	Order int
	// Fn performs the export.
	Fn func(ctx context.Context, clients *awsclient.Clients, opts *Options) ([]client.Object, error)
}

var (
	mu        sync.Mutex
	exporters []Exporter
)

// Register adds an exporter; called from init() in per-service files.
func Register(e Exporter) {
	mu.Lock()
	defer mu.Unlock()
	exporters = append(exporters, e)
}

// All returns registered exporters sorted by Order, then Service, then Kind.
func All() []Exporter {
	mu.Lock()
	defer mu.Unlock()
	out := make([]Exporter, len(exporters))
	copy(out, exporters)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		if out[i].Service != out[j].Service {
			return out[i].Service < out[j].Service
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

// RefIndex maps AWS identifiers (IDs/ARNs/names) to exported CR names.
type RefIndex struct {
	mu sync.RWMutex
	m  map[string]string
}

func NewRefIndex() *RefIndex {
	return &RefIndex{m: map[string]string{}}
}

// Add records that the AWS identifier id is managed by the CR named name.
func (r *RefIndex) Add(id, name string) {
	if id == "" || name == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[id] = name
}

// Lookup returns the CR name for an AWS identifier, if exported.
func (r *RefIndex) Lookup(id string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n, ok := r.m[id]
	return n, ok
}
