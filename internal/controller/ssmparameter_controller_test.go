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
	awsssm "github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"
	corev1 "k8s.io/api/core/v1"
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

type fakeSSM struct {
	getParameter    func(ctx context.Context, params *awsssm.GetParameterInput) (*awsssm.GetParameterOutput, error)
	putParameter    func(ctx context.Context, params *awsssm.PutParameterInput) (*awsssm.PutParameterOutput, error)
	deleteParameter func(ctx context.Context, params *awsssm.DeleteParameterInput) (*awsssm.DeleteParameterOutput, error)

	putCalled    bool
	putInput     *awsssm.PutParameterInput
	deleteCalled bool
	deletedName  string
}

func (f *fakeSSM) GetParameter(ctx context.Context, params *awsssm.GetParameterInput, _ ...func(*awsssm.Options)) (*awsssm.GetParameterOutput, error) {
	if f.getParameter == nil {
		return nil, fmt.Errorf("unexpected call to GetParameter")
	}
	return f.getParameter(ctx, params)
}

func (f *fakeSSM) PutParameter(ctx context.Context, params *awsssm.PutParameterInput, _ ...func(*awsssm.Options)) (*awsssm.PutParameterOutput, error) {
	f.putCalled = true
	f.putInput = params
	if f.putParameter == nil {
		return nil, fmt.Errorf("unexpected call to PutParameter")
	}
	return f.putParameter(ctx, params)
}

func (f *fakeSSM) DeleteParameter(ctx context.Context, params *awsssm.DeleteParameterInput, _ ...func(*awsssm.Options)) (*awsssm.DeleteParameterOutput, error) {
	f.deleteCalled = true
	f.deletedName = aws.ToString(params.Name)
	if f.deleteParameter == nil {
		return nil, fmt.Errorf("unexpected call to DeleteParameter")
	}
	return f.deleteParameter(ctx, params)
}

func ssmNotFoundErr() error {
	return &smithy.GenericAPIError{Code: "ParameterNotFound", Message: "not found"}
}

const testParamARN = "arn:aws:ssm:us-east-1:123456789012:parameter/myapp/db/password"

func ssmParamCR(mutate ...func(*awsv1alpha1.SSMParameter)) *awsv1alpha1.SSMParameter {
	p := &awsv1alpha1.SSMParameter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-param",
			Namespace: "default",
		},
		Spec: awsv1alpha1.SSMParameterSpec{
			ParameterName: "/myapp/db/password",
			Type:          "SecureString",
			ValueFrom: &awsv1alpha1.SecretRef{
				Name: "param-source",
				Key:  "value",
			},
		},
	}
	for _, m := range mutate {
		m(p)
	}
	return p
}

func ssmSourceSecret(value string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "param-source",
			Namespace: "default",
		},
		Data: map[string][]byte{"value": []byte(value)},
	}
}

func newSSMScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add aws scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	return scheme
}

func TestSSMParameterReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-param", Namespace: "default"}}

	tests := []struct {
		name    string
		objs    []client.Object
		fake    *fakeSSM
		setup   func(t *testing.T, ctx context.Context, c client.Client)
		wantErr bool
		assert  func(t *testing.T, ctx context.Context, c client.Client, f *fakeSSM, res ctrl.Result)
	}{
		{
			name: "create happy path resolves valueFrom and persists status",
			objs: []client.Object{ssmParamCR(), ssmSourceSecret("hunter2")},
			fake: &fakeSSM{
				putParameter: func(_ context.Context, params *awsssm.PutParameterInput) (*awsssm.PutParameterOutput, error) {
					if aws.ToString(params.Name) != "/myapp/db/password" {
						return nil, fmt.Errorf("unexpected parameter name")
					}
					if aws.ToString(params.Value) != "hunter2" {
						return nil, fmt.Errorf("valueFrom not resolved, got %q", aws.ToString(params.Value))
					}
					return &awsssm.PutParameterOutput{Version: 1}, nil
				},
				getParameter: func(_ context.Context, _ *awsssm.GetParameterInput) (*awsssm.GetParameterOutput, error) {
					return &awsssm.GetParameterOutput{
						Parameter: &ssmtypes.Parameter{
							ARN:     aws.String(testParamARN),
							Value:   aws.String("hunter2"),
							Version: 1,
						},
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSSM, _ ctrl.Result) {
				got := &awsv1alpha1.SSMParameter{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if !f.putCalled {
					t.Error("expected PutParameter to be called")
				}
				if got.Status.ARN != testParamARN {
					t.Errorf("status.arn = %q, want %q", got.Status.ARN, testParamARN)
				}
				if got.Status.Version != 1 {
					t.Errorf("status.version = %d, want 1", got.Status.Version)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "put succeeds but trailing get fails: version persisted, no error",
			objs: []client.Object{ssmParamCR(), ssmSourceSecret("hunter2")},
			fake: &fakeSSM{
				putParameter: func(_ context.Context, _ *awsssm.PutParameterInput) (*awsssm.PutParameterOutput, error) {
					return &awsssm.PutParameterOutput{Version: 1}, nil
				},
				getParameter: func(_ context.Context, _ *awsssm.GetParameterInput) (*awsssm.GetParameterOutput, error) {
					return nil, fmt.Errorf("throttled")
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSSM, _ ctrl.Result) {
				got := &awsv1alpha1.SSMParameter{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.Version != 1 {
					t.Errorf("status.version = %d, want 1 (persisted even when trailing describe fails)", got.Status.Version)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "steady state matching value does not put",
			objs: []client.Object{
				ssmParamCR(func(p *awsv1alpha1.SSMParameter) {
					p.Finalizers = []string{awsv1alpha1.FinalizerName}
					p.Status.ARN = testParamARN
				}),
				ssmSourceSecret("hunter2"),
			},
			fake: &fakeSSM{
				getParameter: func(_ context.Context, _ *awsssm.GetParameterInput) (*awsssm.GetParameterOutput, error) {
					return &awsssm.GetParameterOutput{
						Parameter: &ssmtypes.Parameter{
							ARN:     aws.String(testParamARN),
							Value:   aws.String("hunter2"),
							Version: 3,
						},
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSSM, _ ctrl.Result) {
				if f.putCalled {
					t.Error("PutParameter must not be called when value matches")
				}
				got := &awsv1alpha1.SSMParameter{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.Version != 3 {
					t.Errorf("status.version = %d, want 3", got.Status.Version)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "value drift triggers PutParameter with Overwrite",
			objs: []client.Object{
				ssmParamCR(func(p *awsv1alpha1.SSMParameter) {
					p.Finalizers = []string{awsv1alpha1.FinalizerName}
					p.Status.ARN = testParamARN
				}),
				ssmSourceSecret("rotated"),
			},
			fake: &fakeSSM{
				getParameter: func(_ context.Context, _ *awsssm.GetParameterInput) (*awsssm.GetParameterOutput, error) {
					return &awsssm.GetParameterOutput{
						Parameter: &ssmtypes.Parameter{
							ARN:     aws.String(testParamARN),
							Value:   aws.String("stale"),
							Version: 3,
						},
					}, nil
				},
				putParameter: func(_ context.Context, params *awsssm.PutParameterInput) (*awsssm.PutParameterOutput, error) {
					return &awsssm.PutParameterOutput{Version: 4}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSSM, _ ctrl.Result) {
				if !f.putCalled {
					t.Fatal("expected PutParameter to be called for drifted value")
				}
				if aws.ToString(f.putInput.Value) != "rotated" {
					t.Errorf("PutParameter value = %q, want %q", aws.ToString(f.putInput.Value), "rotated")
				}
				if !aws.ToBool(f.putInput.Overwrite) {
					t.Error("PutParameter Overwrite must be true when updating existing parameter")
				}
				if len(f.putInput.Tags) != 0 {
					t.Error("PutParameter must not send Tags with Overwrite")
				}
				got := &awsv1alpha1.SSMParameter{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.Version != 4 {
					t.Errorf("status.version = %d, want 4", got.Status.Version)
				}
			},
		},
		{
			name: "delete with finalizer calls AWS delete with parameter name",
			objs: []client.Object{ssmParamCR(func(p *awsv1alpha1.SSMParameter) {
				p.Finalizers = []string{awsv1alpha1.FinalizerName}
				p.Status.ARN = testParamARN
			})},
			fake: &fakeSSM{
				deleteParameter: func(_ context.Context, _ *awsssm.DeleteParameterInput) (*awsssm.DeleteParameterOutput, error) {
					return &awsssm.DeleteParameterOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, ssmParamCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSSM, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeleteParameter to be called")
				}
				if f.deletedName != "/myapp/db/password" {
					t.Errorf("DeleteParameter name = %q, want %q", f.deletedName, "/myapp/db/password")
				}
				got := &awsv1alpha1.SSMParameter{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "delete uses spec name even when status ARN empty (fallback)",
			objs: []client.Object{ssmParamCR(func(p *awsv1alpha1.SSMParameter) {
				p.Finalizers = []string{awsv1alpha1.FinalizerName}
			})},
			fake: &fakeSSM{
				deleteParameter: func(_ context.Context, _ *awsssm.DeleteParameterInput) (*awsssm.DeleteParameterOutput, error) {
					return nil, ssmNotFoundErr()
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, ssmParamCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSSM, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeleteParameter to be called via spec name")
				}
				if f.deletedName != "/myapp/db/password" {
					t.Errorf("DeleteParameter name = %q, want %q", f.deletedName, "/myapp/db/password")
				}
				got := &awsv1alpha1.SSMParameter{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone (NotFound from AWS is tolerated), got err=%v", err)
				}
			},
		},
		{
			name: "abandon policy skips AWS delete",
			objs: []client.Object{ssmParamCR(func(p *awsv1alpha1.SSMParameter) {
				p.Finalizers = []string{awsv1alpha1.FinalizerName}
				p.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				p.Status.ARN = testParamARN
			})},
			fake: &fakeSSM{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, ssmParamCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSSM, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeleteParameter must not be called when abandoning")
				}
				got := &awsv1alpha1.SSMParameter{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := newSSMScheme(t)
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&awsv1alpha1.SSMParameter{}).
				WithObjects(tc.objs...).
				Build()
			r := &SSMParameterReconciler{Client: c, Scheme: scheme, SSMClient: tc.fake}

			if tc.setup != nil {
				tc.setup(t, ctx, c)
			}

			res, err := r.Reconcile(ctx, req)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.assert(t, ctx, c, tc.fake, res)
		})
	}
}
