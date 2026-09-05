package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeSTS struct{ calls int }

func (f *fakeSTS) AssumeRole(ctx context.Context, in *sts.AssumeRoleInput, _ ...func(*sts.Options)) (*sts.AssumeRoleOutput, error) {
	f.calls++
	return &sts.AssumeRoleOutput{Credentials: &ststypes.Credentials{
		AccessKeyId: aws.String("AKIA"), SecretAccessKey: aws.String("s"), SessionToken: aws.String("t"),
		Expiration: aws.Time(metav1.Now().Add(3600e9)),
	}}, nil
}
func (f *fakeSTS) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	return &sts.GetCallerIdentityOutput{Account: aws.String("111122223333")}, nil
}

type obj struct {
	ns  string
	ref *awsv1alpha1.ProviderRef
}

func (o obj) GetProviderRef() *awsv1alpha1.ProviderRef { return o.ref }
func (o obj) GetNamespace() string                     { return o.ns }

func newResolver(t *testing.T, objs ...client.Object) (*Resolver, *fakeSTS) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = awsv1alpha1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	f := &fakeSTS{}
	return &Resolver{Client: c, STS: f, BaseRegion: "us-east-1"}, f
}

func TestForObjectDefault(t *testing.T) {
	r, _ := newResolver(t)
	s, err := r.ForObject(context.Background(), obj{ns: "a"})
	if err != nil || s != nil {
		t.Fatalf("expected nil scope, got %v %v", s, err)
	}
}

func TestForObjectExplicitRefAssumesRole(t *testing.T) {
	p := &awsv1alpha1.AWSProvider{ObjectMeta: metav1.ObjectMeta{Name: "prod", Generation: 1},
		Spec: awsv1alpha1.AWSProviderSpec{RoleARN: "arn:aws:iam::111122223333:role/spoke", Region: "eu-west-1"}}
	r, f := newResolver(t, p)
	s, err := r.ForObject(context.Background(), obj{ns: "a", ref: &awsv1alpha1.ProviderRef{Name: "prod"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Region != "eu-west-1" || s.AccountID != "111122223333" || s.Credentials == nil {
		t.Fatalf("bad scope %+v", s)
	}
	if _, err := s.Credentials.Retrieve(context.Background()); err != nil {
		t.Fatal(err)
	}
	// cached: second resolve + retrieve must not call AssumeRole again
	s2, _ := r.ForObject(context.Background(), obj{ns: "a", ref: &awsv1alpha1.ProviderRef{Name: "prod", Region: "us-west-2"}})
	if _, err := s2.Credentials.Retrieve(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 {
		t.Fatalf("expected 1 AssumeRole call, got %d", f.calls)
	}
	if s2.Region != "us-west-2" {
		t.Fatalf("per-resource region override not applied: %s", s2.Region)
	}
}

func TestForObjectNamespaceAnnotation(t *testing.T) {
	p := &awsv1alpha1.AWSProvider{ObjectMeta: metav1.ObjectMeta{Name: "dev"}, Spec: awsv1alpha1.AWSProviderSpec{Region: "ap-south-1"}}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "team", Annotations: map[string]string{awsv1alpha1.ProviderAnnotation: "dev"}}}
	r, _ := newResolver(t, p, ns)
	s, err := r.ForObject(context.Background(), obj{ns: "team"})
	if err != nil || s == nil || s.Region != "ap-south-1" || s.Name != "dev" {
		t.Fatalf("got %+v %v", s, err)
	}
}

func TestAllowedNamespaces(t *testing.T) {
	p := &awsv1alpha1.AWSProvider{ObjectMeta: metav1.ObjectMeta{Name: "prod"},
		Spec: awsv1alpha1.AWSProviderSpec{AllowedNamespaces: []string{"prod-*"}}}
	r, _ := newResolver(t, p)
	if _, err := r.ForObject(context.Background(), obj{ns: "prod-api", ref: &awsv1alpha1.ProviderRef{Name: "prod"}}); err != nil {
		t.Fatal(err)
	}
	_, err := r.ForObject(context.Background(), obj{ns: "dev", ref: &awsv1alpha1.ProviderRef{Name: "prod"}})
	var np *ErrNotPermitted
	if !errors.As(err, &np) {
		t.Fatalf("expected ErrNotPermitted, got %v", err)
	}
}

func TestMissingProvider(t *testing.T) {
	r, _ := newResolver(t)
	if _, err := r.ForObject(context.Background(), obj{ns: "a", ref: &awsv1alpha1.ProviderRef{Name: "nope"}}); err == nil {
		t.Fatal("expected error")
	}
}
