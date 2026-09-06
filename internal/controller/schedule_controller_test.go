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
	awsscheduler "github.com/aws/aws-sdk-go-v2/service/scheduler"
	schedulertypes "github.com/aws/aws-sdk-go-v2/service/scheduler/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeSchedulerAPI struct {
	getSchedule    func(ctx context.Context, params *awsscheduler.GetScheduleInput) (*awsscheduler.GetScheduleOutput, error)
	createSchedule func(ctx context.Context, params *awsscheduler.CreateScheduleInput) (*awsscheduler.CreateScheduleOutput, error)
	updateSchedule func(ctx context.Context, params *awsscheduler.UpdateScheduleInput) (*awsscheduler.UpdateScheduleOutput, error)
	deleteSchedule func(ctx context.Context, params *awsscheduler.DeleteScheduleInput) (*awsscheduler.DeleteScheduleOutput, error)

	getScheduleGroup    func(ctx context.Context, params *awsscheduler.GetScheduleGroupInput) (*awsscheduler.GetScheduleGroupOutput, error)
	createScheduleGroup func(ctx context.Context, params *awsscheduler.CreateScheduleGroupInput) (*awsscheduler.CreateScheduleGroupOutput, error)
	deleteScheduleGroup func(ctx context.Context, params *awsscheduler.DeleteScheduleGroupInput) (*awsscheduler.DeleteScheduleGroupOutput, error)

	createCalled      bool
	createInput       *awsscheduler.CreateScheduleInput
	updateCalled      bool
	updateInput       *awsscheduler.UpdateScheduleInput
	deleteCalled      bool
	deleteInput       *awsscheduler.DeleteScheduleInput
	createGroupCalled bool
	deleteGroupCalled bool
}

func (f *fakeSchedulerAPI) GetSchedule(ctx context.Context, params *awsscheduler.GetScheduleInput, _ ...func(*awsscheduler.Options)) (*awsscheduler.GetScheduleOutput, error) {
	if f.getSchedule == nil {
		return nil, fmt.Errorf("unexpected call to GetSchedule")
	}
	return f.getSchedule(ctx, params)
}

func (f *fakeSchedulerAPI) CreateSchedule(ctx context.Context, params *awsscheduler.CreateScheduleInput, _ ...func(*awsscheduler.Options)) (*awsscheduler.CreateScheduleOutput, error) {
	f.createCalled = true
	f.createInput = params
	if f.createSchedule == nil {
		return nil, fmt.Errorf("unexpected call to CreateSchedule")
	}
	return f.createSchedule(ctx, params)
}

func (f *fakeSchedulerAPI) UpdateSchedule(ctx context.Context, params *awsscheduler.UpdateScheduleInput, _ ...func(*awsscheduler.Options)) (*awsscheduler.UpdateScheduleOutput, error) {
	f.updateCalled = true
	f.updateInput = params
	if f.updateSchedule == nil {
		return nil, fmt.Errorf("unexpected call to UpdateSchedule")
	}
	return f.updateSchedule(ctx, params)
}

func (f *fakeSchedulerAPI) DeleteSchedule(ctx context.Context, params *awsscheduler.DeleteScheduleInput, _ ...func(*awsscheduler.Options)) (*awsscheduler.DeleteScheduleOutput, error) {
	f.deleteCalled = true
	f.deleteInput = params
	if f.deleteSchedule == nil {
		return nil, fmt.Errorf("unexpected call to DeleteSchedule")
	}
	return f.deleteSchedule(ctx, params)
}

func (f *fakeSchedulerAPI) GetScheduleGroup(ctx context.Context, params *awsscheduler.GetScheduleGroupInput, _ ...func(*awsscheduler.Options)) (*awsscheduler.GetScheduleGroupOutput, error) {
	if f.getScheduleGroup == nil {
		return nil, fmt.Errorf("unexpected call to GetScheduleGroup")
	}
	return f.getScheduleGroup(ctx, params)
}

func (f *fakeSchedulerAPI) CreateScheduleGroup(ctx context.Context, params *awsscheduler.CreateScheduleGroupInput, _ ...func(*awsscheduler.Options)) (*awsscheduler.CreateScheduleGroupOutput, error) {
	f.createGroupCalled = true
	if f.createScheduleGroup == nil {
		return nil, fmt.Errorf("unexpected call to CreateScheduleGroup")
	}
	return f.createScheduleGroup(ctx, params)
}

func (f *fakeSchedulerAPI) DeleteScheduleGroup(ctx context.Context, params *awsscheduler.DeleteScheduleGroupInput, _ ...func(*awsscheduler.Options)) (*awsscheduler.DeleteScheduleGroupOutput, error) {
	f.deleteGroupCalled = true
	if f.deleteScheduleGroup == nil {
		return nil, fmt.Errorf("unexpected call to DeleteScheduleGroup")
	}
	return f.deleteScheduleGroup(ctx, params)
}

func schedulerNotFoundErr() error {
	return &schedulertypes.ResourceNotFoundException{Message: aws.String("not found")}
}

const (
	testScheduleARN      = "arn:aws:scheduler:us-east-1:123456789012:schedule/default/my-schedule"
	testSchedulerRoleARN = "arn:aws:iam::123456789012:role/scheduler-role"
)

func scheduleCR(mutate ...func(*awsv1alpha1.Schedule)) *awsv1alpha1.Schedule {
	s := &awsv1alpha1.Schedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-schedule",
			Namespace:  "default",
			Finalizers: []string{awsv1alpha1.FinalizerName},
		},
		Spec: awsv1alpha1.ScheduleSpec{
			Name:               "my-schedule",
			ScheduleExpression: "rate(5 minutes)",
			FlexibleTimeWindow: awsv1alpha1.ScheduleFlexibleTimeWindow{Mode: "OFF"},
			Target: awsv1alpha1.ScheduleTarget{
				ARN:     "arn:aws:lambda:us-east-1:123456789012:function:my-fn",
				RoleRef: awsv1alpha1.RoleRef{ARN: testSchedulerRoleARN},
			},
		},
	}
	for _, m := range mutate {
		m(s)
	}
	return s
}

func TestScheduleReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-schedule", Namespace: "default"}}

	tests := []struct {
		name    string
		objs    []client.Object
		fake    *fakeSchedulerAPI
		setup   func(t *testing.T, ctx context.Context, c client.Client)
		wantErr bool
		assert  func(t *testing.T, ctx context.Context, c client.Client, f *fakeSchedulerAPI, res ctrl.Result)
	}{
		{
			name: "create happy path persists ARN and Ready",
			objs: []client.Object{scheduleCR()},
			fake: &fakeSchedulerAPI{
				getSchedule: func(_ context.Context, _ *awsscheduler.GetScheduleInput) (*awsscheduler.GetScheduleOutput, error) {
					return nil, schedulerNotFoundErr()
				},
				createSchedule: func(_ context.Context, params *awsscheduler.CreateScheduleInput) (*awsscheduler.CreateScheduleOutput, error) {
					if aws.ToString(params.Name) != "my-schedule" {
						return nil, fmt.Errorf("unexpected name %q", aws.ToString(params.Name))
					}
					if params.State != schedulertypes.ScheduleStateEnabled {
						return nil, fmt.Errorf("expected default ENABLED state, got %q", params.State)
					}
					if params.Target == nil || aws.ToString(params.Target.RoleArn) != testSchedulerRoleARN {
						return nil, fmt.Errorf("target role not resolved")
					}
					return &awsscheduler.CreateScheduleOutput{ScheduleArn: aws.String(testScheduleARN)}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSchedulerAPI, _ ctrl.Result) {
				got := &awsv1alpha1.Schedule{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if !f.createCalled {
					t.Error("expected CreateSchedule to be called")
				}
				if got.Status.ARN != testScheduleARN {
					t.Errorf("status.arn = %q, want %q", got.Status.ARN, testScheduleARN)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "create with flexible window, retry policy and DLQ",
			objs: []client.Object{scheduleCR(func(s *awsv1alpha1.Schedule) {
				s.Spec.FlexibleTimeWindow = awsv1alpha1.ScheduleFlexibleTimeWindow{
					Mode:                   "FLEXIBLE",
					MaximumWindowInMinutes: aws.Int32(15),
				}
				s.Spec.Timezone = "America/New_York"
				s.Spec.Target.Input = `{"key":"value"}`
				s.Spec.Target.RetryPolicy = &awsv1alpha1.ScheduleRetryPolicy{
					MaximumRetryAttempts:     aws.Int32(3),
					MaximumEventAgeInSeconds: aws.Int32(3600),
				}
				s.Spec.Target.DeadLetterARN = "arn:aws:sqs:us-east-1:123456789012:dlq"
				s.Spec.State = "DISABLED"
			})},
			fake: &fakeSchedulerAPI{
				getSchedule: func(_ context.Context, _ *awsscheduler.GetScheduleInput) (*awsscheduler.GetScheduleOutput, error) {
					return nil, schedulerNotFoundErr()
				},
				createSchedule: func(_ context.Context, _ *awsscheduler.CreateScheduleInput) (*awsscheduler.CreateScheduleOutput, error) {
					return &awsscheduler.CreateScheduleOutput{ScheduleArn: aws.String(testScheduleARN)}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSchedulerAPI, _ ctrl.Result) {
				in := f.createInput
				if in == nil {
					t.Fatal("expected CreateSchedule to be called")
				}
				if in.FlexibleTimeWindow.Mode != schedulertypes.FlexibleTimeWindowModeFlexible {
					t.Errorf("mode = %q", in.FlexibleTimeWindow.Mode)
				}
				if aws.ToInt32(in.FlexibleTimeWindow.MaximumWindowInMinutes) != 15 {
					t.Errorf("window = %d", aws.ToInt32(in.FlexibleTimeWindow.MaximumWindowInMinutes))
				}
				if aws.ToString(in.ScheduleExpressionTimezone) != "America/New_York" {
					t.Errorf("timezone = %q", aws.ToString(in.ScheduleExpressionTimezone))
				}
				if in.State != schedulertypes.ScheduleStateDisabled {
					t.Errorf("state = %q", in.State)
				}
				if aws.ToString(in.Target.Input) != `{"key":"value"}` {
					t.Errorf("input = %q", aws.ToString(in.Target.Input))
				}
				if in.Target.RetryPolicy == nil || aws.ToInt32(in.Target.RetryPolicy.MaximumRetryAttempts) != 3 {
					t.Error("retry policy not passed")
				}
				if in.Target.DeadLetterConfig == nil || aws.ToString(in.Target.DeadLetterConfig.Arn) != "arn:aws:sqs:us-east-1:123456789012:dlq" {
					t.Error("dead letter config not passed")
				}
			},
		},
		{
			name: "waits for group ref without ARN",
			objs: []client.Object{
				scheduleCR(func(s *awsv1alpha1.Schedule) {
					s.Spec.GroupRef = &awsv1alpha1.ScheduleGroupRef{Name: "my-group"}
				}),
				&awsv1alpha1.ScheduleGroup{
					ObjectMeta: metav1.ObjectMeta{Name: "my-group", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
					Spec:       awsv1alpha1.ScheduleGroupSpec{Name: "my-group"},
				},
			},
			fake: &fakeSchedulerAPI{},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSchedulerAPI, res ctrl.Result) {
				if res != requeueDependency {
					t.Errorf("result = %+v, want requeueDependency %+v", res, requeueDependency)
				}
				if f.createCalled {
					t.Error("CreateSchedule must not be called while group is not ready")
				}
			},
		},
		{
			name: "resolves group ref with ARN into create input",
			objs: []client.Object{
				scheduleCR(func(s *awsv1alpha1.Schedule) {
					s.Spec.GroupRef = &awsv1alpha1.ScheduleGroupRef{Name: "my-group"}
				}),
				&awsv1alpha1.ScheduleGroup{
					ObjectMeta: metav1.ObjectMeta{Name: "my-group", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
					Spec:       awsv1alpha1.ScheduleGroupSpec{Name: "my-aws-group"},
					Status: awsv1alpha1.ScheduleGroupStatus{
						ARN: "arn:aws:scheduler:us-east-1:123456789012:schedule-group/my-aws-group",
					},
				},
			},
			fake: &fakeSchedulerAPI{
				getSchedule: func(_ context.Context, params *awsscheduler.GetScheduleInput) (*awsscheduler.GetScheduleOutput, error) {
					if aws.ToString(params.GroupName) != "my-aws-group" {
						return nil, fmt.Errorf("unexpected group %q", aws.ToString(params.GroupName))
					}
					return nil, schedulerNotFoundErr()
				},
				createSchedule: func(_ context.Context, params *awsscheduler.CreateScheduleInput) (*awsscheduler.CreateScheduleOutput, error) {
					if aws.ToString(params.GroupName) != "my-aws-group" {
						return nil, fmt.Errorf("unexpected group %q", aws.ToString(params.GroupName))
					}
					return &awsscheduler.CreateScheduleOutput{ScheduleArn: aws.String(testScheduleARN)}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSchedulerAPI, _ ctrl.Result) {
				if !f.createCalled {
					t.Error("expected CreateSchedule to be called")
				}
			},
		},
		{
			name: "spec change triggers UpdateSchedule",
			objs: []client.Object{scheduleCR(func(s *awsv1alpha1.Schedule) {
				s.Finalizers = []string{awsv1alpha1.FinalizerName}
				s.Generation = 2
				s.Status.ARN = testScheduleARN
				s.Status.ObservedGeneration = 1
			})},
			fake: &fakeSchedulerAPI{
				getSchedule: func(_ context.Context, _ *awsscheduler.GetScheduleInput) (*awsscheduler.GetScheduleOutput, error) {
					return &awsscheduler.GetScheduleOutput{
						Arn:  aws.String(testScheduleARN),
						Name: aws.String("my-schedule"),
					}, nil
				},
				updateSchedule: func(_ context.Context, params *awsscheduler.UpdateScheduleInput) (*awsscheduler.UpdateScheduleOutput, error) {
					if aws.ToString(params.ScheduleExpression) != "rate(5 minutes)" {
						return nil, fmt.Errorf("unexpected expression")
					}
					return &awsscheduler.UpdateScheduleOutput{ScheduleArn: aws.String(testScheduleARN)}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSchedulerAPI, _ ctrl.Result) {
				if f.createCalled {
					t.Error("CreateSchedule must not be called when the schedule exists")
				}
				if !f.updateCalled {
					t.Error("expected UpdateSchedule to be called for changed generation")
				}
				got := &awsv1alpha1.Schedule{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.ObservedGeneration != got.Generation {
					t.Errorf("observedGeneration = %d, want %d", got.Status.ObservedGeneration, got.Generation)
				}
			},
		},
		{
			name: "steady state does not create or update",
			objs: []client.Object{scheduleCR(func(s *awsv1alpha1.Schedule) {
				s.Finalizers = []string{awsv1alpha1.FinalizerName}
				s.Generation = 1
				s.Status.ARN = testScheduleARN
				s.Status.ObservedGeneration = 1
			})},
			fake: &fakeSchedulerAPI{
				getSchedule: func(_ context.Context, _ *awsscheduler.GetScheduleInput) (*awsscheduler.GetScheduleOutput, error) {
					return &awsscheduler.GetScheduleOutput{
						Arn:  aws.String(testScheduleARN),
						Name: aws.String("my-schedule"),
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSchedulerAPI, _ ctrl.Result) {
				if f.createCalled || f.updateCalled {
					t.Error("no mutating call expected in steady state")
				}
				got := &awsv1alpha1.Schedule{}
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
			name: "identifier persisted even when later status write fails is covered by create path",
			objs: []client.Object{scheduleCR()},
			fake: &fakeSchedulerAPI{
				getSchedule: func(_ context.Context, _ *awsscheduler.GetScheduleInput) (*awsscheduler.GetScheduleOutput, error) {
					return nil, schedulerNotFoundErr()
				},
				createSchedule: func(_ context.Context, _ *awsscheduler.CreateScheduleInput) (*awsscheduler.CreateScheduleOutput, error) {
					return &awsscheduler.CreateScheduleOutput{ScheduleArn: aws.String(testScheduleARN)}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSchedulerAPI, _ ctrl.Result) {
				got := &awsv1alpha1.Schedule{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.ARN != testScheduleARN {
					t.Errorf("status.arn = %q, want persisted %q", got.Status.ARN, testScheduleARN)
				}
			},
		},
		{
			name: "delete with finalizer calls AWS delete with group name",
			objs: []client.Object{scheduleCR(func(s *awsv1alpha1.Schedule) {
				s.Finalizers = []string{awsv1alpha1.FinalizerName}
				s.Spec.GroupRef = &awsv1alpha1.ScheduleGroupRef{GroupName: "my-aws-group"}
				s.Status.ARN = testScheduleARN
			})},
			fake: &fakeSchedulerAPI{
				deleteSchedule: func(_ context.Context, _ *awsscheduler.DeleteScheduleInput) (*awsscheduler.DeleteScheduleOutput, error) {
					return &awsscheduler.DeleteScheduleOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, scheduleCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSchedulerAPI, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeleteSchedule to be called")
				}
				if aws.ToString(f.deleteInput.Name) != "my-schedule" {
					t.Errorf("delete name = %q", aws.ToString(f.deleteInput.Name))
				}
				if aws.ToString(f.deleteInput.GroupName) != "my-aws-group" {
					t.Errorf("delete group = %q", aws.ToString(f.deleteInput.GroupName))
				}
				got := &awsv1alpha1.Schedule{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "delete tolerates already-deleted schedule",
			objs: []client.Object{scheduleCR(func(s *awsv1alpha1.Schedule) {
				s.Finalizers = []string{awsv1alpha1.FinalizerName}
			})},
			fake: &fakeSchedulerAPI{
				deleteSchedule: func(_ context.Context, _ *awsscheduler.DeleteScheduleInput) (*awsscheduler.DeleteScheduleOutput, error) {
					return nil, schedulerNotFoundErr()
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, scheduleCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSchedulerAPI, _ ctrl.Result) {
				got := &awsv1alpha1.Schedule{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "abandon policy skips AWS delete",
			objs: []client.Object{scheduleCR(func(s *awsv1alpha1.Schedule) {
				s.Finalizers = []string{awsv1alpha1.FinalizerName}
				s.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				s.Status.ARN = testScheduleARN
			})},
			fake: &fakeSchedulerAPI{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, scheduleCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSchedulerAPI, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeleteSchedule must not be called when abandoning")
				}
				got := &awsv1alpha1.Schedule{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "role ref by CR name waits for IAMRole without ARN",
			objs: []client.Object{
				scheduleCR(func(s *awsv1alpha1.Schedule) {
					s.Spec.Target.RoleRef = awsv1alpha1.RoleRef{Name: "my-role"}
				}),
				&awsv1alpha1.IAMRole{
					ObjectMeta: metav1.ObjectMeta{Name: "my-role", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
					Spec:       awsv1alpha1.IAMRoleSpec{RoleName: "my-role"},
				},
			},
			fake: &fakeSchedulerAPI{},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSchedulerAPI, res ctrl.Result) {
				if res != requeueDependency {
					t.Errorf("result = %+v, want requeueDependency %+v", res, requeueDependency)
				}
				if f.createCalled {
					t.Error("CreateSchedule must not be called while role is not ready")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := newScalingScheme(t)
			c := newScalingFakeClient(scheme, tc.objs...)
			r := &ScheduleReconciler{Client: c, Scheme: scheme, SchedulerClient: tc.fake}

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
