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

package provider

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	corev1 "k8s.io/api/core/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// STSAPI is the subset of STS used by the resolver.
type STSAPI interface {
	AssumeRole(ctx context.Context, params *sts.AssumeRoleInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleOutput, error)
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// ProviderScoped is implemented by every namespaced resource kind (generated
// GetProviderRef accessors) so controllers can resolve its AWSProvider.
type ProviderScoped interface {
	GetProviderRef() *awsv1alpha1.ProviderRef
	GetNamespace() string
}

// Resolver turns AWSProvider objects into Scopes, caching assumed-role
// credential providers per provider generation.
type Resolver struct {
	Client client.Client
	// STS is the operator-credential STS client used for role assumption. It
	// MUST be an unscoped client (*sts.Client, not the multi wrapper): the
	// wrapper applies the scope's credentials to every call, and the scope's
	// credentials are the assume-role cache itself, which deadlocks.
	STS STSAPI
	// BaseCredentials are the operator's own credentials (Pod Identity).
	BaseCredentials aws.CredentialsProvider
	// BaseRegion is the operator's own region.
	BaseRegion string

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	generation int64
	creds      aws.CredentialsProvider
	accountID  string
}

// ErrNotPermitted is returned when a namespace references a provider whose
// AllowedNamespaces excludes it.
type ErrNotPermitted struct{ Provider, Namespace string }

func (e *ErrNotPermitted) Error() string {
	return fmt.Sprintf("AWSProvider %q does not allow namespace %q", e.Provider, e.Namespace)
}

// ForObject resolves the Scope for a namespaced resource following the
// documented resolution order. A nil Scope means "use operator defaults".
func (r *Resolver) ForObject(ctx context.Context, obj ProviderScoped) (*Scope, error) {
	ref := obj.GetProviderRef()
	name, regionOverride := "", ""
	if ref != nil {
		name, regionOverride = ref.Name, ref.Region
	}
	if name == "" && r.Client != nil {
		ns := &corev1.Namespace{}
		if err := r.Client.Get(ctx, k8stypes.NamespacedName{Name: obj.GetNamespace()}, ns); err == nil {
			name = ns.Annotations[awsv1alpha1.ProviderAnnotation]
		}
	}
	if name == "" {
		if def, err := r.defaultProvider(ctx); err != nil {
			return nil, err
		} else if def != nil {
			s, err := r.ForProvider(ctx, def, 0)
			if err != nil {
				return nil, err
			}
			if regionOverride != "" {
				s.Region = regionOverride
			}
			return s, nil
		}
		if regionOverride == "" {
			return nil, nil
		}
		return &Scope{Region: regionOverride, Credentials: r.BaseCredentials}, nil
	}
	s, err := r.ForName(ctx, name, obj.GetNamespace())
	if err != nil {
		return nil, err
	}
	if regionOverride != "" {
		s.Region = regionOverride
	}
	return s, nil
}

// defaultProvider returns the AWSProvider with spec.default set (lowest name
// wins when several are marked), or nil when none is.
func (r *Resolver) defaultProvider(ctx context.Context) (*awsv1alpha1.AWSProvider, error) {
	if r.Client == nil {
		return nil, nil
	}
	list := &awsv1alpha1.AWSProviderList{}
	if err := r.Client.List(ctx, list); err != nil {
		return nil, fmt.Errorf("list AWSProviders: %w", err)
	}
	var def *awsv1alpha1.AWSProvider
	for i := range list.Items {
		p := &list.Items[i]
		if p.Spec.Default && (def == nil || p.Name < def.Name) {
			def = p
		}
	}
	return def, nil
}

// ForName resolves the named AWSProvider. namespace is checked against
// AllowedNamespaces; pass "" to skip the check (cluster-scoped callers).
func (r *Resolver) ForName(ctx context.Context, name, namespace string) (*Scope, error) {
	p := &awsv1alpha1.AWSProvider{}
	if err := r.Client.Get(ctx, k8stypes.NamespacedName{Name: name}, p); err != nil {
		return nil, fmt.Errorf("resolve AWSProvider %q: %w", name, err)
	}
	if namespace != "" && !namespaceAllowed(p.Spec.AllowedNamespaces, namespace) {
		return nil, &ErrNotPermitted{Provider: name, Namespace: namespace}
	}
	return r.ForProvider(ctx, p, 0)
}

// ForProvider builds a Scope for p. depth guards against chained-provider loops.
func (r *Resolver) ForProvider(ctx context.Context, p *awsv1alpha1.AWSProvider, depth int) (*Scope, error) {
	if depth > 4 {
		return nil, fmt.Errorf("AWSProvider %q: sourceProviderRef chain too deep", p.Name)
	}
	region := p.Spec.Region
	if region == "" {
		region = r.BaseRegion
	}
	creds := r.BaseCredentials
	if p.Spec.RoleARN == "" {
		if p.Spec.SourceProviderRef != nil {
			src, err := r.ForName(ctx, p.Spec.SourceProviderRef.Name, "")
			if err != nil {
				return nil, err
			}
			creds = src.Credentials
		}
		return &Scope{Name: p.Name, Region: region, Credentials: creds, AccountID: p.Status.AccountID}, nil
	}

	r.mu.Lock()
	if r.cache == nil {
		r.cache = map[string]cacheEntry{}
	}
	if e, ok := r.cache[p.Name]; ok && e.generation == p.Generation {
		r.mu.Unlock()
		return &Scope{Name: p.Name, Region: region, Credentials: e.creds, AccountID: e.accountID}, nil
	}
	r.mu.Unlock()

	stsClient := r.STS
	if p.Spec.SourceProviderRef != nil {
		src, err := r.ForName(ctx, p.Spec.SourceProviderRef.Name, "")
		if err != nil {
			return nil, err
		}
		stsClient = sts.New(sts.Options{Region: region, Credentials: src.Credentials})
	}
	if stsClient == nil {
		return nil, fmt.Errorf("AWSProvider %q: no STS client configured", p.Name)
	}
	assumer, ok := stsClient.(stscreds.AssumeRoleAPIClient)
	if !ok {
		return nil, fmt.Errorf("AWSProvider %q: STS client cannot assume roles", p.Name)
	}
	ap := stscreds.NewAssumeRoleProvider(assumer, p.Spec.RoleARN, func(o *stscreds.AssumeRoleOptions) {
		o.RoleSessionName = p.Spec.SessionName
		if o.RoleSessionName == "" {
			o.RoleSessionName = "konfig-konector"
		}
		if p.Spec.ExternalID != "" {
			o.ExternalID = aws.String(p.Spec.ExternalID)
		}
		if p.Spec.DurationSeconds > 0 {
			o.Duration = time.Duration(p.Spec.DurationSeconds) * time.Second
		}
	})
	cached := aws.NewCredentialsCache(ap, func(o *aws.CredentialsCacheOptions) {
		o.ExpiryWindow = 2 * time.Minute
	})
	accountID := accountFromRoleARN(p.Spec.RoleARN)

	r.mu.Lock()
	r.cache[p.Name] = cacheEntry{generation: p.Generation, creds: cached, accountID: accountID}
	r.mu.Unlock()
	return &Scope{Name: p.Name, Region: region, Credentials: cached, AccountID: accountID}, nil
}

// Invalidate drops cached credentials for a provider (called when it changes
// or is deleted).
func (r *Resolver) Invalidate(name string) {
	r.mu.Lock()
	delete(r.cache, name)
	r.mu.Unlock()
}

func namespaceAllowed(allowed []string, ns string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, a := range allowed {
		if a == ns || a == "*" {
			return true
		}
		if strings.HasSuffix(a, "*") && strings.HasPrefix(ns, strings.TrimSuffix(a, "*")) {
			return true
		}
	}
	return false
}

func accountFromRoleARN(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) >= 5 && len(parts[4]) == 12 {
		return parts[4]
	}
	return ""
}
