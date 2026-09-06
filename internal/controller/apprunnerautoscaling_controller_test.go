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
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsapprunner "github.com/aws/aws-sdk-go-v2/service/apprunner"
	apprunnertypes "github.com/aws/aws-sdk-go-v2/service/apprunner/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

const testAppRunnerASCARN = "arn:aws:apprunner:us-east-1:123456789012:autoscalingconfiguration/my-asc/1/xyz"

type fakeAppRunnerASCAPI struct {
	createCalled bool
	deleteCalled bool
	deleteInput  *awsapprunner.DeleteAutoScalingConfigurationInput
}

func (f *fakeAppRunnerASCAPI) CreateAutoScalingConfiguration(_ context.Context, params *awsapprunner.CreateAutoScalingConfigurationInput, _ ...func(*awsapprunner.Options)) (*awsapprunner.CreateAutoScalingConfigurationOutput, error) {
	f.createCalled = true
	return &awsapprunner.CreateAutoScalingConfigurationOutput{
		AutoScalingConfiguration: &apprunnertypes.AutoScalingConfiguration{
			AutoScalingConfigurationArn:      aws.String(testAppRunnerASCARN),
			AutoScalingConfigurationName:     params.AutoScalingConfigurationName,
			AutoScalingConfigurationRevision: aws.Int32(1),
		},
	}, nil
}

func (f *fakeAppRunnerASCAPI) DeleteAutoScalingConfiguration(_ context.Context, params *awsapprunner.DeleteAutoScalingConfigurationInput, _ ...func(*awsapprunner.Options)) (*awsapprunner.DeleteAutoScalingConfigurationOutput, error) {
	f.deleteCalled = true
	f.deleteInput = params
	return &awsapprunner.DeleteAutoScalingConfigurationOutput{}, nil
}

func appRunnerASCCR(mutate ...func(*awsv1alpha1.AppRunnerAutoScaling)) *awsv1alpha1.AppRunnerAutoScaling {
	asc := &awsv1alpha1.AppRunnerAutoScaling{
		ObjectMeta: metav1.ObjectMeta{Name: "my-asc", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.AppRunnerAutoScalingSpec{
			Name:           "my-asc",
			MaxConcurrency: 50,
			MaxSize:        10,
			MinSize:        1,
		},
	}
	for _, m := range mutate {
		m(asc)
	}
	return asc
}

func TestAppRunnerAutoScalingReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-asc", Namespace: "default"}}
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}

	t.Run("create persists ARN and becomes Ready", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeAppRunnerASCAPI{}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.AppRunnerAutoScaling{}).
			WithObjects(appRunnerASCCR()).Build()
		r := &AppRunnerAutoScalingReconciler{Client: c, Scheme: scheme, AppRunnerClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateAutoScalingConfiguration to be called")
		}
		got := &awsv1alpha1.AppRunnerAutoScaling{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.AutoScalingConfigurationARN != testAppRunnerASCARN {
			t.Errorf("status.autoScalingConfigurationArn = %q, want %q", got.Status.AutoScalingConfigurationARN, testAppRunnerASCARN)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
	})

	t.Run("spec change after create reports UpdateNotSupported", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeAppRunnerASCAPI{}
		asc := appRunnerASCCR(func(a *awsv1alpha1.AppRunnerAutoScaling) {
			a.Finalizers = []string{awsv1alpha1.FinalizerName}
			a.Generation = 2
			a.Status.AutoScalingConfigurationARN = testAppRunnerASCARN
			a.Status.ObservedGeneration = 1
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.AppRunnerAutoScaling{}).
			WithObjects(asc).Build()
		r := &AppRunnerAutoScalingReconciler{Client: c, Scheme: scheme, AppRunnerClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.createCalled {
			t.Error("expected no create for existing configuration")
		}
		got := &awsv1alpha1.AppRunnerAutoScaling{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != awsv1alpha1.ReasonUpdateNotSupported {
			t.Errorf("Ready condition = %+v, want False/UpdateNotSupported", cond)
		}
		if got.Status.ObservedGeneration == got.Generation {
			t.Error("observedGeneration must not be bumped on unsupported update")
		}
	})

	t.Run("delete with finalizer calls DeleteAutoScalingConfiguration", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeAppRunnerASCAPI{}
		now := metav1.Now()
		asc := appRunnerASCCR(func(a *awsv1alpha1.AppRunnerAutoScaling) {
			a.Finalizers = []string{awsv1alpha1.FinalizerName}
			a.DeletionTimestamp = &now
			a.Status.AutoScalingConfigurationARN = testAppRunnerASCARN
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.AppRunnerAutoScaling{}).
			WithObjects(asc).Build()
		r := &AppRunnerAutoScalingReconciler{Client: c, Scheme: scheme, AppRunnerClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled {
			t.Fatal("expected DeleteAutoScalingConfiguration to be called")
		}
		if aws.ToString(f.deleteInput.AutoScalingConfigurationArn) != testAppRunnerASCARN {
			t.Errorf("delete ARN = %q, want %q", aws.ToString(f.deleteInput.AutoScalingConfigurationArn), testAppRunnerASCARN)
		}
		got := &awsv1alpha1.AppRunnerAutoScaling{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})

	t.Run("abandon annotation skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeAppRunnerASCAPI{}
		now := metav1.Now()
		asc := appRunnerASCCR(func(a *awsv1alpha1.AppRunnerAutoScaling) {
			a.Finalizers = []string{awsv1alpha1.FinalizerName}
			a.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			a.DeletionTimestamp = &now
			a.Status.AutoScalingConfigurationARN = testAppRunnerASCARN
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.AppRunnerAutoScaling{}).
			WithObjects(asc).Build()
		r := &AppRunnerAutoScalingReconciler{Client: c, Scheme: scheme, AppRunnerClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("expected DeleteAutoScalingConfiguration NOT to be called for abandoned resource")
		}
	})
}
