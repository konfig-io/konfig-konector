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
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// fakeECSServiceAPI implements ECSServiceAWSAPI with overridable function fields.
type fakeECSServiceAPI struct {
	createCalls int
	updateCalls int
	deleteCalls int

	describeServices func(*awsecs.DescribeServicesInput) (*awsecs.DescribeServicesOutput, error)
	createService    func(*awsecs.CreateServiceInput) (*awsecs.CreateServiceOutput, error)
	updateService    func(*awsecs.UpdateServiceInput) (*awsecs.UpdateServiceOutput, error)
	deleteService    func(*awsecs.DeleteServiceInput) (*awsecs.DeleteServiceOutput, error)
}

func (f *fakeECSServiceAPI) DescribeServices(_ context.Context, in *awsecs.DescribeServicesInput, _ ...func(*awsecs.Options)) (*awsecs.DescribeServicesOutput, error) {
	if f.describeServices == nil {
		return &awsecs.DescribeServicesOutput{}, nil
	}
	return f.describeServices(in)
}

func (f *fakeECSServiceAPI) CreateService(_ context.Context, in *awsecs.CreateServiceInput, _ ...func(*awsecs.Options)) (*awsecs.CreateServiceOutput, error) {
	f.createCalls++
	if f.createService == nil {
		return &awsecs.CreateServiceOutput{Service: &ecstypes.Service{}}, nil
	}
	return f.createService(in)
}

func (f *fakeECSServiceAPI) UpdateService(_ context.Context, in *awsecs.UpdateServiceInput, _ ...func(*awsecs.Options)) (*awsecs.UpdateServiceOutput, error) {
	f.updateCalls++
	if f.updateService == nil {
		return &awsecs.UpdateServiceOutput{}, nil
	}
	return f.updateService(in)
}

func (f *fakeECSServiceAPI) DeleteService(_ context.Context, in *awsecs.DeleteServiceInput, _ ...func(*awsecs.Options)) (*awsecs.DeleteServiceOutput, error) {
	f.deleteCalls++
	if f.deleteService == nil {
		return &awsecs.DeleteServiceOutput{}, nil
	}
	return f.deleteService(in)
}

const ecsSvcTestARN = "arn:aws:ecs:us-east-1:123456789012:service/prod-cluster/web-svc"

func ecsSvcTestClient(s *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&awsv1alpha1.ECSService{}, &awsv1alpha1.ECSCluster{}).
		Build()
}

func TestECSServiceReconcile(t *testing.T) {
	ctx := context.Background()
	nn := k8stypes.NamespacedName{Name: "test-ecssvc", Namespace: "default"}
	req := ctrl.Request{NamespacedName: nn}

	newSvc := func(mutate ...func(*awsv1alpha1.ECSService)) *awsv1alpha1.ECSService {
		svc := &awsv1alpha1.ECSService{
			ObjectMeta: metav1.ObjectMeta{Name: nn.Name, Namespace: nn.Namespace, Finalizers: []string{awsv1alpha1.FinalizerName}, Generation: 1},
			Spec: awsv1alpha1.ECSServiceSpec{
				ClusterName:       "prod-cluster",
				ServiceName:       "web-svc",
				TaskDefinitionArn: "arn:aws:ecs:us-east-1:123456789012:task-definition/web:3",
				DesiredCount:      2,
				LaunchType:        "FARGATE",
			},
		}
		for _, m := range mutate {
			m(svc)
		}
		return svc
	}

	t.Run("create happy path persists ARN and Ready", func(t *testing.T) {
		s := vpcTestScheme(t)
		cl := ecsSvcTestClient(s, newSvc())
		f := &fakeECSServiceAPI{
			createService: func(in *awsecs.CreateServiceInput) (*awsecs.CreateServiceOutput, error) {
				if aws.ToString(in.Cluster) != "prod-cluster" {
					t.Errorf("CreateService cluster = %q, want prod-cluster", aws.ToString(in.Cluster))
				}
				if aws.ToString(in.ServiceName) != "web-svc" {
					t.Errorf("CreateService name = %q, want web-svc", aws.ToString(in.ServiceName))
				}
				if aws.ToInt32(in.DesiredCount) != 2 {
					t.Errorf("CreateService desiredCount = %d, want 2", aws.ToInt32(in.DesiredCount))
				}
				return &awsecs.CreateServiceOutput{Service: &ecstypes.Service{
					ServiceArn:   aws.String(ecsSvcTestARN),
					Status:       aws.String("ACTIVE"),
					RunningCount: 0,
					PendingCount: 2,
				}}, nil
			},
		}
		r := &ECSServiceReconciler{Client: cl, Scheme: s, ECSClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		got := &awsv1alpha1.ECSService{}
		if err := cl.Get(ctx, nn, got); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if f.createCalls != 1 {
			t.Errorf("CreateService calls = %d, want 1", f.createCalls)
		}
		if got.Status.ServiceARN != ecsSvcTestARN {
			t.Errorf("status.serviceArn = %q, want %q", got.Status.ServiceARN, ecsSvcTestARN)
		}
		cond := meta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
	})

	t.Run("steady state does not call create or update", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newSvc(func(svc *awsv1alpha1.ECSService) {
			svc.Finalizers = []string{awsv1alpha1.FinalizerName}
			svc.Generation = 2
			svc.Status.ServiceARN = ecsSvcTestARN
			svc.Status.ObservedGeneration = 2
		})
		cl := ecsSvcTestClient(s, existing)
		f := &fakeECSServiceAPI{
			describeServices: func(*awsecs.DescribeServicesInput) (*awsecs.DescribeServicesOutput, error) {
				return &awsecs.DescribeServicesOutput{Services: []ecstypes.Service{{
					ServiceArn:   aws.String(ecsSvcTestARN),
					ServiceName:  aws.String("web-svc"),
					Status:       aws.String("ACTIVE"),
					RunningCount: 2,
				}}}, nil
			},
		}
		r := &ECSServiceReconciler{Client: cl, Scheme: s, ECSClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if f.createCalls != 0 {
			t.Errorf("CreateService calls = %d, want 0", f.createCalls)
		}
		if f.updateCalls != 0 {
			t.Errorf("UpdateService calls = %d, want 0 (ObservedGeneration == Generation)", f.updateCalls)
		}
	})

	t.Run("generation bump triggers UpdateService", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newSvc(func(svc *awsv1alpha1.ECSService) {
			svc.Finalizers = []string{awsv1alpha1.FinalizerName}
			svc.Generation = 3
			svc.Spec.DesiredCount = 5
			svc.Status.ServiceARN = ecsSvcTestARN
			svc.Status.ObservedGeneration = 2
		})
		cl := ecsSvcTestClient(s, existing)
		var gotCount int32
		f := &fakeECSServiceAPI{
			describeServices: func(*awsecs.DescribeServicesInput) (*awsecs.DescribeServicesOutput, error) {
				return &awsecs.DescribeServicesOutput{Services: []ecstypes.Service{{
					ServiceArn:  aws.String(ecsSvcTestARN),
					ServiceName: aws.String("web-svc"),
					Status:      aws.String("ACTIVE"),
				}}}, nil
			},
			updateService: func(in *awsecs.UpdateServiceInput) (*awsecs.UpdateServiceOutput, error) {
				gotCount = aws.ToInt32(in.DesiredCount)
				return &awsecs.UpdateServiceOutput{}, nil
			},
		}
		r := &ECSServiceReconciler{Client: cl, Scheme: s, ECSClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if f.updateCalls != 1 {
			t.Fatalf("UpdateService calls = %d, want 1", f.updateCalls)
		}
		if gotCount != 5 {
			t.Errorf("UpdateService desiredCount = %d, want 5", gotCount)
		}
		if f.createCalls != 0 {
			t.Errorf("CreateService calls = %d, want 0", f.createCalls)
		}
	})

	t.Run("delete with finalizer calls DeleteService with cluster and name", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newSvc(func(svc *awsv1alpha1.ECSService) {
			svc.Finalizers = []string{awsv1alpha1.FinalizerName}
			svc.Status.ServiceARN = ecsSvcTestARN
		})
		cl := ecsSvcTestClient(s, existing)
		var gotCluster, gotService string
		f := &fakeECSServiceAPI{
			deleteService: func(in *awsecs.DeleteServiceInput) (*awsecs.DeleteServiceOutput, error) {
				gotCluster = aws.ToString(in.Cluster)
				gotService = aws.ToString(in.Service)
				return &awsecs.DeleteServiceOutput{}, nil
			},
		}
		r := &ECSServiceReconciler{Client: cl, Scheme: s, ECSClient: f}

		if err := cl.Delete(ctx, existing); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if gotCluster != "prod-cluster" || gotService != "web-svc" {
			t.Errorf("DeleteService cluster/service = %q/%q, want prod-cluster/web-svc", gotCluster, gotService)
		}
		if err := cl.Get(ctx, nn, &awsv1alpha1.ECSService{}); !apierrors.IsNotFound(err) {
			t.Errorf("Get after finalize = %v, want NotFound", err)
		}
	})

	t.Run("abandon annotation skips AWS delete and removes finalizer", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newSvc(func(svc *awsv1alpha1.ECSService) {
			svc.Finalizers = []string{awsv1alpha1.FinalizerName}
			svc.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			svc.Status.ServiceARN = ecsSvcTestARN
		})
		cl := ecsSvcTestClient(s, existing)
		f := &fakeECSServiceAPI{}
		r := &ECSServiceReconciler{Client: cl, Scheme: s, ECSClient: f}

		if err := cl.Delete(ctx, existing); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if f.deleteCalls != 0 {
			t.Errorf("DeleteService calls = %d, want 0", f.deleteCalls)
		}
		if err := cl.Get(ctx, nn, &awsv1alpha1.ECSService{}); !apierrors.IsNotFound(err) {
			t.Errorf("Get after finalize = %v, want NotFound", err)
		}
	})

	t.Run("ClusterRef without status ARN yields requeueDependency, no error", func(t *testing.T) {
		s := vpcTestScheme(t)
		cluster := &awsv1alpha1.ECSCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "my-cluster", Namespace: nn.Namespace, Finalizers: []string{awsv1alpha1.FinalizerName}},
			// Status.ClusterARN intentionally empty: not yet synced to AWS.
		}
		svc := newSvc(func(svc *awsv1alpha1.ECSService) {
			svc.Spec.ClusterName = ""
			svc.Spec.ClusterRef = &awsv1alpha1.ECSClusterRef{Name: "my-cluster"}
		})
		cl := ecsSvcTestClient(s, svc, cluster)
		f := &fakeECSServiceAPI{}
		r := &ECSServiceReconciler{Client: cl, Scheme: s, ECSClient: f}

		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("Reconcile: want nil error for dependency wait, got %v", err)
		}
		if res != requeueDependency {
			t.Errorf("Reconcile result = %+v, want requeueDependency %+v", res, requeueDependency)
		}
		if f.createCalls != 0 {
			t.Errorf("CreateService calls = %d, want 0 while dependency not ready", f.createCalls)
		}
	})
}
