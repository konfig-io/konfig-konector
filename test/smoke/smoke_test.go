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

// Package smoke validates the Helm smoke-test manifest against the real CRD
// schemas using an envtest API server, so the smoke CRs and the CRDs cannot
// drift apart unnoticed. It renders the chart with smoketest.enabled=true and
// creates every document; it also asserts that the CEL immutability rules
// actually reject a mutation.
package smoke

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// chartPath is where the deployed (Argo) chart lives relative to this repo.
const chartPath = "../../../argo/konfig-konector"

func renderSmokeManifest(t *testing.T) []byte {
	t.Helper()
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm not on PATH; skipping smoke-manifest validation")
	}
	abs, err := filepath.Abs(chartPath)
	if err != nil {
		t.Fatalf("resolve chart path: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Skipf("argo chart not present at %s; skipping", abs)
	}
	out, err := exec.Command(helm, "template", abs,
		"--set", "smoketest.enabled=true",
		"-s", "templates/test.yaml").Output()
	if err != nil {
		t.Fatalf("helm template: %v", err)
	}
	return out
}

func TestSmokeManifestValidatesAgainstCRDs(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS not set; run via make test")
	}

	manifest := renderSmokeManifest(t)

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	defer func() { _ = testEnv.Stop() }()

	c, err := client.New(cfg, client.Options{})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "konfig-system"}}
	if err := c.Create(ctx, ns); err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	docs := strings.Split(string(manifest), "\n---")
	created := 0
	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" || strings.HasPrefix(doc, "#") && !strings.Contains(doc, "apiVersion") {
			continue
		}
		obj := &unstructured.Unstructured{}
		if err := yaml.Unmarshal([]byte(doc), obj); err != nil {
			t.Errorf("unmarshal document: %v\n%s", err, doc)
			continue
		}
		if obj.GetKind() == "" {
			continue
		}
		if err := c.Create(ctx, obj); err != nil {
			t.Errorf("API server rejected %s %s/%s: %v",
				obj.GetKind(), obj.GetNamespace(), obj.GetName(), err)
			continue
		}
		created++
	}
	if created == 0 {
		t.Fatal("no smoke resources were created — manifest empty or all rejected")
	}
	t.Logf("API server accepted %d smoke resources", created)

	// The CEL immutability rules must reject changing an immutable field.
	q := &unstructured.Unstructured{}
	q.SetAPIVersion("aws.konfig.io/v1alpha1")
	q.SetKind("SQSQueue")
	if err := c.Get(ctx, client.ObjectKey{Namespace: "konfig-system", Name: "kk-smoke-queue"}, q); err != nil {
		t.Fatalf("get smoke queue: %v", err)
	}
	if err := unstructured.SetNestedField(q.Object, "renamed-queue", "spec", "queueName"); err != nil {
		t.Fatalf("set field: %v", err)
	}
	err = c.Update(ctx, q)
	if err == nil {
		t.Error("expected CEL immutability rule to reject queueName change, but update succeeded")
	} else if !apierrors.IsInvalid(err) {
		t.Errorf("expected Invalid error from CEL rule, got: %v", err)
	}
}
