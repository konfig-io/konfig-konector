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
	awsaas "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
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

type fakeAAS struct {
	registerScalableTarget   func(ctx context.Context, params *awsaas.RegisterScalableTargetInput) (*awsaas.RegisterScalableTargetOutput, error)
	deregisterScalableTarget func(ctx context.Context, params *awsaas.DeregisterScalableTargetInput) (*awsaas.DeregisterScalableTargetOutput, error)
	putScalingPolicy         func(ctx context.Context, params *awsaas.PutScalingPolicyInput) (*awsaas.PutScalingPolicyOutput, error)
	deleteScalingPolicy      func(ctx context.Context, params *awsaas.DeleteScalingPolicyInput) (*awsaas.DeleteScalingPolicyOutput, error)

	registerCalled   bool
	registerInput    *awsaas.RegisterScalableTargetInput
	deregisterCalled bool
	deregisterInput  *awsaas.DeregisterScalableTargetInput
	putCalled        bool
	putInput         *awsaas.PutScalingPolicyInput
	deleteCalled     bool
	deleteInput      *awsaas.DeleteScalingPolicyInput
}

func (f *fakeAAS) RegisterScalableTarget(ctx context.Context, params *awsaas.RegisterScalableTargetInput, _ ...func(*awsaas.Options)) (*awsaas.RegisterScalableTargetOutput, error) {
	f.registerCalled = true
	f.registerInput = params
	if f.registerScalableTarget == nil {
		return nil, fmt.Errorf("unexpected call to RegisterScalableTarget")
	}
	return f.registerScalableTarget(ctx, params)
}

func (f *fakeAAS) DeregisterScalableTarget(ctx context.Context, params *awsaas.DeregisterScalableTargetInput, _ ...func(*awsaas.Options)) (*awsaas.DeregisterScalableTargetOutput, error) {
	f.deregisterCalled = true
	f.deregisterInput = params
	if f.deregisterScalableTarget == nil {
		return nil, fmt.Errorf("unexpected call to DeregisterScalableTarget")
	}
	return f.deregisterScalableTarget(ctx, params)
}

func (f *fakeAAS) PutScalingPolicy(ctx context.Context, params *awsaas.PutScalingPolicyInput, _ ...func(*awsaas.Options)) (*awsaas.PutScalingPolicyOutput, error) {
	f.putCalled = true
	f.putInput = params
	if f.putScalingPolicy == nil {
		return nil, fmt.Errorf("unexpected call to PutScalingPolicy")
	}
	return f.putScalingPolicy(ctx, params)
}

func (f *fakeAAS) DeleteScalingPolicy(ctx context.Context, params *awsaas.DeleteScalingPolicyInput, _ ...func(*awsaas.Options)) (*awsaas.DeleteScalingPolicyOutput, error) {
	f.deleteCalled = true
	f.deleteInput = params
	if f.deleteScalingPolicy == nil {
		return nil, fmt.Errorf("unexpected call to DeleteScalingPolicy")
	}
	return f.deleteScalingPolicy(ctx, params)
}

func aasNotFoundErr() error {
	return &smithy.GenericAPIError{Code: "ObjectNotFoundException", Message: "no scalable target found"}
}

func newScalingScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func newScalingFakeClient(scheme *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&awsv1alpha1.ScalableTarget{},
			&awsv1alpha1.AppScalingPolicy{},
			&awsv1alpha1.ScheduleGroup{},
			&awsv1alpha1.Schedule{},
			&awsv1alpha1.SSMMaintenanceWindow{},
			&awsv1alpha1.SSMPatchBaseline{},
			&awsv1alpha1.SSMAssociation{},
			&awsv1alpha1.RDSGlobalCluster{},
			&awsv1alpha1.RDSEventSubscription{},
			&awsv1alpha1.LambdaProvisionedConcurrency{},
			&awsv1alpha1.LambdaEventInvokeConfig{},
			&awsv1alpha1.S3AccessPoint{},
			&awsv1alpha1.IAMInstanceProfile{},
			&awsv1alpha1.IAMRole{},
			&awsv1alpha1.SNSTopic{},
			&awsv1alpha1.S3Bucket{},
			&awsv1alpha1.VPC{},
			&awsv1alpha1.LambdaFunction{},
			&awsv1alpha1.SSMDocument{},
		).
		WithObjects(objs...).
		Build()
}

const testScalableTargetARN = "arn:aws:application-autoscaling:us-east-1:123456789012:scalable-target/abc123"

func scalableTargetCR(mutate ...func(*awsv1alpha1.ScalableTarget)) *awsv1alpha1.ScalableTarget {
	st := &awsv1alpha1.ScalableTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-target",
			Namespace:  "default",
			Finalizers: []string{awsv1alpha1.FinalizerName},
		},
		Spec: awsv1alpha1.ScalableTargetSpec{
			ServiceNamespace:  "ecs",
			ResourceID:        "service/my-cluster/my-service",
			ScalableDimension: "ecs:service:DesiredCount",
			MinCapacity:       1,
			MaxCapacity:       10,
		},
	}
	for _, m := range mutate {
		m(st)
	}
	return st
}

func TestScalableTargetReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-target", Namespace: "default"}}

	tests := []struct {
		name    string
		objs    []client.Object
		fake    *fakeAAS
		setup   func(t *testing.T, ctx context.Context, c client.Client)
		wantErr bool
		assert  func(t *testing.T, ctx context.Context, c client.Client, f *fakeAAS, res ctrl.Result)
	}{
		{
			name: "create happy path persists identifier and Ready",
			objs: []client.Object{scalableTargetCR()},
			fake: &fakeAAS{
				registerScalableTarget: func(_ context.Context, params *awsaas.RegisterScalableTargetInput) (*awsaas.RegisterScalableTargetOutput, error) {
					if aws.ToString(params.ResourceId) != "service/my-cluster/my-service" {
						return nil, fmt.Errorf("unexpected resource ID %q", aws.ToString(params.ResourceId))
					}
					if aws.ToInt32(params.MinCapacity) != 1 || aws.ToInt32(params.MaxCapacity) != 10 {
						return nil, fmt.Errorf("unexpected capacity %d/%d", aws.ToInt32(params.MinCapacity), aws.ToInt32(params.MaxCapacity))
					}
					return &awsaas.RegisterScalableTargetOutput{ScalableTargetARN: aws.String(testScalableTargetARN)}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAAS, _ ctrl.Result) {
				got := &awsv1alpha1.ScalableTarget{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if !f.registerCalled {
					t.Error("expected RegisterScalableTarget to be called")
				}
				if got.Status.ScalableTargetARN != testScalableTargetARN {
					t.Errorf("status.scalableTargetArn = %q, want %q", got.Status.ScalableTargetARN, testScalableTargetARN)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "register passes optional roleARN and tags",
			objs: []client.Object{scalableTargetCR(func(st *awsv1alpha1.ScalableTarget) {
				st.Spec.RoleARN = "arn:aws:iam::123456789012:role/scaling-role"
				st.Spec.Tags = map[string]string{"env": "test"}
			})},
			fake: &fakeAAS{
				registerScalableTarget: func(_ context.Context, params *awsaas.RegisterScalableTargetInput) (*awsaas.RegisterScalableTargetOutput, error) {
					return &awsaas.RegisterScalableTargetOutput{ScalableTargetARN: aws.String(testScalableTargetARN)}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAAS, _ ctrl.Result) {
				if f.registerInput == nil {
					t.Fatal("expected RegisterScalableTarget to be called")
				}
				if aws.ToString(f.registerInput.RoleARN) != "arn:aws:iam::123456789012:role/scaling-role" {
					t.Errorf("RoleARN = %q", aws.ToString(f.registerInput.RoleARN))
				}
				if f.registerInput.Tags["env"] != "test" {
					t.Errorf("Tags = %v", f.registerInput.Tags)
				}
			},
		},
		{
			name: "register error sets Ready=False and returns error",
			objs: []client.Object{scalableTargetCR(func(st *awsv1alpha1.ScalableTarget) {
				st.Finalizers = []string{awsv1alpha1.FinalizerName}
			})},
			fake: &fakeAAS{
				registerScalableTarget: func(_ context.Context, _ *awsaas.RegisterScalableTargetInput) (*awsaas.RegisterScalableTargetOutput, error) {
					return nil, fmt.Errorf("throttled")
				},
			},
			wantErr: true,
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAAS, _ ctrl.Result) {
				got := &awsv1alpha1.ScalableTarget{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionFalse {
					t.Errorf("Ready condition = %+v, want False", cond)
				}
			},
		},
		{
			name: "steady state re-registers (idempotent upsert) without error",
			objs: []client.Object{scalableTargetCR(func(st *awsv1alpha1.ScalableTarget) {
				st.Finalizers = []string{awsv1alpha1.FinalizerName}
				st.Status.ScalableTargetARN = testScalableTargetARN
			})},
			fake: &fakeAAS{
				registerScalableTarget: func(_ context.Context, _ *awsaas.RegisterScalableTargetInput) (*awsaas.RegisterScalableTargetOutput, error) {
					return &awsaas.RegisterScalableTargetOutput{ScalableTargetARN: aws.String(testScalableTargetARN)}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAAS, _ ctrl.Result) {
				got := &awsv1alpha1.ScalableTarget{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "delete with finalizer deregisters using spec identity",
			objs: []client.Object{scalableTargetCR(func(st *awsv1alpha1.ScalableTarget) {
				st.Finalizers = []string{awsv1alpha1.FinalizerName}
				st.Status.ScalableTargetARN = testScalableTargetARN
			})},
			fake: &fakeAAS{
				deregisterScalableTarget: func(_ context.Context, _ *awsaas.DeregisterScalableTargetInput) (*awsaas.DeregisterScalableTargetOutput, error) {
					return &awsaas.DeregisterScalableTargetOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, scalableTargetCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAAS, _ ctrl.Result) {
				if !f.deregisterCalled {
					t.Error("expected DeregisterScalableTarget to be called")
				}
				if aws.ToString(f.deregisterInput.ResourceId) != "service/my-cluster/my-service" {
					t.Errorf("deregister resource ID = %q", aws.ToString(f.deregisterInput.ResourceId))
				}
				got := &awsv1alpha1.ScalableTarget{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "delete tolerates already-deregistered target",
			objs: []client.Object{scalableTargetCR(func(st *awsv1alpha1.ScalableTarget) {
				st.Finalizers = []string{awsv1alpha1.FinalizerName}
			})},
			fake: &fakeAAS{
				deregisterScalableTarget: func(_ context.Context, _ *awsaas.DeregisterScalableTargetInput) (*awsaas.DeregisterScalableTargetOutput, error) {
					return nil, aasNotFoundErr()
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, scalableTargetCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAAS, _ ctrl.Result) {
				got := &awsv1alpha1.ScalableTarget{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "abandon policy skips AWS deregister",
			objs: []client.Object{scalableTargetCR(func(st *awsv1alpha1.ScalableTarget) {
				st.Finalizers = []string{awsv1alpha1.FinalizerName}
				st.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			})},
			fake: &fakeAAS{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, scalableTargetCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAAS, _ ctrl.Result) {
				if f.deregisterCalled {
					t.Error("DeregisterScalableTarget must not be called when abandoning")
				}
				got := &awsv1alpha1.ScalableTarget{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := newScalingScheme(t)
			c := newScalingFakeClient(scheme, tc.objs...)
			r := &ScalableTargetReconciler{Client: c, Scheme: scheme, AASClient: tc.fake}

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
