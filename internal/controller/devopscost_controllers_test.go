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

package controller

// Create + delete + abandon coverage for the devopscost family kinds that do
// not have dedicated full test files (Budget and PrometheusWorkspace carry
// the family's full suites).

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsamp "github.com/aws/aws-sdk-go-v2/service/amp"
	awscodeartifact "github.com/aws/aws-sdk-go-v2/service/codeartifact"
	codeartifacttypes "github.com/aws/aws-sdk-go-v2/service/codeartifact/types"
	awsce "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	awsgrafana "github.com/aws/aws-sdk-go-v2/service/grafana"
	grafanatypes "github.com/aws/aws-sdk-go-v2/service/grafana/types"
	awsxray "github.com/aws/aws-sdk-go-v2/service/xray"
	xraytypes "github.com/aws/aws-sdk-go-v2/service/xray/types"
	"github.com/aws/smithy-go"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// devopsAPIErr is a reusable smithy API error with an arbitrary code.
type devopsAPIErr struct{ code string }

func (e devopsAPIErr) Error() string                 { return e.code }
func (e devopsAPIErr) ErrorCode() string             { return e.code }
func (e devopsAPIErr) ErrorMessage() string          { return e.code }
func (e devopsAPIErr) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }

func devopsFakeClient(t *testing.T, statusObj client.Object, objs ...client.Object) (*runtime.Scheme, client.Client) {
	t.Helper()
	scheme := newDevOpsCostScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(statusObj).
		WithObjects(objs...).
		Build()
	return scheme, c
}

func devopsAssertReadyTrue(t *testing.T, conditions []metav1.Condition) {
	t.Helper()
	cond := apimeta.FindStatusCondition(conditions, awsv1alpha1.ConditionReady)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Errorf("Ready condition = %+v, want True", cond)
	}
}

func devopsAssertGone(t *testing.T, ctx context.Context, c client.Client, key k8stypes.NamespacedName, obj client.Object) {
	t.Helper()
	if err := c.Get(ctx, key, obj); !apierrors.IsNotFound(err) {
		t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
	}
}

// ---------------------------------------------------------------- CodeArtifactDomain

type fakeCodeArtifactDomain struct {
	notFoundOnDescribe bool
	createCalled       bool
	deleteCalled       bool
	deletedDomain      string
}

func (f *fakeCodeArtifactDomain) DescribeDomain(_ context.Context, _ *awscodeartifact.DescribeDomainInput, _ ...func(*awscodeartifact.Options)) (*awscodeartifact.DescribeDomainOutput, error) {
	if f.notFoundOnDescribe {
		return nil, devopsAPIErr{"ResourceNotFoundException"}
	}
	return &awscodeartifact.DescribeDomainOutput{Domain: &codeartifacttypes.DomainDescription{
		Arn:   aws.String("arn:aws:codeartifact:us-east-1:123456789012:domain/corp"),
		Owner: aws.String("123456789012"),
	}}, nil
}

func (f *fakeCodeArtifactDomain) CreateDomain(_ context.Context, params *awscodeartifact.CreateDomainInput, _ ...func(*awscodeartifact.Options)) (*awscodeartifact.CreateDomainOutput, error) {
	f.createCalled = true
	return &awscodeartifact.CreateDomainOutput{Domain: &codeartifacttypes.DomainDescription{
		Arn:   aws.String("arn:aws:codeartifact:us-east-1:123456789012:domain/" + aws.ToString(params.Domain)),
		Owner: aws.String("123456789012"),
	}}, nil
}

func (f *fakeCodeArtifactDomain) DeleteDomain(_ context.Context, params *awscodeartifact.DeleteDomainInput, _ ...func(*awscodeartifact.Options)) (*awscodeartifact.DeleteDomainOutput, error) {
	f.deleteCalled = true
	f.deletedDomain = aws.ToString(params.Domain)
	return &awscodeartifact.DeleteDomainOutput{}, nil
}

func codeArtifactDomainCR(mutate ...func(*awsv1alpha1.CodeArtifactDomain)) *awsv1alpha1.CodeArtifactDomain {
	d := &awsv1alpha1.CodeArtifactDomain{
		ObjectMeta: metav1.ObjectMeta{Name: "corp", Namespace: "default"},
		Spec:       awsv1alpha1.CodeArtifactDomainSpec{DomainName: "corp"},
	}
	for _, m := range mutate {
		m(d)
	}
	return d
}

func TestCodeArtifactDomainReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "corp", Namespace: "default"}}

	t.Run("create persists ARN and Ready", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeCodeArtifactDomain{notFoundOnDescribe: true}
		scheme, c := devopsFakeClient(t, &awsv1alpha1.CodeArtifactDomain{}, codeArtifactDomainCR())
		r := &CodeArtifactDomainReconciler{Client: c, Scheme: scheme, CodeArtifactClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Error("expected CreateDomain to be called")
		}
		got := &awsv1alpha1.CodeArtifactDomain{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ARN == "" {
			t.Error("expected status.arn to be set")
		}
		devopsAssertReadyTrue(t, got.Status.Conditions)
	})

	t.Run("delete calls AWS delete with domain name", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeCodeArtifactDomain{}
		obj := codeArtifactDomainCR(func(d *awsv1alpha1.CodeArtifactDomain) {
			d.Finalizers = []string{awsv1alpha1.FinalizerName}
		})
		scheme, c := devopsFakeClient(t, &awsv1alpha1.CodeArtifactDomain{}, obj)
		if err := c.Delete(ctx, codeArtifactDomainCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &CodeArtifactDomainReconciler{Client: c, Scheme: scheme, CodeArtifactClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedDomain != "corp" {
			t.Errorf("DeleteDomain called=%v domain=%q, want corp", f.deleteCalled, f.deletedDomain)
		}
		devopsAssertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.CodeArtifactDomain{})
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeCodeArtifactDomain{}
		obj := codeArtifactDomainCR(func(d *awsv1alpha1.CodeArtifactDomain) {
			d.Finalizers = []string{awsv1alpha1.FinalizerName}
			d.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		})
		scheme, c := devopsFakeClient(t, &awsv1alpha1.CodeArtifactDomain{}, obj)
		if err := c.Delete(ctx, codeArtifactDomainCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &CodeArtifactDomainReconciler{Client: c, Scheme: scheme, CodeArtifactClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteDomain must not be called when abandoning")
		}
		devopsAssertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.CodeArtifactDomain{})
	})
}

// ---------------------------------------------------------------- CodeArtifactRepository

type fakeCodeArtifactRepo struct {
	notFoundOnDescribe bool
	createCalled       bool
	associateCalled    bool
	deleteCalled       bool
	deletedRepo        string
	deletedDomain      string
}

func (f *fakeCodeArtifactRepo) DescribeRepository(_ context.Context, _ *awscodeartifact.DescribeRepositoryInput, _ ...func(*awscodeartifact.Options)) (*awscodeartifact.DescribeRepositoryOutput, error) {
	if f.notFoundOnDescribe {
		return nil, devopsAPIErr{"ResourceNotFoundException"}
	}
	return &awscodeartifact.DescribeRepositoryOutput{Repository: &codeartifacttypes.RepositoryDescription{
		Arn: aws.String("arn:aws:codeartifact:us-east-1:123456789012:repository/corp/npm"),
	}}, nil
}

func (f *fakeCodeArtifactRepo) CreateRepository(_ context.Context, params *awscodeartifact.CreateRepositoryInput, _ ...func(*awscodeartifact.Options)) (*awscodeartifact.CreateRepositoryOutput, error) {
	f.createCalled = true
	return &awscodeartifact.CreateRepositoryOutput{Repository: &codeartifacttypes.RepositoryDescription{
		Arn: aws.String("arn:aws:codeartifact:us-east-1:123456789012:repository/corp/" + aws.ToString(params.Repository)),
	}}, nil
}

func (f *fakeCodeArtifactRepo) UpdateRepository(_ context.Context, _ *awscodeartifact.UpdateRepositoryInput, _ ...func(*awscodeartifact.Options)) (*awscodeartifact.UpdateRepositoryOutput, error) {
	return &awscodeartifact.UpdateRepositoryOutput{}, nil
}

func (f *fakeCodeArtifactRepo) DeleteRepository(_ context.Context, params *awscodeartifact.DeleteRepositoryInput, _ ...func(*awscodeartifact.Options)) (*awscodeartifact.DeleteRepositoryOutput, error) {
	f.deleteCalled = true
	f.deletedRepo = aws.ToString(params.Repository)
	f.deletedDomain = aws.ToString(params.Domain)
	return &awscodeartifact.DeleteRepositoryOutput{}, nil
}

func (f *fakeCodeArtifactRepo) AssociateExternalConnection(_ context.Context, _ *awscodeartifact.AssociateExternalConnectionInput, _ ...func(*awscodeartifact.Options)) (*awscodeartifact.AssociateExternalConnectionOutput, error) {
	f.associateCalled = true
	return &awscodeartifact.AssociateExternalConnectionOutput{}, nil
}

func (f *fakeCodeArtifactRepo) DisassociateExternalConnection(_ context.Context, _ *awscodeartifact.DisassociateExternalConnectionInput, _ ...func(*awscodeartifact.Options)) (*awscodeartifact.DisassociateExternalConnectionOutput, error) {
	return &awscodeartifact.DisassociateExternalConnectionOutput{}, nil
}

func codeArtifactRepoCR(mutate ...func(*awsv1alpha1.CodeArtifactRepository)) *awsv1alpha1.CodeArtifactRepository {
	repo := &awsv1alpha1.CodeArtifactRepository{
		ObjectMeta: metav1.ObjectMeta{Name: "npm", Namespace: "default"},
		Spec: awsv1alpha1.CodeArtifactRepositorySpec{
			RepositoryName:      "npm",
			DomainRef:           awsv1alpha1.CodeArtifactDomainRef{DomainName: "corp"},
			ExternalConnections: []string{"public:npmjs"},
		},
	}
	for _, m := range mutate {
		m(repo)
	}
	return repo
}

func TestCodeArtifactRepositoryReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "npm", Namespace: "default"}}

	t.Run("create persists ARN, associates external connection", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeCodeArtifactRepo{notFoundOnDescribe: true}
		scheme, c := devopsFakeClient(t, &awsv1alpha1.CodeArtifactRepository{}, codeArtifactRepoCR())
		r := &CodeArtifactRepositoryReconciler{Client: c, Scheme: scheme, CodeArtifactClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Error("expected CreateRepository to be called")
		}
		if !f.associateCalled {
			t.Error("expected AssociateExternalConnection to be called")
		}
		got := &awsv1alpha1.CodeArtifactRepository{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ARN == "" {
			t.Error("expected status.arn to be set")
		}
		if got.Status.DomainName != "corp" {
			t.Errorf("status.domainName = %q, want corp", got.Status.DomainName)
		}
		devopsAssertReadyTrue(t, got.Status.Conditions)
	})

	t.Run("dependency not ready requeues without error", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeCodeArtifactRepo{}
		obj := codeArtifactRepoCR(func(repo *awsv1alpha1.CodeArtifactRepository) {
			repo.Spec.DomainRef = awsv1alpha1.CodeArtifactDomainRef{Name: "corp"}
		})
		domain := codeArtifactDomainCR() // no ARN in status yet
		scheme, c := devopsFakeClient(t, &awsv1alpha1.CodeArtifactRepository{}, obj, domain)
		r := &CodeArtifactRepositoryReconciler{Client: c, Scheme: scheme, CodeArtifactClient: f}
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res.RequeueAfter != requeueDependency.RequeueAfter {
			t.Errorf("RequeueAfter = %v, want dependency requeue %v", res.RequeueAfter, requeueDependency.RequeueAfter)
		}
		if f.createCalled {
			t.Error("CreateRepository must not be called while the domain is not ready")
		}
	})

	t.Run("delete calls AWS delete with domain and repo", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeCodeArtifactRepo{}
		obj := codeArtifactRepoCR(func(repo *awsv1alpha1.CodeArtifactRepository) {
			repo.Finalizers = []string{awsv1alpha1.FinalizerName}
			repo.Status.DomainName = "corp"
		})
		scheme, c := devopsFakeClient(t, &awsv1alpha1.CodeArtifactRepository{}, obj)
		if err := c.Delete(ctx, codeArtifactRepoCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &CodeArtifactRepositoryReconciler{Client: c, Scheme: scheme, CodeArtifactClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedRepo != "npm" || f.deletedDomain != "corp" {
			t.Errorf("DeleteRepository called=%v repo=%q domain=%q", f.deleteCalled, f.deletedRepo, f.deletedDomain)
		}
		devopsAssertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.CodeArtifactRepository{})
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeCodeArtifactRepo{}
		obj := codeArtifactRepoCR(func(repo *awsv1alpha1.CodeArtifactRepository) {
			repo.Finalizers = []string{awsv1alpha1.FinalizerName}
			repo.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		})
		scheme, c := devopsFakeClient(t, &awsv1alpha1.CodeArtifactRepository{}, obj)
		if err := c.Delete(ctx, codeArtifactRepoCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &CodeArtifactRepositoryReconciler{Client: c, Scheme: scheme, CodeArtifactClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteRepository must not be called when abandoning")
		}
		devopsAssertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.CodeArtifactRepository{})
	})
}

// ---------------------------------------------------------------- XRayGroup

type fakeXRayGroup struct {
	notFoundOnGet bool
	createCalled  bool
	deleteCalled  bool
	deleteInput   *awsxray.DeleteGroupInput
}

const testXRayGroupARN = "arn:aws:xray:us-east-1:123456789012:group/api-traces/ABCDEF"

func (f *fakeXRayGroup) GetGroup(_ context.Context, _ *awsxray.GetGroupInput, _ ...func(*awsxray.Options)) (*awsxray.GetGroupOutput, error) {
	if f.notFoundOnGet {
		return nil, devopsAPIErr{"InvalidRequestException"}
	}
	return &awsxray.GetGroupOutput{Group: &xraytypes.Group{GroupARN: aws.String(testXRayGroupARN)}}, nil
}

func (f *fakeXRayGroup) CreateGroup(_ context.Context, _ *awsxray.CreateGroupInput, _ ...func(*awsxray.Options)) (*awsxray.CreateGroupOutput, error) {
	f.createCalled = true
	return &awsxray.CreateGroupOutput{Group: &xraytypes.Group{GroupARN: aws.String(testXRayGroupARN)}}, nil
}

func (f *fakeXRayGroup) UpdateGroup(_ context.Context, _ *awsxray.UpdateGroupInput, _ ...func(*awsxray.Options)) (*awsxray.UpdateGroupOutput, error) {
	return &awsxray.UpdateGroupOutput{}, nil
}

func (f *fakeXRayGroup) DeleteGroup(_ context.Context, params *awsxray.DeleteGroupInput, _ ...func(*awsxray.Options)) (*awsxray.DeleteGroupOutput, error) {
	f.deleteCalled = true
	f.deleteInput = params
	return &awsxray.DeleteGroupOutput{}, nil
}

func xrayGroupCR(mutate ...func(*awsv1alpha1.XRayGroup)) *awsv1alpha1.XRayGroup {
	g := &awsv1alpha1.XRayGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "api-traces", Namespace: "default"},
		Spec: awsv1alpha1.XRayGroupSpec{
			GroupName:        "api-traces",
			FilterExpression: `service("api")`,
		},
	}
	for _, m := range mutate {
		m(g)
	}
	return g
}

func TestXRayGroupReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "api-traces", Namespace: "default"}}

	t.Run("create persists ARN and Ready", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeXRayGroup{notFoundOnGet: true}
		scheme, c := devopsFakeClient(t, &awsv1alpha1.XRayGroup{}, xrayGroupCR())
		r := &XRayGroupReconciler{Client: c, Scheme: scheme, XRayClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Error("expected CreateGroup to be called")
		}
		got := &awsv1alpha1.XRayGroup{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ARN != testXRayGroupARN {
			t.Errorf("status.arn = %q, want %q", got.Status.ARN, testXRayGroupARN)
		}
		devopsAssertReadyTrue(t, got.Status.Conditions)
	})

	t.Run("delete prefers status ARN", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeXRayGroup{}
		obj := xrayGroupCR(func(g *awsv1alpha1.XRayGroup) {
			g.Finalizers = []string{awsv1alpha1.FinalizerName}
			g.Status.ARN = testXRayGroupARN
		})
		scheme, c := devopsFakeClient(t, &awsv1alpha1.XRayGroup{}, obj)
		if err := c.Delete(ctx, xrayGroupCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &XRayGroupReconciler{Client: c, Scheme: scheme, XRayClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || aws.ToString(f.deleteInput.GroupARN) != testXRayGroupARN {
			t.Errorf("DeleteGroup called=%v input=%+v", f.deleteCalled, f.deleteInput)
		}
		devopsAssertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.XRayGroup{})
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeXRayGroup{}
		obj := xrayGroupCR(func(g *awsv1alpha1.XRayGroup) {
			g.Finalizers = []string{awsv1alpha1.FinalizerName}
			g.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		})
		scheme, c := devopsFakeClient(t, &awsv1alpha1.XRayGroup{}, obj)
		if err := c.Delete(ctx, xrayGroupCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &XRayGroupReconciler{Client: c, Scheme: scheme, XRayClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteGroup must not be called when abandoning")
		}
		devopsAssertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.XRayGroup{})
	})
}

// ---------------------------------------------------------------- XRaySamplingRule

type fakeXRaySamplingRule struct {
	rules        []xraytypes.SamplingRuleRecord
	createCalled bool
	createInput  *awsxray.CreateSamplingRuleInput
	deleteCalled bool
	deleteInput  *awsxray.DeleteSamplingRuleInput
}

const testXRayRuleARN = "arn:aws:xray:us-east-1:123456789012:sampling-rule/api-sampling"

func (f *fakeXRaySamplingRule) GetSamplingRules(_ context.Context, _ *awsxray.GetSamplingRulesInput, _ ...func(*awsxray.Options)) (*awsxray.GetSamplingRulesOutput, error) {
	return &awsxray.GetSamplingRulesOutput{SamplingRuleRecords: f.rules}, nil
}

func (f *fakeXRaySamplingRule) CreateSamplingRule(_ context.Context, params *awsxray.CreateSamplingRuleInput, _ ...func(*awsxray.Options)) (*awsxray.CreateSamplingRuleOutput, error) {
	f.createCalled = true
	f.createInput = params
	return &awsxray.CreateSamplingRuleOutput{SamplingRuleRecord: &xraytypes.SamplingRuleRecord{
		SamplingRule: &xraytypes.SamplingRule{
			RuleName: params.SamplingRule.RuleName,
			RuleARN:  aws.String(testXRayRuleARN),
		},
	}}, nil
}

func (f *fakeXRaySamplingRule) UpdateSamplingRule(_ context.Context, _ *awsxray.UpdateSamplingRuleInput, _ ...func(*awsxray.Options)) (*awsxray.UpdateSamplingRuleOutput, error) {
	return &awsxray.UpdateSamplingRuleOutput{}, nil
}

func (f *fakeXRaySamplingRule) DeleteSamplingRule(_ context.Context, params *awsxray.DeleteSamplingRuleInput, _ ...func(*awsxray.Options)) (*awsxray.DeleteSamplingRuleOutput, error) {
	f.deleteCalled = true
	f.deleteInput = params
	return &awsxray.DeleteSamplingRuleOutput{}, nil
}

func xraySamplingRuleCR(mutate ...func(*awsv1alpha1.XRaySamplingRule)) *awsv1alpha1.XRaySamplingRule {
	rule := &awsv1alpha1.XRaySamplingRule{
		ObjectMeta: metav1.ObjectMeta{Name: "api-sampling", Namespace: "default"},
		Spec: awsv1alpha1.XRaySamplingRuleSpec{
			RuleName:      "api-sampling",
			Priority:      100,
			FixedRate:     0.05,
			ReservoirSize: 1,
			ServiceName:   "api",
		},
	}
	for _, m := range mutate {
		m(rule)
	}
	return rule
}

func TestXRaySamplingRuleReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "api-sampling", Namespace: "default"}}

	t.Run("create defaults matchers to star and persists ARN", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeXRaySamplingRule{}
		scheme, c := devopsFakeClient(t, &awsv1alpha1.XRaySamplingRule{}, xraySamplingRuleCR())
		r := &XRaySamplingRuleReconciler{Client: c, Scheme: scheme, XRayClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateSamplingRule to be called")
		}
		sr := f.createInput.SamplingRule
		if aws.ToString(sr.ServiceName) != "api" {
			t.Errorf("serviceName = %q, want api", aws.ToString(sr.ServiceName))
		}
		if aws.ToString(sr.Host) != "*" || aws.ToString(sr.URLPath) != "*" {
			t.Error("expected unset matchers to default to *")
		}
		if aws.ToInt32(sr.Version) != 1 {
			t.Errorf("version = %d, want 1", aws.ToInt32(sr.Version))
		}
		got := &awsv1alpha1.XRaySamplingRule{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ARN != testXRayRuleARN {
			t.Errorf("status.arn = %q, want %q", got.Status.ARN, testXRayRuleARN)
		}
		devopsAssertReadyTrue(t, got.Status.Conditions)
	})

	t.Run("delete prefers status ARN", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeXRaySamplingRule{}
		obj := xraySamplingRuleCR(func(rule *awsv1alpha1.XRaySamplingRule) {
			rule.Finalizers = []string{awsv1alpha1.FinalizerName}
			rule.Status.ARN = testXRayRuleARN
		})
		scheme, c := devopsFakeClient(t, &awsv1alpha1.XRaySamplingRule{}, obj)
		if err := c.Delete(ctx, xraySamplingRuleCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &XRaySamplingRuleReconciler{Client: c, Scheme: scheme, XRayClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || aws.ToString(f.deleteInput.RuleARN) != testXRayRuleARN {
			t.Errorf("DeleteSamplingRule called=%v input=%+v", f.deleteCalled, f.deleteInput)
		}
		devopsAssertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.XRaySamplingRule{})
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeXRaySamplingRule{}
		obj := xraySamplingRuleCR(func(rule *awsv1alpha1.XRaySamplingRule) {
			rule.Finalizers = []string{awsv1alpha1.FinalizerName}
			rule.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		})
		scheme, c := devopsFakeClient(t, &awsv1alpha1.XRaySamplingRule{}, obj)
		if err := c.Delete(ctx, xraySamplingRuleCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &XRaySamplingRuleReconciler{Client: c, Scheme: scheme, XRayClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteSamplingRule must not be called when abandoning")
		}
		devopsAssertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.XRaySamplingRule{})
	})
}

// ---------------------------------------------------------------- PrometheusRuleGroupsNamespace

type fakeAMPRuleNS struct {
	notFoundOnDescribe bool
	createCalled       bool
	deleteCalled       bool
	deletedWorkspace   string
	deletedName        string
}

const testRuleNSARN = "arn:aws:aps:us-east-1:123456789012:rulegroupsnamespace/" + testAMPWorkspaceID + "/alerts"

func (f *fakeAMPRuleNS) DescribeRuleGroupsNamespace(_ context.Context, _ *awsamp.DescribeRuleGroupsNamespaceInput, _ ...func(*awsamp.Options)) (*awsamp.DescribeRuleGroupsNamespaceOutput, error) {
	if f.notFoundOnDescribe {
		return nil, devopsAPIErr{"ResourceNotFoundException"}
	}
	return &awsamp.DescribeRuleGroupsNamespaceOutput{}, nil
}

func (f *fakeAMPRuleNS) CreateRuleGroupsNamespace(_ context.Context, _ *awsamp.CreateRuleGroupsNamespaceInput, _ ...func(*awsamp.Options)) (*awsamp.CreateRuleGroupsNamespaceOutput, error) {
	f.createCalled = true
	return &awsamp.CreateRuleGroupsNamespaceOutput{Arn: aws.String(testRuleNSARN), Name: aws.String("alerts")}, nil
}

func (f *fakeAMPRuleNS) PutRuleGroupsNamespace(_ context.Context, _ *awsamp.PutRuleGroupsNamespaceInput, _ ...func(*awsamp.Options)) (*awsamp.PutRuleGroupsNamespaceOutput, error) {
	return &awsamp.PutRuleGroupsNamespaceOutput{}, nil
}

func (f *fakeAMPRuleNS) DeleteRuleGroupsNamespace(_ context.Context, params *awsamp.DeleteRuleGroupsNamespaceInput, _ ...func(*awsamp.Options)) (*awsamp.DeleteRuleGroupsNamespaceOutput, error) {
	f.deleteCalled = true
	f.deletedWorkspace = aws.ToString(params.WorkspaceId)
	f.deletedName = aws.ToString(params.Name)
	return &awsamp.DeleteRuleGroupsNamespaceOutput{}, nil
}

func ruleNSCR(mutate ...func(*awsv1alpha1.PrometheusRuleGroupsNamespace)) *awsv1alpha1.PrometheusRuleGroupsNamespace {
	ns := &awsv1alpha1.PrometheusRuleGroupsNamespace{
		ObjectMeta: metav1.ObjectMeta{Name: "alerts", Namespace: "default"},
		Spec: awsv1alpha1.PrometheusRuleGroupsNamespaceSpec{
			WorkspaceRef: awsv1alpha1.PrometheusWorkspaceRef{WorkspaceID: testAMPWorkspaceID},
			Name:         "alerts",
			Data:         "groups: []",
		},
	}
	for _, m := range mutate {
		m(ns)
	}
	return ns
}

func TestPrometheusRuleGroupsNamespaceReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "alerts", Namespace: "default"}}

	t.Run("create persists ARN and workspace ID", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeAMPRuleNS{notFoundOnDescribe: true}
		scheme, c := devopsFakeClient(t, &awsv1alpha1.PrometheusRuleGroupsNamespace{}, ruleNSCR())
		r := &PrometheusRuleGroupsNamespaceReconciler{Client: c, Scheme: scheme, AMPClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Error("expected CreateRuleGroupsNamespace to be called")
		}
		got := &awsv1alpha1.PrometheusRuleGroupsNamespace{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ARN != testRuleNSARN {
			t.Errorf("status.arn = %q, want %q", got.Status.ARN, testRuleNSARN)
		}
		if got.Status.WorkspaceID != testAMPWorkspaceID {
			t.Errorf("status.workspaceId = %q, want %q", got.Status.WorkspaceID, testAMPWorkspaceID)
		}
		devopsAssertReadyTrue(t, got.Status.Conditions)
	})

	t.Run("delete calls AWS delete with workspace and name", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeAMPRuleNS{}
		obj := ruleNSCR(func(ns *awsv1alpha1.PrometheusRuleGroupsNamespace) {
			ns.Finalizers = []string{awsv1alpha1.FinalizerName}
			ns.Status.WorkspaceID = testAMPWorkspaceID
		})
		scheme, c := devopsFakeClient(t, &awsv1alpha1.PrometheusRuleGroupsNamespace{}, obj)
		if err := c.Delete(ctx, ruleNSCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &PrometheusRuleGroupsNamespaceReconciler{Client: c, Scheme: scheme, AMPClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedWorkspace != testAMPWorkspaceID || f.deletedName != "alerts" {
			t.Errorf("DeleteRuleGroupsNamespace called=%v ws=%q name=%q", f.deleteCalled, f.deletedWorkspace, f.deletedName)
		}
		devopsAssertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.PrometheusRuleGroupsNamespace{})
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeAMPRuleNS{}
		obj := ruleNSCR(func(ns *awsv1alpha1.PrometheusRuleGroupsNamespace) {
			ns.Finalizers = []string{awsv1alpha1.FinalizerName}
			ns.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		})
		scheme, c := devopsFakeClient(t, &awsv1alpha1.PrometheusRuleGroupsNamespace{}, obj)
		if err := c.Delete(ctx, ruleNSCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &PrometheusRuleGroupsNamespaceReconciler{Client: c, Scheme: scheme, AMPClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteRuleGroupsNamespace must not be called when abandoning")
		}
		devopsAssertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.PrometheusRuleGroupsNamespace{})
	})
}

// ---------------------------------------------------------------- PrometheusAlertManagerDefinition

type fakeAMPAlertManager struct {
	notFoundOnDescribe bool
	createCalled       bool
	deleteCalled       bool
	deletedWorkspace   string
}

func (f *fakeAMPAlertManager) DescribeAlertManagerDefinition(_ context.Context, _ *awsamp.DescribeAlertManagerDefinitionInput, _ ...func(*awsamp.Options)) (*awsamp.DescribeAlertManagerDefinitionOutput, error) {
	if f.notFoundOnDescribe {
		return nil, devopsAPIErr{"ResourceNotFoundException"}
	}
	return &awsamp.DescribeAlertManagerDefinitionOutput{}, nil
}

func (f *fakeAMPAlertManager) CreateAlertManagerDefinition(_ context.Context, _ *awsamp.CreateAlertManagerDefinitionInput, _ ...func(*awsamp.Options)) (*awsamp.CreateAlertManagerDefinitionOutput, error) {
	f.createCalled = true
	return &awsamp.CreateAlertManagerDefinitionOutput{}, nil
}

func (f *fakeAMPAlertManager) PutAlertManagerDefinition(_ context.Context, _ *awsamp.PutAlertManagerDefinitionInput, _ ...func(*awsamp.Options)) (*awsamp.PutAlertManagerDefinitionOutput, error) {
	return &awsamp.PutAlertManagerDefinitionOutput{}, nil
}

func (f *fakeAMPAlertManager) DeleteAlertManagerDefinition(_ context.Context, params *awsamp.DeleteAlertManagerDefinitionInput, _ ...func(*awsamp.Options)) (*awsamp.DeleteAlertManagerDefinitionOutput, error) {
	f.deleteCalled = true
	f.deletedWorkspace = aws.ToString(params.WorkspaceId)
	return &awsamp.DeleteAlertManagerDefinitionOutput{}, nil
}

func alertManagerCR(mutate ...func(*awsv1alpha1.PrometheusAlertManagerDefinition)) *awsv1alpha1.PrometheusAlertManagerDefinition {
	def := &awsv1alpha1.PrometheusAlertManagerDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "alertmanager", Namespace: "default"},
		Spec: awsv1alpha1.PrometheusAlertManagerDefinitionSpec{
			WorkspaceRef: awsv1alpha1.PrometheusWorkspaceRef{WorkspaceID: testAMPWorkspaceID},
			Definition:   "alertmanager_config: |\n  route:\n    receiver: default",
		},
	}
	for _, m := range mutate {
		m(def)
	}
	return def
}

func TestPrometheusAlertManagerDefinitionReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "alertmanager", Namespace: "default"}}

	t.Run("create persists workspace ID and Ready", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeAMPAlertManager{notFoundOnDescribe: true}
		scheme, c := devopsFakeClient(t, &awsv1alpha1.PrometheusAlertManagerDefinition{}, alertManagerCR())
		r := &PrometheusAlertManagerDefinitionReconciler{Client: c, Scheme: scheme, AMPClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Error("expected CreateAlertManagerDefinition to be called")
		}
		got := &awsv1alpha1.PrometheusAlertManagerDefinition{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.WorkspaceID != testAMPWorkspaceID {
			t.Errorf("status.workspaceId = %q, want %q", got.Status.WorkspaceID, testAMPWorkspaceID)
		}
		devopsAssertReadyTrue(t, got.Status.Conditions)
	})

	t.Run("delete calls AWS delete with workspace ID", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeAMPAlertManager{}
		obj := alertManagerCR(func(def *awsv1alpha1.PrometheusAlertManagerDefinition) {
			def.Finalizers = []string{awsv1alpha1.FinalizerName}
			def.Status.WorkspaceID = testAMPWorkspaceID
		})
		scheme, c := devopsFakeClient(t, &awsv1alpha1.PrometheusAlertManagerDefinition{}, obj)
		if err := c.Delete(ctx, alertManagerCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &PrometheusAlertManagerDefinitionReconciler{Client: c, Scheme: scheme, AMPClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedWorkspace != testAMPWorkspaceID {
			t.Errorf("DeleteAlertManagerDefinition called=%v ws=%q", f.deleteCalled, f.deletedWorkspace)
		}
		devopsAssertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.PrometheusAlertManagerDefinition{})
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeAMPAlertManager{}
		obj := alertManagerCR(func(def *awsv1alpha1.PrometheusAlertManagerDefinition) {
			def.Finalizers = []string{awsv1alpha1.FinalizerName}
			def.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		})
		scheme, c := devopsFakeClient(t, &awsv1alpha1.PrometheusAlertManagerDefinition{}, obj)
		if err := c.Delete(ctx, alertManagerCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &PrometheusAlertManagerDefinitionReconciler{Client: c, Scheme: scheme, AMPClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteAlertManagerDefinition must not be called when abandoning")
		}
		devopsAssertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.PrometheusAlertManagerDefinition{})
	})
}

// ---------------------------------------------------------------- GrafanaWorkspace

type fakeGrafana struct {
	createCalled bool
	deleteCalled bool
	deletedID    string
}

const testGrafanaWorkspaceID = "g-abcdef1234"

func (f *fakeGrafana) DescribeWorkspace(_ context.Context, _ *awsgrafana.DescribeWorkspaceInput, _ ...func(*awsgrafana.Options)) (*awsgrafana.DescribeWorkspaceOutput, error) {
	return &awsgrafana.DescribeWorkspaceOutput{Workspace: &grafanatypes.WorkspaceDescription{
		Id:       aws.String(testGrafanaWorkspaceID),
		Status:   grafanatypes.WorkspaceStatusActive,
		Endpoint: aws.String("g-abcdef1234.grafana-workspace.us-east-1.amazonaws.com"),
	}}, nil
}

func (f *fakeGrafana) CreateWorkspace(_ context.Context, params *awsgrafana.CreateWorkspaceInput, _ ...func(*awsgrafana.Options)) (*awsgrafana.CreateWorkspaceOutput, error) {
	f.createCalled = true
	if params.AccountAccessType != grafanatypes.AccountAccessTypeCurrentAccount {
		return nil, fmt.Errorf("unexpected accountAccessType %q", params.AccountAccessType)
	}
	return &awsgrafana.CreateWorkspaceOutput{Workspace: &grafanatypes.WorkspaceDescription{
		Id:     aws.String(testGrafanaWorkspaceID),
		Status: grafanatypes.WorkspaceStatusCreating,
	}}, nil
}

func (f *fakeGrafana) UpdateWorkspace(_ context.Context, _ *awsgrafana.UpdateWorkspaceInput, _ ...func(*awsgrafana.Options)) (*awsgrafana.UpdateWorkspaceOutput, error) {
	return &awsgrafana.UpdateWorkspaceOutput{}, nil
}

func (f *fakeGrafana) DeleteWorkspace(_ context.Context, params *awsgrafana.DeleteWorkspaceInput, _ ...func(*awsgrafana.Options)) (*awsgrafana.DeleteWorkspaceOutput, error) {
	f.deleteCalled = true
	f.deletedID = aws.ToString(params.WorkspaceId)
	return &awsgrafana.DeleteWorkspaceOutput{}, nil
}

func grafanaWorkspaceCR(mutate ...func(*awsv1alpha1.GrafanaWorkspace)) *awsv1alpha1.GrafanaWorkspace {
	ws := &awsv1alpha1.GrafanaWorkspace{
		ObjectMeta: metav1.ObjectMeta{Name: "dashboards", Namespace: "default"},
		Spec: awsv1alpha1.GrafanaWorkspaceSpec{
			WorkspaceName:           "dashboards",
			AccountAccessType:       "CURRENT_ACCOUNT",
			AuthenticationProviders: []string{"AWS_SSO"},
			PermissionType:          "CUSTOMER_MANAGED",
			WorkspaceRoleRef:        &awsv1alpha1.RoleRef{ARN: "arn:aws:iam::123456789012:role/grafana"},
		},
	}
	for _, m := range mutate {
		m(ws)
	}
	return ws
}

func TestGrafanaWorkspaceReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "dashboards", Namespace: "default"}}

	t.Run("create persists workspace ID and polls", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeGrafana{}
		scheme, c := devopsFakeClient(t, &awsv1alpha1.GrafanaWorkspace{}, grafanaWorkspaceCR())
		r := &GrafanaWorkspaceReconciler{Client: c, Scheme: scheme, GrafanaClient: f}
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Error("expected CreateWorkspace to be called")
		}
		if res.RequeueAfter != requeueDevOpsCostPolling.RequeueAfter {
			t.Errorf("RequeueAfter = %v, want polling interval", res.RequeueAfter)
		}
		got := &awsv1alpha1.GrafanaWorkspace{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.WorkspaceID != testGrafanaWorkspaceID {
			t.Errorf("status.workspaceId = %q, want %q", got.Status.WorkspaceID, testGrafanaWorkspaceID)
		}
	})

	t.Run("delete calls AWS delete with workspace ID", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeGrafana{}
		obj := grafanaWorkspaceCR(func(ws *awsv1alpha1.GrafanaWorkspace) {
			ws.Finalizers = []string{awsv1alpha1.FinalizerName}
			ws.Status.WorkspaceID = testGrafanaWorkspaceID
		})
		scheme, c := devopsFakeClient(t, &awsv1alpha1.GrafanaWorkspace{}, obj)
		if err := c.Delete(ctx, grafanaWorkspaceCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &GrafanaWorkspaceReconciler{Client: c, Scheme: scheme, GrafanaClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedID != testGrafanaWorkspaceID {
			t.Errorf("DeleteWorkspace called=%v id=%q", f.deleteCalled, f.deletedID)
		}
		devopsAssertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.GrafanaWorkspace{})
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeGrafana{}
		obj := grafanaWorkspaceCR(func(ws *awsv1alpha1.GrafanaWorkspace) {
			ws.Finalizers = []string{awsv1alpha1.FinalizerName}
			ws.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			ws.Status.WorkspaceID = testGrafanaWorkspaceID
		})
		scheme, c := devopsFakeClient(t, &awsv1alpha1.GrafanaWorkspace{}, obj)
		if err := c.Delete(ctx, grafanaWorkspaceCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &GrafanaWorkspaceReconciler{Client: c, Scheme: scheme, GrafanaClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteWorkspace must not be called when abandoning")
		}
		devopsAssertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.GrafanaWorkspace{})
	})
}

// ---------------------------------------------------------------- CostAnomalyMonitor

type fakeCEMonitor struct {
	monitors     []cetypes.AnomalyMonitor
	createCalled bool
	deleteCalled bool
	deletedARN   string
}

const testCEMonitorARN = "arn:aws:ce::123456789012:anomalymonitor/12345678-1234-1234-1234-123456789012"

func (f *fakeCEMonitor) GetAnomalyMonitors(_ context.Context, params *awsce.GetAnomalyMonitorsInput, _ ...func(*awsce.Options)) (*awsce.GetAnomalyMonitorsOutput, error) {
	if len(params.MonitorArnList) > 0 {
		var out []cetypes.AnomalyMonitor
		for _, m := range f.monitors {
			for _, arn := range params.MonitorArnList {
				if aws.ToString(m.MonitorArn) == arn {
					out = append(out, m)
				}
			}
		}
		if len(out) == 0 {
			return nil, devopsAPIErr{"UnknownMonitorException"}
		}
		return &awsce.GetAnomalyMonitorsOutput{AnomalyMonitors: out}, nil
	}
	return &awsce.GetAnomalyMonitorsOutput{AnomalyMonitors: f.monitors}, nil
}

func (f *fakeCEMonitor) CreateAnomalyMonitor(_ context.Context, params *awsce.CreateAnomalyMonitorInput, _ ...func(*awsce.Options)) (*awsce.CreateAnomalyMonitorOutput, error) {
	f.createCalled = true
	if params.AnomalyMonitor == nil || params.AnomalyMonitor.MonitorType != cetypes.MonitorTypeDimensional {
		return nil, fmt.Errorf("unexpected monitor type")
	}
	return &awsce.CreateAnomalyMonitorOutput{MonitorArn: aws.String(testCEMonitorARN)}, nil
}

func (f *fakeCEMonitor) UpdateAnomalyMonitor(_ context.Context, _ *awsce.UpdateAnomalyMonitorInput, _ ...func(*awsce.Options)) (*awsce.UpdateAnomalyMonitorOutput, error) {
	return &awsce.UpdateAnomalyMonitorOutput{}, nil
}

func (f *fakeCEMonitor) DeleteAnomalyMonitor(_ context.Context, params *awsce.DeleteAnomalyMonitorInput, _ ...func(*awsce.Options)) (*awsce.DeleteAnomalyMonitorOutput, error) {
	f.deleteCalled = true
	f.deletedARN = aws.ToString(params.MonitorArn)
	return &awsce.DeleteAnomalyMonitorOutput{}, nil
}

func ceMonitorCR(mutate ...func(*awsv1alpha1.CostAnomalyMonitor)) *awsv1alpha1.CostAnomalyMonitor {
	m := &awsv1alpha1.CostAnomalyMonitor{
		ObjectMeta: metav1.ObjectMeta{Name: "service-monitor", Namespace: "default"},
		Spec: awsv1alpha1.CostAnomalyMonitorSpec{
			MonitorName:      "service-monitor",
			MonitorType:      "DIMENSIONAL",
			MonitorDimension: "SERVICE",
		},
	}
	for _, mu := range mutate {
		mu(m)
	}
	return m
}

func TestCostAnomalyMonitorReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "service-monitor", Namespace: "default"}}

	t.Run("create persists ARN and Ready", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeCEMonitor{}
		scheme, c := devopsFakeClient(t, &awsv1alpha1.CostAnomalyMonitor{}, ceMonitorCR())
		r := &CostAnomalyMonitorReconciler{Client: c, Scheme: scheme, CostExplorerClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Error("expected CreateAnomalyMonitor to be called")
		}
		got := &awsv1alpha1.CostAnomalyMonitor{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ARN != testCEMonitorARN {
			t.Errorf("status.arn = %q, want %q", got.Status.ARN, testCEMonitorARN)
		}
		devopsAssertReadyTrue(t, got.Status.Conditions)
	})

	t.Run("delete calls AWS delete with ARN", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeCEMonitor{}
		obj := ceMonitorCR(func(m *awsv1alpha1.CostAnomalyMonitor) {
			m.Finalizers = []string{awsv1alpha1.FinalizerName}
			m.Status.ARN = testCEMonitorARN
		})
		scheme, c := devopsFakeClient(t, &awsv1alpha1.CostAnomalyMonitor{}, obj)
		if err := c.Delete(ctx, ceMonitorCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &CostAnomalyMonitorReconciler{Client: c, Scheme: scheme, CostExplorerClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedARN != testCEMonitorARN {
			t.Errorf("DeleteAnomalyMonitor called=%v arn=%q", f.deleteCalled, f.deletedARN)
		}
		devopsAssertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.CostAnomalyMonitor{})
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeCEMonitor{}
		obj := ceMonitorCR(func(m *awsv1alpha1.CostAnomalyMonitor) {
			m.Finalizers = []string{awsv1alpha1.FinalizerName}
			m.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			m.Status.ARN = testCEMonitorARN
		})
		scheme, c := devopsFakeClient(t, &awsv1alpha1.CostAnomalyMonitor{}, obj)
		if err := c.Delete(ctx, ceMonitorCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &CostAnomalyMonitorReconciler{Client: c, Scheme: scheme, CostExplorerClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteAnomalyMonitor must not be called when abandoning")
		}
		devopsAssertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.CostAnomalyMonitor{})
	})
}

// ---------------------------------------------------------------- CostAnomalySubscription

type fakeCESubscription struct {
	subscriptions []cetypes.AnomalySubscription
	createCalled  bool
	createInput   *awsce.CreateAnomalySubscriptionInput
	deleteCalled  bool
	deletedARN    string
}

const testCESubscriptionARN = "arn:aws:ce::123456789012:anomalysubscription/87654321-4321-4321-4321-210987654321"

func (f *fakeCESubscription) GetAnomalySubscriptions(_ context.Context, params *awsce.GetAnomalySubscriptionsInput, _ ...func(*awsce.Options)) (*awsce.GetAnomalySubscriptionsOutput, error) {
	if len(params.SubscriptionArnList) > 0 {
		var out []cetypes.AnomalySubscription
		for _, s := range f.subscriptions {
			for _, arn := range params.SubscriptionArnList {
				if aws.ToString(s.SubscriptionArn) == arn {
					out = append(out, s)
				}
			}
		}
		if len(out) == 0 {
			return nil, devopsAPIErr{"UnknownSubscriptionException"}
		}
		return &awsce.GetAnomalySubscriptionsOutput{AnomalySubscriptions: out}, nil
	}
	return &awsce.GetAnomalySubscriptionsOutput{AnomalySubscriptions: f.subscriptions}, nil
}

func (f *fakeCESubscription) CreateAnomalySubscription(_ context.Context, params *awsce.CreateAnomalySubscriptionInput, _ ...func(*awsce.Options)) (*awsce.CreateAnomalySubscriptionOutput, error) {
	f.createCalled = true
	f.createInput = params
	return &awsce.CreateAnomalySubscriptionOutput{SubscriptionArn: aws.String(testCESubscriptionARN)}, nil
}

func (f *fakeCESubscription) UpdateAnomalySubscription(_ context.Context, _ *awsce.UpdateAnomalySubscriptionInput, _ ...func(*awsce.Options)) (*awsce.UpdateAnomalySubscriptionOutput, error) {
	return &awsce.UpdateAnomalySubscriptionOutput{}, nil
}

func (f *fakeCESubscription) DeleteAnomalySubscription(_ context.Context, params *awsce.DeleteAnomalySubscriptionInput, _ ...func(*awsce.Options)) (*awsce.DeleteAnomalySubscriptionOutput, error) {
	f.deleteCalled = true
	f.deletedARN = aws.ToString(params.SubscriptionArn)
	return &awsce.DeleteAnomalySubscriptionOutput{}, nil
}

func ceSubscriptionCR(mutate ...func(*awsv1alpha1.CostAnomalySubscription)) *awsv1alpha1.CostAnomalySubscription {
	s := &awsv1alpha1.CostAnomalySubscription{
		ObjectMeta: metav1.ObjectMeta{Name: "cost-alerts", Namespace: "default"},
		Spec: awsv1alpha1.CostAnomalySubscriptionSpec{
			SubscriptionName: "cost-alerts",
			MonitorRefs:      []awsv1alpha1.CostAnomalyMonitorRef{{ARN: testCEMonitorARN}},
			Threshold:        "100",
			Frequency:        "DAILY",
			Subscribers: []awsv1alpha1.CostAnomalySubscriber{{
				Address: "ops@example.com",
				Type:    "EMAIL",
			}},
		},
	}
	for _, m := range mutate {
		m(s)
	}
	return s
}

func TestCostAnomalySubscriptionReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "cost-alerts", Namespace: "default"}}

	t.Run("create builds threshold expression and persists ARN", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeCESubscription{}
		scheme, c := devopsFakeClient(t, &awsv1alpha1.CostAnomalySubscription{}, ceSubscriptionCR())
		r := &CostAnomalySubscriptionReconciler{Client: c, Scheme: scheme, CostExplorerClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateAnomalySubscription to be called")
		}
		sub := f.createInput.AnomalySubscription
		if len(sub.MonitorArnList) != 1 || sub.MonitorArnList[0] != testCEMonitorARN {
			t.Errorf("monitorArnList = %v", sub.MonitorArnList)
		}
		te := sub.ThresholdExpression
		if te == nil || te.Dimensions == nil || te.Dimensions.Key != cetypes.DimensionAnomalyTotalImpactAbsolute || len(te.Dimensions.Values) != 1 || te.Dimensions.Values[0] != "100" {
			t.Errorf("thresholdExpression = %+v", te)
		}
		got := &awsv1alpha1.CostAnomalySubscription{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ARN != testCESubscriptionARN {
			t.Errorf("status.arn = %q, want %q", got.Status.ARN, testCESubscriptionARN)
		}
		devopsAssertReadyTrue(t, got.Status.Conditions)
	})

	t.Run("monitor ref not ready requeues without error", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeCESubscription{}
		obj := ceSubscriptionCR(func(s *awsv1alpha1.CostAnomalySubscription) {
			s.Spec.MonitorRefs = []awsv1alpha1.CostAnomalyMonitorRef{{Name: "service-monitor"}}
		})
		monitor := ceMonitorCR() // no ARN yet
		scheme, c := devopsFakeClient(t, &awsv1alpha1.CostAnomalySubscription{}, obj, monitor)
		r := &CostAnomalySubscriptionReconciler{Client: c, Scheme: scheme, CostExplorerClient: f}
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res.RequeueAfter != requeueDependency.RequeueAfter {
			t.Errorf("RequeueAfter = %v, want dependency requeue", res.RequeueAfter)
		}
		if f.createCalled {
			t.Error("CreateAnomalySubscription must not be called while the monitor is not ready")
		}
	})

	t.Run("delete calls AWS delete with ARN", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeCESubscription{}
		obj := ceSubscriptionCR(func(s *awsv1alpha1.CostAnomalySubscription) {
			s.Finalizers = []string{awsv1alpha1.FinalizerName}
			s.Status.ARN = testCESubscriptionARN
		})
		scheme, c := devopsFakeClient(t, &awsv1alpha1.CostAnomalySubscription{}, obj)
		if err := c.Delete(ctx, ceSubscriptionCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &CostAnomalySubscriptionReconciler{Client: c, Scheme: scheme, CostExplorerClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedARN != testCESubscriptionARN {
			t.Errorf("DeleteAnomalySubscription called=%v arn=%q", f.deleteCalled, f.deletedARN)
		}
		devopsAssertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.CostAnomalySubscription{})
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeCESubscription{}
		obj := ceSubscriptionCR(func(s *awsv1alpha1.CostAnomalySubscription) {
			s.Finalizers = []string{awsv1alpha1.FinalizerName}
			s.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			s.Status.ARN = testCESubscriptionARN
		})
		scheme, c := devopsFakeClient(t, &awsv1alpha1.CostAnomalySubscription{}, obj)
		if err := c.Delete(ctx, ceSubscriptionCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		r := &CostAnomalySubscriptionReconciler{Client: c, Scheme: scheme, CostExplorerClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteAnomalySubscription must not be called when abandoning")
		}
		devopsAssertGone(t, ctx, c, req.NamespacedName, &awsv1alpha1.CostAnomalySubscription{})
	})
}
