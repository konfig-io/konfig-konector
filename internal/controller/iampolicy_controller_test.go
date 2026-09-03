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

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// fakeIAMPolicyAPI implements IAMPolicyAWSAPI via function fields.
type fakeIAMPolicyAPI struct {
	CreatePolicyFn        func(ctx context.Context, params *awsiam.CreatePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.CreatePolicyOutput, error)
	GetPolicyFn           func(ctx context.Context, params *awsiam.GetPolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.GetPolicyOutput, error)
	GetPolicyVersionFn    func(ctx context.Context, params *awsiam.GetPolicyVersionInput, optFns ...func(*awsiam.Options)) (*awsiam.GetPolicyVersionOutput, error)
	ListPolicyVersionsFn  func(ctx context.Context, params *awsiam.ListPolicyVersionsInput, optFns ...func(*awsiam.Options)) (*awsiam.ListPolicyVersionsOutput, error)
	CreatePolicyVersionFn func(ctx context.Context, params *awsiam.CreatePolicyVersionInput, optFns ...func(*awsiam.Options)) (*awsiam.CreatePolicyVersionOutput, error)
	DeletePolicyVersionFn func(ctx context.Context, params *awsiam.DeletePolicyVersionInput, optFns ...func(*awsiam.Options)) (*awsiam.DeletePolicyVersionOutput, error)
	DeletePolicyFn        func(ctx context.Context, params *awsiam.DeletePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.DeletePolicyOutput, error)
}

func (f *fakeIAMPolicyAPI) CreatePolicy(ctx context.Context, params *awsiam.CreatePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.CreatePolicyOutput, error) {
	if f.CreatePolicyFn != nil {
		return f.CreatePolicyFn(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("unexpected CreatePolicy call")
}

func (f *fakeIAMPolicyAPI) GetPolicy(ctx context.Context, params *awsiam.GetPolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.GetPolicyOutput, error) {
	if f.GetPolicyFn != nil {
		return f.GetPolicyFn(ctx, params, optFns...)
	}
	return nil, &iamtypes.NoSuchEntityException{}
}

func (f *fakeIAMPolicyAPI) GetPolicyVersion(ctx context.Context, params *awsiam.GetPolicyVersionInput, optFns ...func(*awsiam.Options)) (*awsiam.GetPolicyVersionOutput, error) {
	if f.GetPolicyVersionFn != nil {
		return f.GetPolicyVersionFn(ctx, params, optFns...)
	}
	return &awsiam.GetPolicyVersionOutput{}, nil
}

func (f *fakeIAMPolicyAPI) ListPolicyVersions(ctx context.Context, params *awsiam.ListPolicyVersionsInput, optFns ...func(*awsiam.Options)) (*awsiam.ListPolicyVersionsOutput, error) {
	if f.ListPolicyVersionsFn != nil {
		return f.ListPolicyVersionsFn(ctx, params, optFns...)
	}
	return &awsiam.ListPolicyVersionsOutput{}, nil
}

func (f *fakeIAMPolicyAPI) CreatePolicyVersion(ctx context.Context, params *awsiam.CreatePolicyVersionInput, optFns ...func(*awsiam.Options)) (*awsiam.CreatePolicyVersionOutput, error) {
	if f.CreatePolicyVersionFn != nil {
		return f.CreatePolicyVersionFn(ctx, params, optFns...)
	}
	return &awsiam.CreatePolicyVersionOutput{PolicyVersion: &iamtypes.PolicyVersion{VersionId: aws.String("v2")}}, nil
}

func (f *fakeIAMPolicyAPI) DeletePolicyVersion(ctx context.Context, params *awsiam.DeletePolicyVersionInput, optFns ...func(*awsiam.Options)) (*awsiam.DeletePolicyVersionOutput, error) {
	if f.DeletePolicyVersionFn != nil {
		return f.DeletePolicyVersionFn(ctx, params, optFns...)
	}
	return &awsiam.DeletePolicyVersionOutput{}, nil
}

func (f *fakeIAMPolicyAPI) DeletePolicy(ctx context.Context, params *awsiam.DeletePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.DeletePolicyOutput, error) {
	if f.DeletePolicyFn != nil {
		return f.DeletePolicyFn(ctx, params, optFns...)
	}
	return &awsiam.DeletePolicyOutput{}, nil
}

const (
	testPolicyDoc = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`
	testPolicyARN = "arn:aws:iam::123456789012:policy/my-policy"
	testAccountID = "123456789012"
)

func newTestIAMPolicy(mutators ...func(*awsv1alpha1.IAMPolicy)) *awsv1alpha1.IAMPolicy {
	p := &awsv1alpha1.IAMPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "default",
		},
		Spec: awsv1alpha1.IAMPolicySpec{
			PolicyName:     "my-policy",
			PolicyDocument: testPolicyDoc,
		},
	}
	for _, m := range mutators {
		m(p)
	}
	return p
}

func newIAMPolicyFakeClient(s *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&awsv1alpha1.IAMPolicy{}).
		Build()
}

func TestIAMPolicyCreateHappyPath(t *testing.T) {
	s := iamRoleTestScheme(t)
	p := newTestIAMPolicy()
	cl := newIAMPolicyFakeClient(s, p)

	var created *awsiam.CreatePolicyInput
	fakeAWS := &fakeIAMPolicyAPI{
		CreatePolicyFn: func(_ context.Context, params *awsiam.CreatePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.CreatePolicyOutput, error) {
			created = params
			return &awsiam.CreatePolicyOutput{Policy: &iamtypes.Policy{
				Arn:              aws.String(testPolicyARN),
				PolicyId:         aws.String("ANPAEXAMPLE"),
				DefaultVersionId: aws.String("v1"),
			}}, nil
		},
	}
	r := &IAMPolicyReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS, AccountID: testAccountID}

	key := k8stypes.NamespacedName{Name: "test-policy", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if created == nil {
		t.Fatal("expected CreatePolicy to be called")
	}
	if aws.ToString(created.PolicyName) != "my-policy" {
		t.Errorf("CreatePolicy name = %q", aws.ToString(created.PolicyName))
	}

	got := &awsv1alpha1.IAMPolicy{}
	if err := cl.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.ARN != testPolicyARN {
		t.Errorf("status ARN = %q", got.Status.ARN)
	}
	if got.Status.PolicyID != "ANPAEXAMPLE" || got.Status.DefaultVersionID != "v1" {
		t.Errorf("status IDs = %q / %q", got.Status.PolicyID, got.Status.DefaultVersionID)
	}
	cond := meta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Errorf("Ready condition = %+v, want True", cond)
	}
}

func TestIAMPolicyAdoptOnAlreadyExists(t *testing.T) {
	s := iamRoleTestScheme(t)
	p := newTestIAMPolicy()
	cl := newIAMPolicyFakeClient(s, p)

	fakeAWS := &fakeIAMPolicyAPI{
		CreatePolicyFn: func(_ context.Context, _ *awsiam.CreatePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.CreatePolicyOutput, error) {
			return nil, &iamtypes.EntityAlreadyExistsException{}
		},
		GetPolicyFn: func(_ context.Context, params *awsiam.GetPolicyInput, _ ...func(*awsiam.Options)) (*awsiam.GetPolicyOutput, error) {
			if aws.ToString(params.PolicyArn) != testPolicyARN {
				t.Errorf("GetPolicy arn = %q, want constructed %q", aws.ToString(params.PolicyArn), testPolicyARN)
			}
			return &awsiam.GetPolicyOutput{Policy: &iamtypes.Policy{
				Arn:              aws.String(testPolicyARN),
				PolicyId:         aws.String("ANPAEXAMPLE"),
				DefaultVersionId: aws.String("v1"),
			}}, nil
		},
	}
	r := &IAMPolicyReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS, AccountID: testAccountID}

	key := k8stypes.NamespacedName{Name: "test-policy", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &awsv1alpha1.IAMPolicy{}
	if err := cl.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.ARN != testPolicyARN {
		t.Errorf("status ARN = %q; adopted policy ARN must be persisted", got.Status.ARN)
	}
}

func TestIAMPolicySteadyStateNoCreate(t *testing.T) {
	s := iamRoleTestScheme(t)
	p := newTestIAMPolicy(func(p *awsv1alpha1.IAMPolicy) {
		p.Finalizers = []string{awsv1alpha1.FinalizerName}
		p.Status.ARN = testPolicyARN
		p.Status.PolicyID = "ANPAEXAMPLE"
		p.Status.DefaultVersionID = "v1"
	})
	cl := newIAMPolicyFakeClient(s, p)

	createCalls, createVersionCalls := 0, 0
	fakeAWS := &fakeIAMPolicyAPI{
		CreatePolicyFn: func(_ context.Context, _ *awsiam.CreatePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.CreatePolicyOutput, error) {
			createCalls++
			return nil, fmt.Errorf("should not be called")
		},
		GetPolicyFn: func(_ context.Context, _ *awsiam.GetPolicyInput, _ ...func(*awsiam.Options)) (*awsiam.GetPolicyOutput, error) {
			return &awsiam.GetPolicyOutput{Policy: &iamtypes.Policy{
				Arn:              aws.String(testPolicyARN),
				PolicyId:         aws.String("ANPAEXAMPLE"),
				DefaultVersionId: aws.String("v1"),
			}}, nil
		},
		GetPolicyVersionFn: func(_ context.Context, _ *awsiam.GetPolicyVersionInput, _ ...func(*awsiam.Options)) (*awsiam.GetPolicyVersionOutput, error) {
			return &awsiam.GetPolicyVersionOutput{PolicyVersion: &iamtypes.PolicyVersion{
				Document: aws.String(testPolicyDoc), // matches spec, no update needed
			}}, nil
		},
		CreatePolicyVersionFn: func(_ context.Context, _ *awsiam.CreatePolicyVersionInput, _ ...func(*awsiam.Options)) (*awsiam.CreatePolicyVersionOutput, error) {
			createVersionCalls++
			return nil, fmt.Errorf("should not be called")
		},
	}
	r := &IAMPolicyReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS, AccountID: testAccountID}

	key := k8stypes.NamespacedName{Name: "test-policy", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if createCalls != 0 {
		t.Errorf("CreatePolicy called %d times, want 0", createCalls)
	}
	if createVersionCalls != 0 {
		t.Errorf("CreatePolicyVersion called %d times, want 0", createVersionCalls)
	}
}

func TestIAMPolicyDocumentUpdate(t *testing.T) {
	s := iamRoleTestScheme(t)
	p := newTestIAMPolicy(func(p *awsv1alpha1.IAMPolicy) {
		p.Finalizers = []string{awsv1alpha1.FinalizerName}
		p.Status.ARN = testPolicyARN
		p.Status.DefaultVersionID = "v1"
	})
	cl := newIAMPolicyFakeClient(s, p)

	var newDoc string
	fakeAWS := &fakeIAMPolicyAPI{
		GetPolicyFn: func(_ context.Context, _ *awsiam.GetPolicyInput, _ ...func(*awsiam.Options)) (*awsiam.GetPolicyOutput, error) {
			return &awsiam.GetPolicyOutput{Policy: &iamtypes.Policy{Arn: aws.String(testPolicyARN)}}, nil
		},
		GetPolicyVersionFn: func(_ context.Context, _ *awsiam.GetPolicyVersionInput, _ ...func(*awsiam.Options)) (*awsiam.GetPolicyVersionOutput, error) {
			return &awsiam.GetPolicyVersionOutput{PolicyVersion: &iamtypes.PolicyVersion{
				Document: aws.String(`{"old":"doc"}`),
			}}, nil
		},
		CreatePolicyVersionFn: func(_ context.Context, params *awsiam.CreatePolicyVersionInput, _ ...func(*awsiam.Options)) (*awsiam.CreatePolicyVersionOutput, error) {
			newDoc = aws.ToString(params.PolicyDocument)
			return &awsiam.CreatePolicyVersionOutput{PolicyVersion: &iamtypes.PolicyVersion{VersionId: aws.String("v2")}}, nil
		},
	}
	r := &IAMPolicyReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS, AccountID: testAccountID}

	key := k8stypes.NamespacedName{Name: "test-policy", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if newDoc != testPolicyDoc {
		t.Errorf("CreatePolicyVersion doc = %q, want spec doc", newDoc)
	}

	got := &awsv1alpha1.IAMPolicy{}
	if err := cl.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.DefaultVersionID != "v2" {
		t.Errorf("status DefaultVersionID = %q, want v2", got.Status.DefaultVersionID)
	}
}

func TestIAMPolicyDeleteWithFinalizer(t *testing.T) {
	s := iamRoleTestScheme(t)
	p := newTestIAMPolicy(func(p *awsv1alpha1.IAMPolicy) {
		p.Finalizers = []string{awsv1alpha1.FinalizerName}
		p.Status.ARN = testPolicyARN
	})
	cl := newIAMPolicyFakeClient(s, p)

	deleteCalls := 0
	fakeAWS := &fakeIAMPolicyAPI{
		DeletePolicyFn: func(_ context.Context, params *awsiam.DeletePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.DeletePolicyOutput, error) {
			deleteCalls++
			if aws.ToString(params.PolicyArn) != testPolicyARN {
				t.Errorf("DeletePolicy arn = %q", aws.ToString(params.PolicyArn))
			}
			return &awsiam.DeletePolicyOutput{}, nil
		},
	}
	r := &IAMPolicyReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS, AccountID: testAccountID}

	ctx := context.Background()
	if err := cl.Delete(ctx, p); err != nil {
		t.Fatalf("delete: %v", err)
	}

	key := k8stypes.NamespacedName{Name: "test-policy", Namespace: "default"}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if deleteCalls != 1 {
		t.Errorf("DeletePolicy called %d times, want 1", deleteCalls)
	}

	got := &awsv1alpha1.IAMPolicy{}
	if err := cl.Get(ctx, key, got); !errors.IsNotFound(err) {
		t.Errorf("expected object gone, got err=%v", err)
	}
}

func TestIAMPolicyDeleteFallbackConstructsARN(t *testing.T) {
	// Status.ARN was never persisted; the delete path must construct the ARN
	// from the account ID + path + name and still delete the policy.
	s := iamRoleTestScheme(t)
	p := newTestIAMPolicy(func(p *awsv1alpha1.IAMPolicy) {
		p.Finalizers = []string{awsv1alpha1.FinalizerName}
	})
	cl := newIAMPolicyFakeClient(s, p)

	deleteCalls := 0
	fakeAWS := &fakeIAMPolicyAPI{
		GetPolicyFn: func(_ context.Context, params *awsiam.GetPolicyInput, _ ...func(*awsiam.Options)) (*awsiam.GetPolicyOutput, error) {
			if aws.ToString(params.PolicyArn) != testPolicyARN {
				t.Errorf("GetPolicy arn = %q, want constructed %q", aws.ToString(params.PolicyArn), testPolicyARN)
			}
			return &awsiam.GetPolicyOutput{Policy: &iamtypes.Policy{Arn: aws.String(testPolicyARN)}}, nil
		},
		DeletePolicyFn: func(_ context.Context, params *awsiam.DeletePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.DeletePolicyOutput, error) {
			deleteCalls++
			if aws.ToString(params.PolicyArn) != testPolicyARN {
				t.Errorf("DeletePolicy arn = %q, want %q", aws.ToString(params.PolicyArn), testPolicyARN)
			}
			return &awsiam.DeletePolicyOutput{}, nil
		},
	}
	r := &IAMPolicyReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS, AccountID: testAccountID}

	ctx := context.Background()
	if err := cl.Delete(ctx, p); err != nil {
		t.Fatalf("delete: %v", err)
	}

	key := k8stypes.NamespacedName{Name: "test-policy", Namespace: "default"}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if deleteCalls != 1 {
		t.Errorf("DeletePolicy called %d times, want 1 (ARN fallback)", deleteCalls)
	}
}

func TestIAMPolicyAbandonAnnotation(t *testing.T) {
	s := iamRoleTestScheme(t)
	p := newTestIAMPolicy(func(p *awsv1alpha1.IAMPolicy) {
		p.Finalizers = []string{awsv1alpha1.FinalizerName}
		p.Status.ARN = testPolicyARN
		p.Annotations = map[string]string{
			awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon,
		}
	})
	cl := newIAMPolicyFakeClient(s, p)

	deleteCalls := 0
	fakeAWS := &fakeIAMPolicyAPI{
		DeletePolicyFn: func(_ context.Context, _ *awsiam.DeletePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.DeletePolicyOutput, error) {
			deleteCalls++
			return &awsiam.DeletePolicyOutput{}, nil
		},
	}
	r := &IAMPolicyReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS, AccountID: testAccountID}

	ctx := context.Background()
	if err := cl.Delete(ctx, p); err != nil {
		t.Fatalf("delete: %v", err)
	}

	key := k8stypes.NamespacedName{Name: "test-policy", Namespace: "default"}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if deleteCalls != 0 {
		t.Errorf("DeletePolicy called %d times, want 0 (abandon)", deleteCalls)
	}

	got := &awsv1alpha1.IAMPolicy{}
	if err := cl.Get(ctx, key, got); !errors.IsNotFound(err) {
		t.Errorf("expected object gone, got err=%v", err)
	}
}
