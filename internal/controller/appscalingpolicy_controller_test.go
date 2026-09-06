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
	awsaas "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

func appScalingPolicyCR(mutate ...func(*awsv1alpha1.AppScalingPolicy)) *awsv1alpha1.AppScalingPolicy {
	sp := &awsv1alpha1.AppScalingPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "my-policy", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.AppScalingPolicySpec{
			PolicyName:        "my-policy",
			ServiceNamespace:  "ecs",
			ResourceID:        "service/my-cluster/my-service",
			ScalableDimension: "ecs:service:DesiredCount",
			PolicyType:        "TargetTrackingScaling",
			TargetTrackingConfiguration: &awsv1alpha1.AppScalingTargetTracking{
				PredefinedMetricType: "ECSServiceAverageCPUUtilization",
				TargetValue:          70,
			},
		},
	}
	for _, m := range mutate {
		m(sp)
	}
	return sp
}

func TestAppScalingPolicyReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-policy", Namespace: "default"}}
	policyARN := "arn:aws:autoscaling:us-east-1:123456789012:scalingPolicy:abc:policyName/my-policy"

	t.Run("create persists policy ARN", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, appScalingPolicyCR())
		f := &fakeAAS{
			putScalingPolicy: func(_ context.Context, params *awsaas.PutScalingPolicyInput) (*awsaas.PutScalingPolicyOutput, error) {
				if aws.ToString(params.ResourceId) != "service/my-cluster/my-service" {
					t.Errorf("resource ID = %q", aws.ToString(params.ResourceId))
				}
				if params.TargetTrackingScalingPolicyConfiguration == nil ||
					aws.ToFloat64(params.TargetTrackingScalingPolicyConfiguration.TargetValue) != 70 {
					t.Error("target tracking config not passed")
				}
				return &awsaas.PutScalingPolicyOutput{PolicyARN: aws.String(policyARN)}, nil
			},
		}
		r := &AppScalingPolicyReconciler{Client: c, Scheme: scheme, AASClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.AppScalingPolicy{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.PolicyARN != policyARN {
			t.Errorf("status.policyArn = %q, want %q", got.Status.PolicyARN, policyARN)
		}
	})

	t.Run("targetRef waits for unregistered ScalableTarget", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme,
			appScalingPolicyCR(func(sp *awsv1alpha1.AppScalingPolicy) {
				sp.Spec.TargetRef = &awsv1alpha1.ScalableTargetRef{Name: "my-target"}
				sp.Spec.ServiceNamespace = ""
				sp.Spec.ResourceID = ""
				sp.Spec.ScalableDimension = ""
			}),
			scalableTargetCR(),
		)
		f := &fakeAAS{}
		r := &AppScalingPolicyReconciler{Client: c, Scheme: scheme, AASClient: f}
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueDependency {
			t.Errorf("result = %+v, want requeueDependency", res)
		}
		if f.putCalled {
			t.Error("PutScalingPolicy must not be called while target not ready")
		}
	})

	t.Run("delete calls DeleteScalingPolicy", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, appScalingPolicyCR(func(sp *awsv1alpha1.AppScalingPolicy) {
			sp.Finalizers = []string{awsv1alpha1.FinalizerName}
			sp.Status.PolicyARN = policyARN
		}))
		f := &fakeAAS{
			deleteScalingPolicy: func(_ context.Context, params *awsaas.DeleteScalingPolicyInput) (*awsaas.DeleteScalingPolicyOutput, error) {
				if aws.ToString(params.PolicyName) != "my-policy" {
					t.Errorf("delete policy name = %q", aws.ToString(params.PolicyName))
				}
				return &awsaas.DeleteScalingPolicyOutput{}, nil
			},
		}
		r := &AppScalingPolicyReconciler{Client: c, Scheme: scheme, AASClient: f}
		if err := c.Delete(ctx, appScalingPolicyCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled {
			t.Error("expected DeleteScalingPolicy to be called")
		}
		got := &awsv1alpha1.AppScalingPolicy{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, appScalingPolicyCR(func(sp *awsv1alpha1.AppScalingPolicy) {
			sp.Finalizers = []string{awsv1alpha1.FinalizerName}
			sp.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		}))
		f := &fakeAAS{}
		r := &AppScalingPolicyReconciler{Client: c, Scheme: scheme, AASClient: f}
		if err := c.Delete(ctx, appScalingPolicyCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteScalingPolicy must not be called when abandoning")
		}
		got := &awsv1alpha1.AppScalingPolicy{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})
}
