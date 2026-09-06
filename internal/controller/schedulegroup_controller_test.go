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
	awsscheduler "github.com/aws/aws-sdk-go-v2/service/scheduler"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

func scheduleGroupCR(mutate ...func(*awsv1alpha1.ScheduleGroup)) *awsv1alpha1.ScheduleGroup {
	sg := &awsv1alpha1.ScheduleGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "my-group", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec:       awsv1alpha1.ScheduleGroupSpec{Name: "my-group", Tags: map[string]string{"env": "test"}},
	}
	for _, m := range mutate {
		m(sg)
	}
	return sg
}

func TestScheduleGroupReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-group", Namespace: "default"}}
	groupARN := "arn:aws:scheduler:us-east-1:123456789012:schedule-group/my-group"

	t.Run("create persists ARN", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, scheduleGroupCR())
		f := &fakeSchedulerAPI{
			getScheduleGroup: func(_ context.Context, _ *awsscheduler.GetScheduleGroupInput) (*awsscheduler.GetScheduleGroupOutput, error) {
				return nil, schedulerNotFoundErr()
			},
			createScheduleGroup: func(_ context.Context, params *awsscheduler.CreateScheduleGroupInput) (*awsscheduler.CreateScheduleGroupOutput, error) {
				if aws.ToString(params.Name) != "my-group" {
					t.Errorf("group name = %q", aws.ToString(params.Name))
				}
				if len(params.Tags) != 1 {
					t.Errorf("tags = %v", params.Tags)
				}
				return &awsscheduler.CreateScheduleGroupOutput{ScheduleGroupArn: aws.String(groupARN)}, nil
			},
		}
		r := &ScheduleGroupReconciler{Client: c, Scheme: scheme, SchedulerClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.ScheduleGroup{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ARN != groupARN {
			t.Errorf("status.arn = %q, want %q", got.Status.ARN, groupARN)
		}
	})

	t.Run("delete calls DeleteScheduleGroup", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, scheduleGroupCR(func(sg *awsv1alpha1.ScheduleGroup) {
			sg.Finalizers = []string{awsv1alpha1.FinalizerName}
			sg.Status.ARN = groupARN
		}))
		f := &fakeSchedulerAPI{
			deleteScheduleGroup: func(_ context.Context, params *awsscheduler.DeleteScheduleGroupInput) (*awsscheduler.DeleteScheduleGroupOutput, error) {
				if aws.ToString(params.Name) != "my-group" {
					t.Errorf("delete group name = %q", aws.ToString(params.Name))
				}
				return &awsscheduler.DeleteScheduleGroupOutput{}, nil
			},
		}
		r := &ScheduleGroupReconciler{Client: c, Scheme: scheme, SchedulerClient: f}
		if err := c.Delete(ctx, scheduleGroupCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteGroupCalled {
			t.Error("expected DeleteScheduleGroup to be called")
		}
		got := &awsv1alpha1.ScheduleGroup{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, scheduleGroupCR(func(sg *awsv1alpha1.ScheduleGroup) {
			sg.Finalizers = []string{awsv1alpha1.FinalizerName}
			sg.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		}))
		f := &fakeSchedulerAPI{}
		r := &ScheduleGroupReconciler{Client: c, Scheme: scheme, SchedulerClient: f}
		if err := c.Delete(ctx, scheduleGroupCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteGroupCalled {
			t.Error("DeleteScheduleGroup must not be called when abandoning")
		}
		got := &awsv1alpha1.ScheduleGroup{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})
}
