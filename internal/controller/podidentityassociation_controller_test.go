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
	awseks "github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	corev1 "k8s.io/api/core/v1"
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

// fakePodIdentityAPI implements PodIdentityAssociationAWSAPI via function fields.
type fakePodIdentityAPI struct {
	ListPodIdentityAssociationsFn    func(ctx context.Context, params *awseks.ListPodIdentityAssociationsInput, optFns ...func(*awseks.Options)) (*awseks.ListPodIdentityAssociationsOutput, error)
	DescribePodIdentityAssociationFn func(ctx context.Context, params *awseks.DescribePodIdentityAssociationInput, optFns ...func(*awseks.Options)) (*awseks.DescribePodIdentityAssociationOutput, error)
	CreatePodIdentityAssociationFn   func(ctx context.Context, params *awseks.CreatePodIdentityAssociationInput, optFns ...func(*awseks.Options)) (*awseks.CreatePodIdentityAssociationOutput, error)
	UpdatePodIdentityAssociationFn   func(ctx context.Context, params *awseks.UpdatePodIdentityAssociationInput, optFns ...func(*awseks.Options)) (*awseks.UpdatePodIdentityAssociationOutput, error)
	DeletePodIdentityAssociationFn   func(ctx context.Context, params *awseks.DeletePodIdentityAssociationInput, optFns ...func(*awseks.Options)) (*awseks.DeletePodIdentityAssociationOutput, error)
}

func (f *fakePodIdentityAPI) ListPodIdentityAssociations(ctx context.Context, params *awseks.ListPodIdentityAssociationsInput, optFns ...func(*awseks.Options)) (*awseks.ListPodIdentityAssociationsOutput, error) {
	if f.ListPodIdentityAssociationsFn != nil {
		return f.ListPodIdentityAssociationsFn(ctx, params, optFns...)
	}
	return &awseks.ListPodIdentityAssociationsOutput{}, nil
}

func (f *fakePodIdentityAPI) DescribePodIdentityAssociation(ctx context.Context, params *awseks.DescribePodIdentityAssociationInput, optFns ...func(*awseks.Options)) (*awseks.DescribePodIdentityAssociationOutput, error) {
	if f.DescribePodIdentityAssociationFn != nil {
		return f.DescribePodIdentityAssociationFn(ctx, params, optFns...)
	}
	return nil, &ekstypes.ResourceNotFoundException{}
}

func (f *fakePodIdentityAPI) CreatePodIdentityAssociation(ctx context.Context, params *awseks.CreatePodIdentityAssociationInput, optFns ...func(*awseks.Options)) (*awseks.CreatePodIdentityAssociationOutput, error) {
	if f.CreatePodIdentityAssociationFn != nil {
		return f.CreatePodIdentityAssociationFn(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("unexpected CreatePodIdentityAssociation call")
}

func (f *fakePodIdentityAPI) UpdatePodIdentityAssociation(ctx context.Context, params *awseks.UpdatePodIdentityAssociationInput, optFns ...func(*awseks.Options)) (*awseks.UpdatePodIdentityAssociationOutput, error) {
	if f.UpdatePodIdentityAssociationFn != nil {
		return f.UpdatePodIdentityAssociationFn(ctx, params, optFns...)
	}
	return &awseks.UpdatePodIdentityAssociationOutput{}, nil
}

func (f *fakePodIdentityAPI) DeletePodIdentityAssociation(ctx context.Context, params *awseks.DeletePodIdentityAssociationInput, optFns ...func(*awseks.Options)) (*awseks.DeletePodIdentityAssociationOutput, error) {
	if f.DeletePodIdentityAssociationFn != nil {
		return f.DeletePodIdentityAssociationFn(ctx, params, optFns...)
	}
	return &awseks.DeletePodIdentityAssociationOutput{}, nil
}

const (
	testAssocID  = "a-1234567890"
	testAssocARN = "arn:aws:eks:us-east-1:123456789012:podidentityassociation/my-cluster/a-1234567890"
)

func newTestPIA(mutators ...func(*awsv1alpha1.PodIdentityAssociation)) *awsv1alpha1.PodIdentityAssociation {
	pia := &awsv1alpha1.PodIdentityAssociation{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-pia",
			Namespace:  "default",
			Finalizers: []string{awsv1alpha1.FinalizerName},
		},
		Spec: awsv1alpha1.PodIdentityAssociationSpec{
			ClusterName:        "my-cluster",
			TargetNamespace:    "workload-ns",
			ServiceAccountName: "workload-sa",
			RoleRef:            awsv1alpha1.RoleRef{Name: "test-role"},
		},
	}
	for _, m := range mutators {
		m(pia)
	}
	return pia
}

func newPIAFakeClient(s *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&awsv1alpha1.PodIdentityAssociation{}, &awsv1alpha1.IAMRole{}).
		Build()
}

func piaTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := iamRoleTestScheme(t)
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	return s
}

func TestPodIdentityAssociationCreateHappyPath(t *testing.T) {
	s := piaTestScheme(t)
	pia := newTestPIA()
	cl := newPIAFakeClient(s, pia, readyIAMRole())

	var created *awseks.CreatePodIdentityAssociationInput
	fakeAWS := &fakePodIdentityAPI{
		CreatePodIdentityAssociationFn: func(_ context.Context, params *awseks.CreatePodIdentityAssociationInput, _ ...func(*awseks.Options)) (*awseks.CreatePodIdentityAssociationOutput, error) {
			created = params
			return &awseks.CreatePodIdentityAssociationOutput{Association: &ekstypes.PodIdentityAssociation{
				AssociationId:  aws.String(testAssocID),
				AssociationArn: aws.String(testAssocARN),
			}}, nil
		},
	}
	r := &PodIdentityAssociationReconciler{Client: cl, Scheme: s, EKSClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-pia", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if created == nil {
		t.Fatal("expected CreatePodIdentityAssociation to be called")
	}
	if aws.ToString(created.ClusterName) != "my-cluster" ||
		aws.ToString(created.Namespace) != "workload-ns" ||
		aws.ToString(created.ServiceAccount) != "workload-sa" ||
		aws.ToString(created.RoleArn) != testRoleARN {
		t.Errorf("CreatePodIdentityAssociation input = %+v", created)
	}

	got := &awsv1alpha1.PodIdentityAssociation{}
	if err := cl.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.AssociationID != testAssocID || got.Status.AssociationARN != testAssocARN {
		t.Errorf("status IDs = %q/%q", got.Status.AssociationID, got.Status.AssociationARN)
	}
	if got.Status.RoleARN != testRoleARN {
		t.Errorf("status RoleARN = %q", got.Status.RoleARN)
	}
	cond := meta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Errorf("Ready condition = %+v, want True", cond)
	}
}

func TestPodIdentityAssociationAdoptExisting(t *testing.T) {
	// State was lost (no AssociationID in status) but the association exists in
	// AWS: it must be adopted via ListPodIdentityAssociations, not re-created.
	s := piaTestScheme(t)
	pia := newTestPIA(func(p *awsv1alpha1.PodIdentityAssociation) {
		p.Finalizers = []string{awsv1alpha1.FinalizerName}
	})
	cl := newPIAFakeClient(s, pia, readyIAMRole())

	createCalls := 0
	fakeAWS := &fakePodIdentityAPI{
		ListPodIdentityAssociationsFn: func(_ context.Context, _ *awseks.ListPodIdentityAssociationsInput, _ ...func(*awseks.Options)) (*awseks.ListPodIdentityAssociationsOutput, error) {
			return &awseks.ListPodIdentityAssociationsOutput{
				Associations: []ekstypes.PodIdentityAssociationSummary{{
					AssociationId:  aws.String(testAssocID),
					AssociationArn: aws.String(testAssocARN),
					Namespace:      aws.String("workload-ns"),
					ServiceAccount: aws.String("workload-sa"),
				}},
			}, nil
		},
		CreatePodIdentityAssociationFn: func(_ context.Context, _ *awseks.CreatePodIdentityAssociationInput, _ ...func(*awseks.Options)) (*awseks.CreatePodIdentityAssociationOutput, error) {
			createCalls++
			return nil, fmt.Errorf("should not be called")
		},
	}
	r := &PodIdentityAssociationReconciler{Client: cl, Scheme: s, EKSClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-pia", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if createCalls != 0 {
		t.Errorf("CreatePodIdentityAssociation called %d times, want 0 (adopted)", createCalls)
	}

	got := &awsv1alpha1.PodIdentityAssociation{}
	if err := cl.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.AssociationID != testAssocID {
		t.Errorf("status AssociationID = %q", got.Status.AssociationID)
	}
}

func TestPodIdentityAssociationSteadyStateNoCreate(t *testing.T) {
	s := piaTestScheme(t)
	pia := newTestPIA(func(p *awsv1alpha1.PodIdentityAssociation) {
		p.Finalizers = []string{awsv1alpha1.FinalizerName}
		p.Status.AssociationID = testAssocID
		p.Status.AssociationARN = testAssocARN
	})
	cl := newPIAFakeClient(s, pia, readyIAMRole())

	createCalls, updateCalls := 0, 0
	fakeAWS := &fakePodIdentityAPI{
		DescribePodIdentityAssociationFn: func(_ context.Context, params *awseks.DescribePodIdentityAssociationInput, _ ...func(*awseks.Options)) (*awseks.DescribePodIdentityAssociationOutput, error) {
			if aws.ToString(params.AssociationId) != testAssocID {
				t.Errorf("DescribePodIdentityAssociation id = %q", aws.ToString(params.AssociationId))
			}
			return &awseks.DescribePodIdentityAssociationOutput{Association: &ekstypes.PodIdentityAssociation{
				AssociationId:  aws.String(testAssocID),
				AssociationArn: aws.String(testAssocARN),
				RoleArn:        aws.String(testRoleARN), // matches, no update
			}}, nil
		},
		CreatePodIdentityAssociationFn: func(_ context.Context, _ *awseks.CreatePodIdentityAssociationInput, _ ...func(*awseks.Options)) (*awseks.CreatePodIdentityAssociationOutput, error) {
			createCalls++
			return nil, fmt.Errorf("should not be called")
		},
		UpdatePodIdentityAssociationFn: func(_ context.Context, _ *awseks.UpdatePodIdentityAssociationInput, _ ...func(*awseks.Options)) (*awseks.UpdatePodIdentityAssociationOutput, error) {
			updateCalls++
			return &awseks.UpdatePodIdentityAssociationOutput{}, nil
		},
	}
	r := &PodIdentityAssociationReconciler{Client: cl, Scheme: s, EKSClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-pia", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if createCalls != 0 {
		t.Errorf("Create called %d times, want 0", createCalls)
	}
	if updateCalls != 0 {
		t.Errorf("Update called %d times, want 0", updateCalls)
	}
}

func TestPodIdentityAssociationRoleUpdate(t *testing.T) {
	s := piaTestScheme(t)
	pia := newTestPIA(func(p *awsv1alpha1.PodIdentityAssociation) {
		p.Finalizers = []string{awsv1alpha1.FinalizerName}
		p.Status.AssociationID = testAssocID
	})
	cl := newPIAFakeClient(s, pia, readyIAMRole())

	var updatedRole string
	fakeAWS := &fakePodIdentityAPI{
		DescribePodIdentityAssociationFn: func(_ context.Context, _ *awseks.DescribePodIdentityAssociationInput, _ ...func(*awseks.Options)) (*awseks.DescribePodIdentityAssociationOutput, error) {
			return &awseks.DescribePodIdentityAssociationOutput{Association: &ekstypes.PodIdentityAssociation{
				AssociationId:  aws.String(testAssocID),
				AssociationArn: aws.String(testAssocARN),
				RoleArn:        aws.String("arn:aws:iam::123456789012:role/old-role"),
			}}, nil
		},
		UpdatePodIdentityAssociationFn: func(_ context.Context, params *awseks.UpdatePodIdentityAssociationInput, _ ...func(*awseks.Options)) (*awseks.UpdatePodIdentityAssociationOutput, error) {
			updatedRole = aws.ToString(params.RoleArn)
			return &awseks.UpdatePodIdentityAssociationOutput{Association: &ekstypes.PodIdentityAssociation{
				AssociationId:  aws.String(testAssocID),
				AssociationArn: aws.String(testAssocARN),
				RoleArn:        params.RoleArn,
			}}, nil
		},
	}
	r := &PodIdentityAssociationReconciler{Client: cl, Scheme: s, EKSClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-pia", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if updatedRole != testRoleARN {
		t.Errorf("UpdatePodIdentityAssociation role = %q, want %q", updatedRole, testRoleARN)
	}
}

func TestPodIdentityAssociationDependencyNotReady(t *testing.T) {
	s := piaTestScheme(t)
	pia := newTestPIA(func(p *awsv1alpha1.PodIdentityAssociation) {
		p.Finalizers = []string{awsv1alpha1.FinalizerName}
	})
	// Role exists but has no ARN yet.
	cl := newPIAFakeClient(s, pia, newTestIAMRole())

	createCalls := 0
	fakeAWS := &fakePodIdentityAPI{
		CreatePodIdentityAssociationFn: func(_ context.Context, _ *awseks.CreatePodIdentityAssociationInput, _ ...func(*awseks.Options)) (*awseks.CreatePodIdentityAssociationOutput, error) {
			createCalls++
			return nil, fmt.Errorf("should not be called")
		},
	}
	r := &PodIdentityAssociationReconciler{Client: cl, Scheme: s, EKSClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-pia", Namespace: "default"}
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("dependency-not-ready must not return an error, got %v", err)
	}
	if res != requeueDependency {
		t.Errorf("result = %+v, want requeueDependency %+v", res, requeueDependency)
	}
	if createCalls != 0 {
		t.Errorf("Create called %d times, want 0", createCalls)
	}
}

func TestPodIdentityAssociationAnnotatesServiceAccount(t *testing.T) {
	s := piaTestScheme(t)
	pia := newTestPIA(func(p *awsv1alpha1.PodIdentityAssociation) {
		p.Finalizers = []string{awsv1alpha1.FinalizerName}
		p.Spec.AnnotateServiceAccount = true
	})
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
		Name:      "workload-sa",
		Namespace: "workload-ns",
	}}
	cl := newPIAFakeClient(s, pia, readyIAMRole(), sa)

	fakeAWS := &fakePodIdentityAPI{
		CreatePodIdentityAssociationFn: func(_ context.Context, _ *awseks.CreatePodIdentityAssociationInput, _ ...func(*awseks.Options)) (*awseks.CreatePodIdentityAssociationOutput, error) {
			return &awseks.CreatePodIdentityAssociationOutput{Association: &ekstypes.PodIdentityAssociation{
				AssociationId:  aws.String(testAssocID),
				AssociationArn: aws.String(testAssocARN),
			}}, nil
		},
	}
	r := &PodIdentityAssociationReconciler{Client: cl, Scheme: s, EKSClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-pia", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	gotSA := &corev1.ServiceAccount{}
	if err := cl.Get(context.Background(), k8stypes.NamespacedName{Name: "workload-sa", Namespace: "workload-ns"}, gotSA); err != nil {
		t.Fatalf("get sa: %v", err)
	}
	if gotSA.Annotations["eks.amazonaws.com/role-arn"] != testRoleARN {
		t.Errorf("SA role-arn annotation = %q, want %q", gotSA.Annotations["eks.amazonaws.com/role-arn"], testRoleARN)
	}
}

func TestPodIdentityAssociationDeleteWithFinalizer(t *testing.T) {
	s := piaTestScheme(t)
	pia := newTestPIA(func(p *awsv1alpha1.PodIdentityAssociation) {
		p.Finalizers = []string{awsv1alpha1.FinalizerName}
		p.Status.AssociationID = testAssocID
	})
	cl := newPIAFakeClient(s, pia)

	deleteCalls := 0
	fakeAWS := &fakePodIdentityAPI{
		DeletePodIdentityAssociationFn: func(_ context.Context, params *awseks.DeletePodIdentityAssociationInput, _ ...func(*awseks.Options)) (*awseks.DeletePodIdentityAssociationOutput, error) {
			deleteCalls++
			if aws.ToString(params.ClusterName) != "my-cluster" || aws.ToString(params.AssociationId) != testAssocID {
				t.Errorf("DeletePodIdentityAssociation = %q/%q", aws.ToString(params.ClusterName), aws.ToString(params.AssociationId))
			}
			return &awseks.DeletePodIdentityAssociationOutput{}, nil
		},
	}
	r := &PodIdentityAssociationReconciler{Client: cl, Scheme: s, EKSClient: fakeAWS}

	ctx := context.Background()
	if err := cl.Delete(ctx, pia); err != nil {
		t.Fatalf("delete: %v", err)
	}

	key := k8stypes.NamespacedName{Name: "test-pia", Namespace: "default"}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if deleteCalls != 1 {
		t.Errorf("Delete called %d times, want 1", deleteCalls)
	}

	got := &awsv1alpha1.PodIdentityAssociation{}
	if err := cl.Get(ctx, key, got); !errors.IsNotFound(err) {
		t.Errorf("expected object gone, got err=%v", err)
	}
}

func TestPodIdentityAssociationDeleteWithoutIDSkipsAWS(t *testing.T) {
	s := piaTestScheme(t)
	pia := newTestPIA(func(p *awsv1alpha1.PodIdentityAssociation) {
		p.Finalizers = []string{awsv1alpha1.FinalizerName}
	})
	cl := newPIAFakeClient(s, pia)

	deleteCalls := 0
	fakeAWS := &fakePodIdentityAPI{
		DeletePodIdentityAssociationFn: func(_ context.Context, _ *awseks.DeletePodIdentityAssociationInput, _ ...func(*awseks.Options)) (*awseks.DeletePodIdentityAssociationOutput, error) {
			deleteCalls++
			return &awseks.DeletePodIdentityAssociationOutput{}, nil
		},
	}
	r := &PodIdentityAssociationReconciler{Client: cl, Scheme: s, EKSClient: fakeAWS}

	ctx := context.Background()
	if err := cl.Delete(ctx, pia); err != nil {
		t.Fatalf("delete: %v", err)
	}

	key := k8stypes.NamespacedName{Name: "test-pia", Namespace: "default"}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if deleteCalls != 0 {
		t.Errorf("Delete called %d times, want 0 (no association ID)", deleteCalls)
	}

	got := &awsv1alpha1.PodIdentityAssociation{}
	if err := cl.Get(ctx, key, got); !errors.IsNotFound(err) {
		t.Errorf("expected object gone, got err=%v", err)
	}
}

func TestPodIdentityAssociationAbandonAnnotation(t *testing.T) {
	s := piaTestScheme(t)
	pia := newTestPIA(func(p *awsv1alpha1.PodIdentityAssociation) {
		p.Finalizers = []string{awsv1alpha1.FinalizerName}
		p.Status.AssociationID = testAssocID
		p.Annotations = map[string]string{
			awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon,
		}
	})
	cl := newPIAFakeClient(s, pia)

	deleteCalls := 0
	fakeAWS := &fakePodIdentityAPI{
		DeletePodIdentityAssociationFn: func(_ context.Context, _ *awseks.DeletePodIdentityAssociationInput, _ ...func(*awseks.Options)) (*awseks.DeletePodIdentityAssociationOutput, error) {
			deleteCalls++
			return &awseks.DeletePodIdentityAssociationOutput{}, nil
		},
	}
	r := &PodIdentityAssociationReconciler{Client: cl, Scheme: s, EKSClient: fakeAWS}

	ctx := context.Background()
	if err := cl.Delete(ctx, pia); err != nil {
		t.Fatalf("delete: %v", err)
	}

	key := k8stypes.NamespacedName{Name: "test-pia", Namespace: "default"}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if deleteCalls != 0 {
		t.Errorf("Delete called %d times, want 0 (abandon)", deleteCalls)
	}

	got := &awsv1alpha1.PodIdentityAssociation{}
	if err := cl.Get(ctx, key, got); !errors.IsNotFound(err) {
		t.Errorf("expected object gone, got err=%v", err)
	}
}
