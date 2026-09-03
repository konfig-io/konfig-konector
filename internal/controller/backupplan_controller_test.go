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
	awsbackup "github.com/aws/aws-sdk-go-v2/service/backup"
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

type fakeBackupPlanAPI struct {
	getBackupPlan    func(ctx context.Context, params *awsbackup.GetBackupPlanInput) (*awsbackup.GetBackupPlanOutput, error)
	createBackupPlan func(ctx context.Context, params *awsbackup.CreateBackupPlanInput) (*awsbackup.CreateBackupPlanOutput, error)
	updateBackupPlan func(ctx context.Context, params *awsbackup.UpdateBackupPlanInput) (*awsbackup.UpdateBackupPlanOutput, error)
	deleteBackupPlan func(ctx context.Context, params *awsbackup.DeleteBackupPlanInput) (*awsbackup.DeleteBackupPlanOutput, error)

	createCalled  bool
	updateCalled  bool
	deleteCalled  bool
	deletedPlanID string
	createInput   *awsbackup.CreateBackupPlanInput
}

func (f *fakeBackupPlanAPI) GetBackupPlan(ctx context.Context, params *awsbackup.GetBackupPlanInput, _ ...func(*awsbackup.Options)) (*awsbackup.GetBackupPlanOutput, error) {
	if f.getBackupPlan == nil {
		return nil, fmt.Errorf("unexpected call to GetBackupPlan")
	}
	return f.getBackupPlan(ctx, params)
}

func (f *fakeBackupPlanAPI) CreateBackupPlan(ctx context.Context, params *awsbackup.CreateBackupPlanInput, _ ...func(*awsbackup.Options)) (*awsbackup.CreateBackupPlanOutput, error) {
	f.createCalled = true
	f.createInput = params
	if f.createBackupPlan == nil {
		return nil, fmt.Errorf("unexpected call to CreateBackupPlan")
	}
	return f.createBackupPlan(ctx, params)
}

func (f *fakeBackupPlanAPI) UpdateBackupPlan(ctx context.Context, params *awsbackup.UpdateBackupPlanInput, _ ...func(*awsbackup.Options)) (*awsbackup.UpdateBackupPlanOutput, error) {
	f.updateCalled = true
	if f.updateBackupPlan == nil {
		return nil, fmt.Errorf("unexpected call to UpdateBackupPlan")
	}
	return f.updateBackupPlan(ctx, params)
}

func (f *fakeBackupPlanAPI) DeleteBackupPlan(ctx context.Context, params *awsbackup.DeleteBackupPlanInput, _ ...func(*awsbackup.Options)) (*awsbackup.DeleteBackupPlanOutput, error) {
	f.deleteCalled = true
	f.deletedPlanID = aws.ToString(params.BackupPlanId)
	if f.deleteBackupPlan == nil {
		return nil, fmt.Errorf("unexpected call to DeleteBackupPlan")
	}
	return f.deleteBackupPlan(ctx, params)
}

func backupNotFoundErr() error {
	return &smithy.GenericAPIError{Code: "ResourceNotFoundException", Message: "not found"}
}

const (
	testPlanID  = "8F81F553-3A74-4A3F-B93D-B3360DC80C50"
	testPlanARN = "arn:aws:backup:us-east-1:123456789012:plan:" + testPlanID
)

func backupPlanCR(mutate ...func(*awsv1alpha1.BackupPlan)) *awsv1alpha1.BackupPlan {
	plan := &awsv1alpha1.BackupPlan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-plan",
			Namespace: "default",
		},
		Spec: awsv1alpha1.BackupPlanSpec{
			PlanName: "my-plan",
			Rules: []awsv1alpha1.BackupPlanRule{
				{
					RuleName:              "daily",
					TargetBackupVaultName: "my-vault",
					ScheduleExpression:    "cron(0 5 * * ? *)",
					StartWindowMinutes:    aws.Int64(60),
					Lifecycle: &awsv1alpha1.BackupLifecycle{
						DeleteAfterDays: aws.Int64(30),
					},
				},
			},
		},
	}
	for _, m := range mutate {
		m(plan)
	}
	return plan
}

func newBackupPlanFakeClient(scheme *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&awsv1alpha1.BackupPlan{}, &awsv1alpha1.BackupVault{}).
		WithObjects(objs...).
		Build()
}

func TestBackupPlanReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-plan", Namespace: "default"}}

	tests := []struct {
		name    string
		objs    []client.Object
		fake    *fakeBackupPlanAPI
		setup   func(t *testing.T, ctx context.Context, c client.Client)
		wantErr bool
		assert  func(t *testing.T, ctx context.Context, c client.Client, f *fakeBackupPlanAPI, res ctrl.Result)
	}{
		{
			name: "create happy path persists plan ID and Ready",
			objs: []client.Object{backupPlanCR()},
			fake: &fakeBackupPlanAPI{
				createBackupPlan: func(_ context.Context, params *awsbackup.CreateBackupPlanInput) (*awsbackup.CreateBackupPlanOutput, error) {
					if aws.ToString(params.BackupPlan.BackupPlanName) != "my-plan" {
						return nil, fmt.Errorf("unexpected plan name")
					}
					if len(params.BackupPlan.Rules) != 1 {
						return nil, fmt.Errorf("expected 1 rule, got %d", len(params.BackupPlan.Rules))
					}
					rule := params.BackupPlan.Rules[0]
					if aws.ToString(rule.TargetBackupVaultName) != "my-vault" {
						return nil, fmt.Errorf("unexpected vault %q", aws.ToString(rule.TargetBackupVaultName))
					}
					if rule.Lifecycle == nil || aws.ToInt64(rule.Lifecycle.DeleteAfterDays) != 30 {
						return nil, fmt.Errorf("lifecycle not mapped")
					}
					return &awsbackup.CreateBackupPlanOutput{
						BackupPlanId:  aws.String(testPlanID),
						BackupPlanArn: aws.String(testPlanARN),
						VersionId:     aws.String("v1"),
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeBackupPlanAPI, _ ctrl.Result) {
				got := &awsv1alpha1.BackupPlan{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if !f.createCalled {
					t.Error("expected CreateBackupPlan to be called")
				}
				if got.Status.PlanID != testPlanID {
					t.Errorf("status.planId = %q, want %q", got.Status.PlanID, testPlanID)
				}
				if got.Status.PlanARN != testPlanARN {
					t.Errorf("status.planArn = %q, want %q", got.Status.PlanARN, testPlanARN)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "identifier persisted even when post-create status write happens first",
			objs: []client.Object{backupPlanCR()},
			fake: &fakeBackupPlanAPI{
				createBackupPlan: func(_ context.Context, _ *awsbackup.CreateBackupPlanInput) (*awsbackup.CreateBackupPlanOutput, error) {
					return &awsbackup.CreateBackupPlanOutput{
						BackupPlanId:  aws.String(testPlanID),
						BackupPlanArn: aws.String(testPlanARN),
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeBackupPlanAPI, _ ctrl.Result) {
				got := &awsv1alpha1.BackupPlan{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.PlanID != testPlanID {
					t.Errorf("status.planId = %q, want %q (identifier must be persisted immediately after create)", got.Status.PlanID, testPlanID)
				}
			},
		},
		{
			name: "steady state does not create",
			objs: []client.Object{backupPlanCR(func(p *awsv1alpha1.BackupPlan) {
				p.Finalizers = []string{awsv1alpha1.FinalizerName}
				p.Status.PlanID = testPlanID
				p.Status.ObservedGeneration = 1
				p.Generation = 1
			})},
			fake: &fakeBackupPlanAPI{
				getBackupPlan: func(_ context.Context, params *awsbackup.GetBackupPlanInput) (*awsbackup.GetBackupPlanOutput, error) {
					if aws.ToString(params.BackupPlanId) != testPlanID {
						return nil, fmt.Errorf("unexpected plan ID")
					}
					return &awsbackup.GetBackupPlanOutput{BackupPlanId: aws.String(testPlanID)}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeBackupPlanAPI, _ ctrl.Result) {
				if f.createCalled {
					t.Error("CreateBackupPlan must not be called in steady state")
				}
				if f.updateCalled {
					t.Error("UpdateBackupPlan must not be called when generation is observed")
				}
				got := &awsv1alpha1.BackupPlan{}
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
			name: "generation change triggers UpdateBackupPlan",
			objs: []client.Object{backupPlanCR(func(p *awsv1alpha1.BackupPlan) {
				p.Finalizers = []string{awsv1alpha1.FinalizerName}
				p.Status.PlanID = testPlanID
				p.Status.ObservedGeneration = 1
				p.Generation = 2
			})},
			fake: &fakeBackupPlanAPI{
				getBackupPlan: func(_ context.Context, _ *awsbackup.GetBackupPlanInput) (*awsbackup.GetBackupPlanOutput, error) {
					return &awsbackup.GetBackupPlanOutput{BackupPlanId: aws.String(testPlanID)}, nil
				},
				updateBackupPlan: func(_ context.Context, params *awsbackup.UpdateBackupPlanInput) (*awsbackup.UpdateBackupPlanOutput, error) {
					if aws.ToString(params.BackupPlanId) != testPlanID {
						return nil, fmt.Errorf("unexpected plan ID")
					}
					return &awsbackup.UpdateBackupPlanOutput{
						BackupPlanArn: aws.String(testPlanARN),
						VersionId:     aws.String("v2"),
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeBackupPlanAPI, _ ctrl.Result) {
				if !f.updateCalled {
					t.Error("expected UpdateBackupPlan to be called for new generation")
				}
				got := &awsv1alpha1.BackupPlan{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.VersionID != "v2" {
					t.Errorf("status.versionId = %q, want v2", got.Status.VersionID)
				}
				if got.Status.ObservedGeneration != 2 {
					t.Errorf("observedGeneration = %d, want 2", got.Status.ObservedGeneration)
				}
			},
		},
		{
			name: "vault ref not ready requeues without AWS call",
			objs: []client.Object{
				backupPlanCR(func(p *awsv1alpha1.BackupPlan) {
					p.Spec.Rules[0].TargetBackupVaultName = ""
					p.Spec.Rules[0].TargetBackupVaultRef = "my-vault-cr"
				}),
				&awsv1alpha1.BackupVault{
					ObjectMeta: metav1.ObjectMeta{Name: "my-vault-cr", Namespace: "default"},
					Spec:       awsv1alpha1.BackupVaultSpec{VaultName: "my-vault"},
				},
			},
			fake: &fakeBackupPlanAPI{},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeBackupPlanAPI, res ctrl.Result) {
				if res != requeueDependency {
					t.Errorf("result = %+v, want requeueDependency %+v", res, requeueDependency)
				}
				if f.createCalled {
					t.Error("CreateBackupPlan must not be called while vault dependency is not ready")
				}
			},
		},
		{
			name: "vault ref resolves to AWS vault name on create",
			objs: []client.Object{
				backupPlanCR(func(p *awsv1alpha1.BackupPlan) {
					p.Spec.Rules[0].TargetBackupVaultName = ""
					p.Spec.Rules[0].TargetBackupVaultRef = "my-vault-cr"
				}),
				&awsv1alpha1.BackupVault{
					ObjectMeta: metav1.ObjectMeta{Name: "my-vault-cr", Namespace: "default"},
					Spec:       awsv1alpha1.BackupVaultSpec{VaultName: "my-aws-vault"},
					Status: awsv1alpha1.BackupVaultStatus{
						VaultARN: "arn:aws:backup:us-east-1:123456789012:backup-vault:my-aws-vault",
					},
				},
			},
			fake: &fakeBackupPlanAPI{
				createBackupPlan: func(_ context.Context, params *awsbackup.CreateBackupPlanInput) (*awsbackup.CreateBackupPlanOutput, error) {
					if aws.ToString(params.BackupPlan.Rules[0].TargetBackupVaultName) != "my-aws-vault" {
						return nil, fmt.Errorf("vault ref not resolved, got %q", aws.ToString(params.BackupPlan.Rules[0].TargetBackupVaultName))
					}
					return &awsbackup.CreateBackupPlanOutput{
						BackupPlanId:  aws.String(testPlanID),
						BackupPlanArn: aws.String(testPlanARN),
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeBackupPlanAPI, _ ctrl.Result) {
				if !f.createCalled {
					t.Error("expected CreateBackupPlan to be called with resolved vault name")
				}
			},
		},
		{
			name: "delete with finalizer calls AWS delete with status identifier",
			objs: []client.Object{backupPlanCR(func(p *awsv1alpha1.BackupPlan) {
				p.Finalizers = []string{awsv1alpha1.FinalizerName}
				p.Status.PlanID = testPlanID
			})},
			fake: &fakeBackupPlanAPI{
				deleteBackupPlan: func(_ context.Context, _ *awsbackup.DeleteBackupPlanInput) (*awsbackup.DeleteBackupPlanOutput, error) {
					return &awsbackup.DeleteBackupPlanOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, backupPlanCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeBackupPlanAPI, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeleteBackupPlan to be called")
				}
				if f.deletedPlanID != testPlanID {
					t.Errorf("DeleteBackupPlan id = %q, want %q", f.deletedPlanID, testPlanID)
				}
				got := &awsv1alpha1.BackupPlan{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "delete with empty status ID skips AWS delete (no unambiguous lookup)",
			objs: []client.Object{backupPlanCR(func(p *awsv1alpha1.BackupPlan) {
				p.Finalizers = []string{awsv1alpha1.FinalizerName}
			})},
			fake: &fakeBackupPlanAPI{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, backupPlanCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeBackupPlanAPI, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeleteBackupPlan must not be called without a stored plan ID")
				}
				got := &awsv1alpha1.BackupPlan{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "abandon policy skips AWS delete",
			objs: []client.Object{backupPlanCR(func(p *awsv1alpha1.BackupPlan) {
				p.Finalizers = []string{awsv1alpha1.FinalizerName}
				p.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				p.Status.PlanID = testPlanID
			})},
			fake: &fakeBackupPlanAPI{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, backupPlanCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeBackupPlanAPI, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeleteBackupPlan must not be called when abandoning")
				}
				got := &awsv1alpha1.BackupPlan{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "recreates plan when stored ID no longer exists",
			objs: []client.Object{backupPlanCR(func(p *awsv1alpha1.BackupPlan) {
				p.Finalizers = []string{awsv1alpha1.FinalizerName}
				p.Status.PlanID = "stale-id"
			})},
			fake: &fakeBackupPlanAPI{
				getBackupPlan: func(_ context.Context, _ *awsbackup.GetBackupPlanInput) (*awsbackup.GetBackupPlanOutput, error) {
					return nil, backupNotFoundErr()
				},
				createBackupPlan: func(_ context.Context, _ *awsbackup.CreateBackupPlanInput) (*awsbackup.CreateBackupPlanOutput, error) {
					return &awsbackup.CreateBackupPlanOutput{
						BackupPlanId:  aws.String(testPlanID),
						BackupPlanArn: aws.String(testPlanARN),
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeBackupPlanAPI, _ ctrl.Result) {
				if !f.createCalled {
					t.Error("expected CreateBackupPlan to be called for stale plan ID")
				}
				got := &awsv1alpha1.BackupPlan{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.PlanID != testPlanID {
					t.Errorf("status.planId = %q, want %q", got.Status.PlanID, testPlanID)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := newGovScheme(t)
			c := newBackupPlanFakeClient(scheme, tc.objs...)
			r := &BackupPlanReconciler{Client: c, Scheme: scheme, BackupClient: tc.fake}

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
