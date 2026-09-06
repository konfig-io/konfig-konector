/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in compliance with the License.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package webhook holds the admission webhook that validates spec.providerRef
// on every aws.konfig.io resource against the referenced AWSProvider's
// allowedNamespaces, so a bad reference is rejected at apply time instead of
// surfacing as a reconcile error.
package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// ProviderRefValidator rejects resources whose providerRef names a missing
// AWSProvider or one that does not allow the resource's namespace.
type ProviderRefValidator struct {
	Client client.Client
}

// Path is where the handler is served on the manager's webhook server.
const Path = "/validate-aws-konfig-io-providerref"

type providerRefOnly struct {
	Spec struct {
		ProviderRef *awsv1alpha1.ProviderRef `json:"providerRef,omitempty"`
	} `json:"spec"`
}

// Handle implements admission.Handler.
func (v *ProviderRefValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Kind.Group != awsv1alpha1.GroupVersion.Group || req.Kind.Kind == "AWSProvider" {
		return admission.Allowed("")
	}
	var obj providerRefOnly
	if err := json.Unmarshal(req.Object.Raw, &obj); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("decode object: %w", err))
	}
	ref := obj.Spec.ProviderRef
	if ref == nil || ref.Name == "" {
		return admission.Allowed("")
	}
	p := &awsv1alpha1.AWSProvider{}
	if err := v.Client.Get(ctx, k8stypes.NamespacedName{Name: ref.Name}, p); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return admission.Denied(fmt.Sprintf("spec.providerRef.name %q: no such AWSProvider", ref.Name))
		}
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if !NamespaceAllowed(p.Spec.AllowedNamespaces, req.Namespace) {
		return admission.Denied(fmt.Sprintf("AWSProvider %q does not allow namespace %q (allowedNamespaces: %s)",
			ref.Name, req.Namespace, strings.Join(p.Spec.AllowedNamespaces, ", ")))
	}
	return admission.Allowed("")
}

// NamespaceAllowed mirrors the resolver's rule: empty list allows all; exact
// names and trailing-* globs match.
func NamespaceAllowed(allowed []string, ns string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, a := range allowed {
		if a == ns || a == "*" || (strings.HasSuffix(a, "*") && strings.HasPrefix(ns, strings.TrimSuffix(a, "*"))) {
			return true
		}
	}
	return false
}
