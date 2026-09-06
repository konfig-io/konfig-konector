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

// Package provider resolves which AWS account and region a reconcile should
// target. A Scope (credentials + region + account ID) is attached to the
// reconcile context; the generated multi-account clients in internal/aws/multi
// read it on every SDK call and override the per-operation Region and
// Credentials options accordingly. With no Scope in the context the base
// client's own credentials (EKS Pod Identity) and region are used unchanged.
package provider

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// Scope is the resolved AWS target for a single reconcile.
type Scope struct {
	// Name of the AWSProvider that produced this scope; empty for the default.
	Name string
	// AccountID is the 12-digit account the credentials belong to, if known.
	AccountID string
	// Region overrides the client region when non-empty.
	Region string
	// Credentials overrides the client credentials when non-nil.
	Credentials aws.CredentialsProvider
}

type scopeKey struct{}

// WithScope returns a context carrying s.
func WithScope(ctx context.Context, s *Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, s)
}

// ScopeFrom returns the Scope attached to ctx, or nil for the default scope.
func ScopeFrom(ctx context.Context) *Scope {
	s, _ := ctx.Value(scopeKey{}).(*Scope)
	return s
}
