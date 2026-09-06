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
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeLambdaAPI struct {
	getFunction    func(ctx context.Context, params *awslambda.GetFunctionInput) (*awslambda.GetFunctionOutput, error)
	createFunction func(ctx context.Context, params *awslambda.CreateFunctionInput) (*awslambda.CreateFunctionOutput, error)

	createCalled       bool
	updateConfigCalled bool
	updateCodeCalled   bool
	putConcCalled      bool
	deleteConcCalled   bool
	deleteCalled       bool
	updateConfigInput  *awslambda.UpdateFunctionConfigurationInput
	updateCodeInput    *awslambda.UpdateFunctionCodeInput
	putConcInput       *awslambda.PutFunctionConcurrencyInput
}

func (f *fakeLambdaAPI) GetFunction(ctx context.Context, params *awslambda.GetFunctionInput, _ ...func(*awslambda.Options)) (*awslambda.GetFunctionOutput, error) {
	if f.getFunction == nil {
		return nil, fmt.Errorf("unexpected call to GetFunction")
	}
	return f.getFunction(ctx, params)
}

func (f *fakeLambdaAPI) CreateFunction(ctx context.Context, params *awslambda.CreateFunctionInput, _ ...func(*awslambda.Options)) (*awslambda.CreateFunctionOutput, error) {
	f.createCalled = true
	if f.createFunction == nil {
		return nil, fmt.Errorf("unexpected call to CreateFunction")
	}
	return f.createFunction(ctx, params)
}

func (f *fakeLambdaAPI) UpdateFunctionConfiguration(_ context.Context, params *awslambda.UpdateFunctionConfigurationInput, _ ...func(*awslambda.Options)) (*awslambda.UpdateFunctionConfigurationOutput, error) {
	f.updateConfigCalled = true
	f.updateConfigInput = params
	return &awslambda.UpdateFunctionConfigurationOutput{}, nil
}

func (f *fakeLambdaAPI) UpdateFunctionCode(_ context.Context, params *awslambda.UpdateFunctionCodeInput, _ ...func(*awslambda.Options)) (*awslambda.UpdateFunctionCodeOutput, error) {
	f.updateCodeCalled = true
	f.updateCodeInput = params
	return &awslambda.UpdateFunctionCodeOutput{}, nil
}

func (f *fakeLambdaAPI) PutFunctionConcurrency(_ context.Context, params *awslambda.PutFunctionConcurrencyInput, _ ...func(*awslambda.Options)) (*awslambda.PutFunctionConcurrencyOutput, error) {
	f.putConcCalled = true
	f.putConcInput = params
	return &awslambda.PutFunctionConcurrencyOutput{}, nil
}

func (f *fakeLambdaAPI) DeleteFunctionConcurrency(_ context.Context, _ *awslambda.DeleteFunctionConcurrencyInput, _ ...func(*awslambda.Options)) (*awslambda.DeleteFunctionConcurrencyOutput, error) {
	f.deleteConcCalled = true
	return &awslambda.DeleteFunctionConcurrencyOutput{}, nil
}

func (f *fakeLambdaAPI) DeleteFunction(_ context.Context, _ *awslambda.DeleteFunctionInput, _ ...func(*awslambda.Options)) (*awslambda.DeleteFunctionOutput, error) {
	f.deleteCalled = true
	return &awslambda.DeleteFunctionOutput{}, nil
}

const (
	testLambdaARN  = "arn:aws:lambda:us-east-1:123456789012:function:my-fn"
	testLambdaRole = "arn:aws:iam::123456789012:role/lambda-exec"
)

func lambdaScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func lambdaFnCR(mutate ...func(*awsv1alpha1.LambdaFunction)) *awsv1alpha1.LambdaFunction {
	fn := &awsv1alpha1.LambdaFunction{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-fn",
			Namespace:  "default",
			Finalizers: []string{awsv1alpha1.FinalizerName},
		},
		Spec: awsv1alpha1.LambdaFunctionSpec{
			FunctionName: "my-fn",
			RoleArn:      testLambdaRole,
			Runtime:      "python3.12",
			Handler:      "index.handler",
			Code: awsv1alpha1.LambdaCodeSource{
				S3: &awsv1alpha1.LambdaS3Code{S3Bucket: "code-bucket", S3Key: "fn.zip"},
			},
		},
	}
	for _, m := range mutate {
		m(fn)
	}
	return fn
}

func lambdaActiveGetOutput() *awslambda.GetFunctionOutput {
	return &awslambda.GetFunctionOutput{
		Configuration: &lambdatypes.FunctionConfiguration{
			FunctionArn: aws.String(testLambdaARN),
			State:       lambdatypes.StateActive,
			CodeSha256:  aws.String("abc123"),
		},
	}
}

func TestLambdaFunctionReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-fn", Namespace: "default"}}

	t.Run("create happy path persists ARN then becomes Ready", func(t *testing.T) {
		ctx := context.Background()
		scheme := lambdaScheme(t)
		f := &fakeLambdaAPI{
			createFunction: func(_ context.Context, params *awslambda.CreateFunctionInput) (*awslambda.CreateFunctionOutput, error) {
				if aws.ToString(params.FunctionName) != "my-fn" {
					return nil, fmt.Errorf("unexpected function name %q", aws.ToString(params.FunctionName))
				}
				if aws.ToString(params.Role) != testLambdaRole {
					return nil, fmt.Errorf("unexpected role %q", aws.ToString(params.Role))
				}
				return &awslambda.CreateFunctionOutput{
					FunctionArn: aws.String(testLambdaARN),
					State:       lambdatypes.StatePending,
				}, nil
			},
			getFunction: func(_ context.Context, _ *awslambda.GetFunctionInput) (*awslambda.GetFunctionOutput, error) {
				return lambdaActiveGetOutput(), nil
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.LambdaFunction{}).
			WithObjects(lambdaFnCR()).Build()
		r := &LambdaFunctionReconciler{Client: c, Scheme: scheme, LambdaClient: f}

		// First reconcile: adds finalizer + creates the function.
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile 1: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateFunction to be called")
		}
		if res.RequeueAfter == 0 {
			t.Error("expected polling requeue after create")
		}
		got := &awsv1alpha1.LambdaFunction{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.FunctionARN != testLambdaARN {
			t.Errorf("status.functionArn = %q, want %q", got.Status.FunctionARN, testLambdaARN)
		}

		// Second reconcile: function is Active → Ready=True.
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile 2: %v", err)
		}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
		if got.Status.State != string(lambdatypes.StateActive) {
			t.Errorf("status.state = %q, want Active", got.Status.State)
		}
	})

	t.Run("steady state does not create or update", func(t *testing.T) {
		ctx := context.Background()
		scheme := lambdaScheme(t)
		f := &fakeLambdaAPI{
			getFunction: func(_ context.Context, _ *awslambda.GetFunctionInput) (*awslambda.GetFunctionOutput, error) {
				return lambdaActiveGetOutput(), nil
			},
		}
		fn := lambdaFnCR(func(fn *awsv1alpha1.LambdaFunction) {
			fn.Finalizers = []string{awsv1alpha1.FinalizerName}
			fn.Generation = 1
			fn.Status.FunctionARN = testLambdaARN
			fn.Status.ObservedGeneration = 1
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.LambdaFunction{}).
			WithObjects(fn).Build()
		r := &LambdaFunctionReconciler{Client: c, Scheme: scheme, LambdaClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.createCalled {
			t.Error("expected CreateFunction NOT to be called")
		}
		if f.updateConfigCalled || f.updateCodeCalled {
			t.Error("expected no update calls at steady state")
		}
	})

	t.Run("generation change updates config, code, and reserved concurrency", func(t *testing.T) {
		ctx := context.Background()
		scheme := lambdaScheme(t)
		f := &fakeLambdaAPI{
			getFunction: func(_ context.Context, _ *awslambda.GetFunctionInput) (*awslambda.GetFunctionOutput, error) {
				return lambdaActiveGetOutput(), nil
			},
		}
		rc := int32(10)
		fn := lambdaFnCR(func(fn *awsv1alpha1.LambdaFunction) {
			fn.Finalizers = []string{awsv1alpha1.FinalizerName}
			fn.Generation = 2
			fn.Spec.MemorySize = aws.Int32(256)
			fn.Spec.ReservedConcurrency = &rc
			fn.Status.FunctionARN = testLambdaARN
			fn.Status.ObservedGeneration = 1
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.LambdaFunction{}).
			WithObjects(fn).Build()
		r := &LambdaFunctionReconciler{Client: c, Scheme: scheme, LambdaClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.updateConfigCalled {
			t.Error("expected UpdateFunctionConfiguration to be called")
		}
		if f.updateConfigInput != nil && aws.ToInt32(f.updateConfigInput.MemorySize) != 256 {
			t.Errorf("update config memory = %d, want 256", aws.ToInt32(f.updateConfigInput.MemorySize))
		}
		if !f.updateCodeCalled {
			t.Error("expected UpdateFunctionCode to be called")
		}
		if f.updateCodeInput != nil && aws.ToString(f.updateCodeInput.S3Bucket) != "code-bucket" {
			t.Errorf("update code bucket = %q, want code-bucket", aws.ToString(f.updateCodeInput.S3Bucket))
		}
		if !f.putConcCalled {
			t.Error("expected PutFunctionConcurrency to be called")
		}
		if f.putConcInput != nil && aws.ToInt32(f.putConcInput.ReservedConcurrentExecutions) != 10 {
			t.Errorf("reserved concurrency = %d, want 10", aws.ToInt32(f.putConcInput.ReservedConcurrentExecutions))
		}
		got := &awsv1alpha1.LambdaFunction{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ObservedGeneration != 2 {
			t.Errorf("observedGeneration = %d, want 2", got.Status.ObservedGeneration)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
	})

	t.Run("reserved concurrency -1 removes reservation", func(t *testing.T) {
		ctx := context.Background()
		scheme := lambdaScheme(t)
		f := &fakeLambdaAPI{
			getFunction: func(_ context.Context, _ *awslambda.GetFunctionInput) (*awslambda.GetFunctionOutput, error) {
				return lambdaActiveGetOutput(), nil
			},
		}
		rc := int32(-1)
		fn := lambdaFnCR(func(fn *awsv1alpha1.LambdaFunction) {
			fn.Finalizers = []string{awsv1alpha1.FinalizerName}
			fn.Generation = 2
			fn.Spec.ReservedConcurrency = &rc
			fn.Status.FunctionARN = testLambdaARN
			fn.Status.ObservedGeneration = 1
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.LambdaFunction{}).
			WithObjects(fn).Build()
		r := &LambdaFunctionReconciler{Client: c, Scheme: scheme, LambdaClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteConcCalled {
			t.Error("expected DeleteFunctionConcurrency to be called for -1")
		}
		if f.putConcCalled {
			t.Error("expected PutFunctionConcurrency NOT to be called for -1")
		}
	})

	t.Run("delete with finalizer calls DeleteFunction", func(t *testing.T) {
		ctx := context.Background()
		scheme := lambdaScheme(t)
		f := &fakeLambdaAPI{}
		now := metav1.Now()
		fn := lambdaFnCR(func(fn *awsv1alpha1.LambdaFunction) {
			fn.Finalizers = []string{awsv1alpha1.FinalizerName}
			fn.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.LambdaFunction{}).
			WithObjects(fn).Build()
		r := &LambdaFunctionReconciler{Client: c, Scheme: scheme, LambdaClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled {
			t.Error("expected DeleteFunction to be called")
		}
		got := &awsv1alpha1.LambdaFunction{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})

	t.Run("abandon annotation skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := lambdaScheme(t)
		f := &fakeLambdaAPI{}
		now := metav1.Now()
		fn := lambdaFnCR(func(fn *awsv1alpha1.LambdaFunction) {
			fn.Finalizers = []string{awsv1alpha1.FinalizerName}
			fn.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			fn.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.LambdaFunction{}).
			WithObjects(fn).Build()
		r := &LambdaFunctionReconciler{Client: c, Scheme: scheme, LambdaClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("expected DeleteFunction NOT to be called for abandoned resource")
		}
		got := &awsv1alpha1.LambdaFunction{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})
}
