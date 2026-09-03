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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeLambdaScaling struct {
	putPC    func(ctx context.Context, params *awslambda.PutProvisionedConcurrencyConfigInput) (*awslambda.PutProvisionedConcurrencyConfigOutput, error)
	deletePC func(ctx context.Context, params *awslambda.DeleteProvisionedConcurrencyConfigInput) (*awslambda.DeleteProvisionedConcurrencyConfigOutput, error)
	putEIC   func(ctx context.Context, params *awslambda.PutFunctionEventInvokeConfigInput) (*awslambda.PutFunctionEventInvokeConfigOutput, error)
	delEIC   func(ctx context.Context, params *awslambda.DeleteFunctionEventInvokeConfigInput) (*awslambda.DeleteFunctionEventInvokeConfigOutput, error)

	putPCCalled    bool
	deletePCCalled bool
	putEICCalled   bool
	delEICCalled   bool
}

func (f *fakeLambdaScaling) PutProvisionedConcurrencyConfig(ctx context.Context, params *awslambda.PutProvisionedConcurrencyConfigInput, _ ...func(*awslambda.Options)) (*awslambda.PutProvisionedConcurrencyConfigOutput, error) {
	f.putPCCalled = true
	if f.putPC == nil {
		return nil, fmt.Errorf("unexpected call to PutProvisionedConcurrencyConfig")
	}
	return f.putPC(ctx, params)
}

func (f *fakeLambdaScaling) DeleteProvisionedConcurrencyConfig(ctx context.Context, params *awslambda.DeleteProvisionedConcurrencyConfigInput, _ ...func(*awslambda.Options)) (*awslambda.DeleteProvisionedConcurrencyConfigOutput, error) {
	f.deletePCCalled = true
	if f.deletePC == nil {
		return nil, fmt.Errorf("unexpected call to DeleteProvisionedConcurrencyConfig")
	}
	return f.deletePC(ctx, params)
}

func (f *fakeLambdaScaling) PutFunctionEventInvokeConfig(ctx context.Context, params *awslambda.PutFunctionEventInvokeConfigInput, _ ...func(*awslambda.Options)) (*awslambda.PutFunctionEventInvokeConfigOutput, error) {
	f.putEICCalled = true
	if f.putEIC == nil {
		return nil, fmt.Errorf("unexpected call to PutFunctionEventInvokeConfig")
	}
	return f.putEIC(ctx, params)
}

func (f *fakeLambdaScaling) DeleteFunctionEventInvokeConfig(ctx context.Context, params *awslambda.DeleteFunctionEventInvokeConfigInput, _ ...func(*awslambda.Options)) (*awslambda.DeleteFunctionEventInvokeConfigOutput, error) {
	f.delEICCalled = true
	if f.delEIC == nil {
		return nil, fmt.Errorf("unexpected call to DeleteFunctionEventInvokeConfig")
	}
	return f.delEIC(ctx, params)
}

func TestLambdaProvisionedConcurrencyReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-pc", Namespace: "default"}}
	pcCR := func(mutate ...func(*awsv1alpha1.LambdaProvisionedConcurrency)) *awsv1alpha1.LambdaProvisionedConcurrency {
		pc := &awsv1alpha1.LambdaProvisionedConcurrency{
			ObjectMeta: metav1.ObjectMeta{Name: "my-pc", Namespace: "default"},
			Spec: awsv1alpha1.LambdaProvisionedConcurrencySpec{
				FunctionName:                    "my-fn",
				Qualifier:                       "live",
				ProvisionedConcurrentExecutions: 5,
			},
		}
		for _, m := range mutate {
			m(pc)
		}
		return pc
	}

	t.Run("create puts config and records status", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, pcCR())
		f := &fakeLambdaScaling{
			putPC: func(_ context.Context, params *awslambda.PutProvisionedConcurrencyConfigInput) (*awslambda.PutProvisionedConcurrencyConfigOutput, error) {
				if aws.ToString(params.FunctionName) != "my-fn" || aws.ToString(params.Qualifier) != "live" {
					t.Errorf("fn/qualifier = %q/%q", aws.ToString(params.FunctionName), aws.ToString(params.Qualifier))
				}
				if aws.ToInt32(params.ProvisionedConcurrentExecutions) != 5 {
					t.Errorf("executions = %d", aws.ToInt32(params.ProvisionedConcurrentExecutions))
				}
				return &awslambda.PutProvisionedConcurrencyConfigOutput{
					AllocatedProvisionedConcurrentExecutions: aws.Int32(5),
					Status:                                   lambdatypes.ProvisionedConcurrencyStatusEnumInProgress,
				}, nil
			},
		}
		r := &LambdaProvisionedConcurrencyReconciler{Client: c, Scheme: scheme, LambdaClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.LambdaProvisionedConcurrency{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.FunctionName != "my-fn" || got.Status.Status != "IN_PROGRESS" {
			t.Errorf("status = %+v", got.Status)
		}
	})

	t.Run("delete calls DeleteProvisionedConcurrencyConfig", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, pcCR(func(pc *awsv1alpha1.LambdaProvisionedConcurrency) {
			pc.Finalizers = []string{awsv1alpha1.FinalizerName}
			pc.Status.FunctionName = "my-fn"
		}))
		f := &fakeLambdaScaling{
			deletePC: func(_ context.Context, params *awslambda.DeleteProvisionedConcurrencyConfigInput) (*awslambda.DeleteProvisionedConcurrencyConfigOutput, error) {
				if aws.ToString(params.FunctionName) != "my-fn" || aws.ToString(params.Qualifier) != "live" {
					t.Errorf("delete fn/qualifier = %q/%q", aws.ToString(params.FunctionName), aws.ToString(params.Qualifier))
				}
				return &awslambda.DeleteProvisionedConcurrencyConfigOutput{}, nil
			},
		}
		r := &LambdaProvisionedConcurrencyReconciler{Client: c, Scheme: scheme, LambdaClient: f}
		if err := c.Delete(ctx, pcCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deletePCCalled {
			t.Error("expected DeleteProvisionedConcurrencyConfig to be called")
		}
		got := &awsv1alpha1.LambdaProvisionedConcurrency{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, pcCR(func(pc *awsv1alpha1.LambdaProvisionedConcurrency) {
			pc.Finalizers = []string{awsv1alpha1.FinalizerName}
			pc.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		}))
		f := &fakeLambdaScaling{}
		r := &LambdaProvisionedConcurrencyReconciler{Client: c, Scheme: scheme, LambdaClient: f}
		if err := c.Delete(ctx, pcCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deletePCCalled {
			t.Error("DeleteProvisionedConcurrencyConfig must not be called when abandoning")
		}
	})
}

func TestLambdaEventInvokeConfigReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-eic", Namespace: "default"}}
	fnARN := "arn:aws:lambda:us-east-1:123456789012:function:my-fn:$LATEST"
	eicCR := func(mutate ...func(*awsv1alpha1.LambdaEventInvokeConfig)) *awsv1alpha1.LambdaEventInvokeConfig {
		eic := &awsv1alpha1.LambdaEventInvokeConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "my-eic", Namespace: "default"},
			Spec: awsv1alpha1.LambdaEventInvokeConfigSpec{
				FunctionName:             "my-fn",
				MaximumRetryAttempts:     aws.Int32(1),
				MaximumEventAgeInSeconds: aws.Int32(3600),
				OnFailureDestinationARN:  "arn:aws:sqs:us-east-1:123456789012:failures",
			},
		}
		for _, m := range mutate {
			m(eic)
		}
		return eic
	}

	t.Run("create puts config and records function ARN", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, eicCR())
		f := &fakeLambdaScaling{
			putEIC: func(_ context.Context, params *awslambda.PutFunctionEventInvokeConfigInput) (*awslambda.PutFunctionEventInvokeConfigOutput, error) {
				if aws.ToInt32(params.MaximumRetryAttempts) != 1 {
					t.Errorf("retries = %d", aws.ToInt32(params.MaximumRetryAttempts))
				}
				if params.DestinationConfig == nil || params.DestinationConfig.OnFailure == nil {
					t.Error("onFailure destination not passed")
				}
				return &awslambda.PutFunctionEventInvokeConfigOutput{FunctionArn: aws.String(fnARN)}, nil
			},
		}
		r := &LambdaEventInvokeConfigReconciler{Client: c, Scheme: scheme, LambdaClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.LambdaEventInvokeConfig{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.FunctionARN != fnARN {
			t.Errorf("status.functionArn = %q", got.Status.FunctionARN)
		}
	})

	t.Run("delete calls DeleteFunctionEventInvokeConfig", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, eicCR(func(eic *awsv1alpha1.LambdaEventInvokeConfig) {
			eic.Finalizers = []string{awsv1alpha1.FinalizerName}
			eic.Status.FunctionARN = fnARN
		}))
		f := &fakeLambdaScaling{
			delEIC: func(_ context.Context, params *awslambda.DeleteFunctionEventInvokeConfigInput) (*awslambda.DeleteFunctionEventInvokeConfigOutput, error) {
				if aws.ToString(params.FunctionName) != fnARN {
					t.Errorf("delete fn = %q", aws.ToString(params.FunctionName))
				}
				return &awslambda.DeleteFunctionEventInvokeConfigOutput{}, nil
			},
		}
		r := &LambdaEventInvokeConfigReconciler{Client: c, Scheme: scheme, LambdaClient: f}
		if err := c.Delete(ctx, eicCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.delEICCalled {
			t.Error("expected DeleteFunctionEventInvokeConfig to be called")
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, eicCR(func(eic *awsv1alpha1.LambdaEventInvokeConfig) {
			eic.Finalizers = []string{awsv1alpha1.FinalizerName}
			eic.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		}))
		f := &fakeLambdaScaling{}
		r := &LambdaEventInvokeConfigReconciler{Client: c, Scheme: scheme, LambdaClient: f}
		if err := c.Delete(ctx, eicCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.delEICCalled {
			t.Error("DeleteFunctionEventInvokeConfig must not be called when abandoning")
		}
	})
}
