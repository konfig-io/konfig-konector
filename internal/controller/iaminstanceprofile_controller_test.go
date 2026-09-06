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
	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeIAMInstanceProfile struct {
	getIP      func(ctx context.Context, params *awsiam.GetInstanceProfileInput) (*awsiam.GetInstanceProfileOutput, error)
	createIP   func(ctx context.Context, params *awsiam.CreateInstanceProfileInput) (*awsiam.CreateInstanceProfileOutput, error)
	deleteIP   func(ctx context.Context, params *awsiam.DeleteInstanceProfileInput) (*awsiam.DeleteInstanceProfileOutput, error)
	addRole    func(ctx context.Context, params *awsiam.AddRoleToInstanceProfileInput) (*awsiam.AddRoleToInstanceProfileOutput, error)
	removeRole func(ctx context.Context, params *awsiam.RemoveRoleFromInstanceProfileInput) (*awsiam.RemoveRoleFromInstanceProfileOutput, error)

	createCalled     bool
	deleteCalled     bool
	addRoleCalled    bool
	addedRole        string
	removeRoleCalled bool
	removedRole      string
}

func (f *fakeIAMInstanceProfile) GetInstanceProfile(ctx context.Context, params *awsiam.GetInstanceProfileInput, _ ...func(*awsiam.Options)) (*awsiam.GetInstanceProfileOutput, error) {
	if f.getIP == nil {
		return nil, fmt.Errorf("unexpected call to GetInstanceProfile")
	}
	return f.getIP(ctx, params)
}

func (f *fakeIAMInstanceProfile) CreateInstanceProfile(ctx context.Context, params *awsiam.CreateInstanceProfileInput, _ ...func(*awsiam.Options)) (*awsiam.CreateInstanceProfileOutput, error) {
	f.createCalled = true
	if f.createIP == nil {
		return nil, fmt.Errorf("unexpected call to CreateInstanceProfile")
	}
	return f.createIP(ctx, params)
}

func (f *fakeIAMInstanceProfile) DeleteInstanceProfile(ctx context.Context, params *awsiam.DeleteInstanceProfileInput, _ ...func(*awsiam.Options)) (*awsiam.DeleteInstanceProfileOutput, error) {
	f.deleteCalled = true
	if f.deleteIP == nil {
		return nil, fmt.Errorf("unexpected call to DeleteInstanceProfile")
	}
	return f.deleteIP(ctx, params)
}

func (f *fakeIAMInstanceProfile) AddRoleToInstanceProfile(ctx context.Context, params *awsiam.AddRoleToInstanceProfileInput, _ ...func(*awsiam.Options)) (*awsiam.AddRoleToInstanceProfileOutput, error) {
	f.addRoleCalled = true
	f.addedRole = aws.ToString(params.RoleName)
	if f.addRole == nil {
		return nil, fmt.Errorf("unexpected call to AddRoleToInstanceProfile")
	}
	return f.addRole(ctx, params)
}

func (f *fakeIAMInstanceProfile) RemoveRoleFromInstanceProfile(ctx context.Context, params *awsiam.RemoveRoleFromInstanceProfileInput, _ ...func(*awsiam.Options)) (*awsiam.RemoveRoleFromInstanceProfileOutput, error) {
	f.removeRoleCalled = true
	f.removedRole = aws.ToString(params.RoleName)
	if f.removeRole == nil {
		return nil, fmt.Errorf("unexpected call to RemoveRoleFromInstanceProfile")
	}
	return f.removeRole(ctx, params)
}

func iamIPNotFoundErr() error {
	return &iamtypes.NoSuchEntityException{Message: aws.String("not found")}
}

func TestIAMInstanceProfileReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-profile", Namespace: "default"}}
	ipARN := "arn:aws:iam::123456789012:instance-profile/my-profile"
	roleARN := "arn:aws:iam::123456789012:role/my-role"
	ipCR := func(mutate ...func(*awsv1alpha1.IAMInstanceProfile)) *awsv1alpha1.IAMInstanceProfile {
		ip := &awsv1alpha1.IAMInstanceProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "my-profile", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
			Spec: awsv1alpha1.IAMInstanceProfileSpec{
				InstanceProfileName: "my-profile",
				RoleRef:             &awsv1alpha1.RoleRef{ARN: roleARN},
			},
		}
		for _, m := range mutate {
			m(ip)
		}
		return ip
	}

	t.Run("create persists ARN and adds role", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, ipCR())
		f := &fakeIAMInstanceProfile{
			getIP: func(_ context.Context, _ *awsiam.GetInstanceProfileInput) (*awsiam.GetInstanceProfileOutput, error) {
				return nil, iamIPNotFoundErr()
			},
			createIP: func(_ context.Context, params *awsiam.CreateInstanceProfileInput) (*awsiam.CreateInstanceProfileOutput, error) {
				if aws.ToString(params.InstanceProfileName) != "my-profile" {
					t.Errorf("profile name = %q", aws.ToString(params.InstanceProfileName))
				}
				return &awsiam.CreateInstanceProfileOutput{InstanceProfile: &iamtypes.InstanceProfile{
					Arn:                 aws.String(ipARN),
					InstanceProfileName: params.InstanceProfileName,
				}}, nil
			},
			addRole: func(_ context.Context, _ *awsiam.AddRoleToInstanceProfileInput) (*awsiam.AddRoleToInstanceProfileOutput, error) {
				return &awsiam.AddRoleToInstanceProfileOutput{}, nil
			},
		}
		r := &IAMInstanceProfileReconciler{Client: c, Scheme: scheme, IAMClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.IAMInstanceProfile{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ARN != ipARN {
			t.Errorf("status.arn = %q", got.Status.ARN)
		}
		if !f.addRoleCalled || f.addedRole != "my-role" {
			t.Errorf("expected AddRoleToInstanceProfile with my-role, got called=%v role=%q", f.addRoleCalled, f.addedRole)
		}
		if got.Status.RoleName != "my-role" {
			t.Errorf("status.roleName = %q", got.Status.RoleName)
		}
	})

	t.Run("role swap removes old role and adds new one", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, ipCR(func(ip *awsv1alpha1.IAMInstanceProfile) {
			ip.Finalizers = []string{awsv1alpha1.FinalizerName}
			ip.Status.ARN = ipARN
		}))
		f := &fakeIAMInstanceProfile{
			getIP: func(_ context.Context, _ *awsiam.GetInstanceProfileInput) (*awsiam.GetInstanceProfileOutput, error) {
				return &awsiam.GetInstanceProfileOutput{InstanceProfile: &iamtypes.InstanceProfile{
					Arn:                 aws.String(ipARN),
					InstanceProfileName: aws.String("my-profile"),
					Roles:               []iamtypes.Role{{RoleName: aws.String("old-role")}},
				}}, nil
			},
			removeRole: func(_ context.Context, _ *awsiam.RemoveRoleFromInstanceProfileInput) (*awsiam.RemoveRoleFromInstanceProfileOutput, error) {
				return &awsiam.RemoveRoleFromInstanceProfileOutput{}, nil
			},
			addRole: func(_ context.Context, _ *awsiam.AddRoleToInstanceProfileInput) (*awsiam.AddRoleToInstanceProfileOutput, error) {
				return &awsiam.AddRoleToInstanceProfileOutput{}, nil
			},
		}
		r := &IAMInstanceProfileReconciler{Client: c, Scheme: scheme, IAMClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.removeRoleCalled || f.removedRole != "old-role" {
			t.Errorf("expected old-role removed, called=%v role=%q", f.removeRoleCalled, f.removedRole)
		}
		if !f.addRoleCalled || f.addedRole != "my-role" {
			t.Errorf("expected my-role added, called=%v role=%q", f.addRoleCalled, f.addedRole)
		}
	})

	t.Run("delete removes roles then deletes profile", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, ipCR(func(ip *awsv1alpha1.IAMInstanceProfile) {
			ip.Finalizers = []string{awsv1alpha1.FinalizerName}
			ip.Status.ARN = ipARN
		}))
		f := &fakeIAMInstanceProfile{
			getIP: func(_ context.Context, _ *awsiam.GetInstanceProfileInput) (*awsiam.GetInstanceProfileOutput, error) {
				return &awsiam.GetInstanceProfileOutput{InstanceProfile: &iamtypes.InstanceProfile{
					Arn:                 aws.String(ipARN),
					InstanceProfileName: aws.String("my-profile"),
					Roles:               []iamtypes.Role{{RoleName: aws.String("my-role")}},
				}}, nil
			},
			removeRole: func(_ context.Context, _ *awsiam.RemoveRoleFromInstanceProfileInput) (*awsiam.RemoveRoleFromInstanceProfileOutput, error) {
				return &awsiam.RemoveRoleFromInstanceProfileOutput{}, nil
			},
			deleteIP: func(_ context.Context, _ *awsiam.DeleteInstanceProfileInput) (*awsiam.DeleteInstanceProfileOutput, error) {
				return &awsiam.DeleteInstanceProfileOutput{}, nil
			},
		}
		r := &IAMInstanceProfileReconciler{Client: c, Scheme: scheme, IAMClient: f}
		if err := c.Delete(ctx, ipCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.removeRoleCalled {
			t.Error("expected role removed before delete")
		}
		if !f.deleteCalled {
			t.Error("expected DeleteInstanceProfile to be called")
		}
		got := &awsv1alpha1.IAMInstanceProfile{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, ipCR(func(ip *awsv1alpha1.IAMInstanceProfile) {
			ip.Finalizers = []string{awsv1alpha1.FinalizerName}
			ip.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		}))
		f := &fakeIAMInstanceProfile{}
		r := &IAMInstanceProfileReconciler{Client: c, Scheme: scheme, IAMClient: f}
		if err := c.Delete(ctx, ipCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteInstanceProfile must not be called when abandoning")
		}
	})
}
