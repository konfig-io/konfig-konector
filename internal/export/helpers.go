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

package export

import (
	"regexp"
	"strings"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var invalidNameChars = regexp.MustCompile(`[^a-z0-9-]+`)
var dashRuns = regexp.MustCompile(`-+`)

// CRName converts an AWS name/identifier into a valid RFC-1123 Kubernetes
// object name. AWS names allow uppercase, slashes, dots, and underscores;
// k8s names do not.
func CRName(awsName string) string {
	n := strings.ToLower(awsName)
	n = invalidNameChars.ReplaceAllString(n, "-")
	n = dashRuns.ReplaceAllString(n, "-")
	n = strings.Trim(n, "-")
	if n == "" {
		n = "unnamed"
	}
	if len(n) > 253 {
		n = n[:253]
		n = strings.Trim(n, "-")
	}
	return n
}

// ObjectMeta builds the standard metadata for an exported CR: sanitised name,
// target namespace, and the abandon deletion policy unless disabled.
func ObjectMeta(awsName string, opts *Options) metav1.ObjectMeta {
	om := metav1.ObjectMeta{
		Name:      CRName(awsName),
		Namespace: opts.Namespace,
	}
	if opts.Abandon {
		om.Annotations = map[string]string{
			awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon,
		}
	}
	return om
}

// TagMap converts AWS tag slices (any type with Key/Value string pointers is
// converted by the per-service exporters) — this helper just filters AWS-
// internal tags that must not round-trip into specs.
func TagMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		// aws: prefixed tags are reserved/system-managed and rejected on write.
		if strings.HasPrefix(k, "aws:") {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
