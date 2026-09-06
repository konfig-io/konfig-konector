package webhook

import (
	"context"
	"encoding/json"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

func req(t *testing.T, kind, ns string, obj interface{}) admission.Request {
	raw, _ := json.Marshal(obj)
	return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Kind:      metav1.GroupVersionKind{Group: "aws.konfig.io", Version: "v1alpha1", Kind: kind},
		Namespace: ns, Object: runtime.RawExtension{Raw: raw},
	}}
}

func TestProviderRefValidator(t *testing.T) {
	s := runtime.NewScheme()
	_ = awsv1alpha1.AddToScheme(s)
	prod := &awsv1alpha1.AWSProvider{ObjectMeta: metav1.ObjectMeta{Name: "prod"}, Spec: awsv1alpha1.AWSProviderSpec{AllowedNamespaces: []string{"prod-*"}}}
	v := &ProviderRefValidator{Client: fake.NewClientBuilder().WithScheme(s).WithObjects(prod).Build()}
	q := func(ns, provider string) admission.Response {
		spec := map[string]interface{}{"queueName": "q"}
		if provider != "" {
			spec["providerRef"] = map[string]string{"name": provider}
		}
		return v.Handle(context.Background(), req(t, "SQSQueue", ns, map[string]interface{}{"spec": spec}))
	}
	if r := q("dev", ""); !r.Allowed {
		t.Fatalf("no providerRef must be allowed: %v", r.Result)
	}
	if r := q("prod-api", "prod"); !r.Allowed {
		t.Fatalf("allowed namespace must pass: %v", r.Result)
	}
	if r := q("dev", "prod"); r.Allowed {
		t.Fatal("namespace outside allowedNamespaces must be denied")
	}
	if r := q("prod-api", "nope"); r.Allowed {
		t.Fatal("unknown provider must be denied")
	}
}
