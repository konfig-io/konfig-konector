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
	awsbudgets "github.com/aws/aws-sdk-go-v2/service/budgets"
	budgetstypes "github.com/aws/aws-sdk-go-v2/service/budgets/types"
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

type fakeBudgets struct {
	describeBudget func(ctx context.Context, params *awsbudgets.DescribeBudgetInput) (*awsbudgets.DescribeBudgetOutput, error)
	createBudget   func(ctx context.Context, params *awsbudgets.CreateBudgetInput) (*awsbudgets.CreateBudgetOutput, error)
	updateBudget   func(ctx context.Context, params *awsbudgets.UpdateBudgetInput) (*awsbudgets.UpdateBudgetOutput, error)
	deleteBudget   func(ctx context.Context, params *awsbudgets.DeleteBudgetInput) (*awsbudgets.DeleteBudgetOutput, error)

	createCalled bool
	createInput  *awsbudgets.CreateBudgetInput
	updateCalled bool
	deleteCalled bool
	deleteInput  *awsbudgets.DeleteBudgetInput
}

func (f *fakeBudgets) DescribeBudget(ctx context.Context, params *awsbudgets.DescribeBudgetInput, _ ...func(*awsbudgets.Options)) (*awsbudgets.DescribeBudgetOutput, error) {
	if f.describeBudget == nil {
		return nil, fmt.Errorf("unexpected call to DescribeBudget")
	}
	return f.describeBudget(ctx, params)
}

func (f *fakeBudgets) CreateBudget(ctx context.Context, params *awsbudgets.CreateBudgetInput, _ ...func(*awsbudgets.Options)) (*awsbudgets.CreateBudgetOutput, error) {
	f.createCalled = true
	f.createInput = params
	if f.createBudget == nil {
		return nil, fmt.Errorf("unexpected call to CreateBudget")
	}
	return f.createBudget(ctx, params)
}

func (f *fakeBudgets) UpdateBudget(ctx context.Context, params *awsbudgets.UpdateBudgetInput, _ ...func(*awsbudgets.Options)) (*awsbudgets.UpdateBudgetOutput, error) {
	f.updateCalled = true
	if f.updateBudget == nil {
		return nil, fmt.Errorf("unexpected call to UpdateBudget")
	}
	return f.updateBudget(ctx, params)
}

func (f *fakeBudgets) DeleteBudget(ctx context.Context, params *awsbudgets.DeleteBudgetInput, _ ...func(*awsbudgets.Options)) (*awsbudgets.DeleteBudgetOutput, error) {
	f.deleteCalled = true
	f.deleteInput = params
	if f.deleteBudget == nil {
		return nil, fmt.Errorf("unexpected call to DeleteBudget")
	}
	return f.deleteBudget(ctx, params)
}

// budgetsNotFound mimics the Budgets NotFoundException smithy error.
type budgetsNotFound struct{}

func (budgetsNotFound) Error() string                 { return "NotFoundException: not found" }
func (budgetsNotFound) ErrorCode() string             { return "NotFoundException" }
func (budgetsNotFound) ErrorMessage() string          { return "not found" }
func (budgetsNotFound) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }

const testBudgetAccount = "123456789012"

func budgetCR(mutate ...func(*awsv1alpha1.Budget)) *awsv1alpha1.Budget {
	b := &awsv1alpha1.Budget{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "monthly-cost",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: awsv1alpha1.BudgetSpec{
			AccountID:   testBudgetAccount,
			BudgetName:  "monthly-cost",
			BudgetType:  "COST",
			TimeUnit:    "MONTHLY",
			LimitAmount: "100",
			LimitUnit:   "USD",
			Notifications: []awsv1alpha1.BudgetNotification{{
				NotificationType:   "ACTUAL",
				ComparisonOperator: "GREATER_THAN",
				Threshold:          "80",
				ThresholdType:      "PERCENTAGE",
				Subscribers: []awsv1alpha1.BudgetSubscriber{{
					Address:          "ops@example.com",
					SubscriptionType: "EMAIL",
				}},
			}},
		},
	}
	for _, m := range mutate {
		m(b)
	}
	return b
}

func newDevOpsCostScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func TestBudgetReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "monthly-cost", Namespace: "default"}}

	tests := []struct {
		name    string
		objs    []client.Object
		fake    *fakeBudgets
		setup   func(t *testing.T, ctx context.Context, c client.Client)
		wantErr bool
		assert  func(t *testing.T, ctx context.Context, c client.Client, f *fakeBudgets, res ctrl.Result)
	}{
		{
			name: "create happy path persists name and Ready",
			objs: []client.Object{budgetCR()},
			fake: &fakeBudgets{
				describeBudget: func(_ context.Context, _ *awsbudgets.DescribeBudgetInput) (*awsbudgets.DescribeBudgetOutput, error) {
					return nil, budgetsNotFound{}
				},
				createBudget: func(_ context.Context, params *awsbudgets.CreateBudgetInput) (*awsbudgets.CreateBudgetOutput, error) {
					if aws.ToString(params.AccountId) != testBudgetAccount {
						return nil, fmt.Errorf("unexpected account ID %q", aws.ToString(params.AccountId))
					}
					if params.Budget == nil || aws.ToString(params.Budget.BudgetName) != "monthly-cost" {
						return nil, fmt.Errorf("unexpected budget name")
					}
					if params.Budget.BudgetType != budgetstypes.BudgetTypeCost {
						return nil, fmt.Errorf("unexpected budget type %q", params.Budget.BudgetType)
					}
					if params.Budget.TimeUnit != budgetstypes.TimeUnitMonthly {
						return nil, fmt.Errorf("unexpected time unit %q", params.Budget.TimeUnit)
					}
					if params.Budget.BudgetLimit == nil || aws.ToString(params.Budget.BudgetLimit.Amount) != "100" || aws.ToString(params.Budget.BudgetLimit.Unit) != "USD" {
						return nil, fmt.Errorf("unexpected budget limit")
					}
					if len(params.NotificationsWithSubscribers) != 1 {
						return nil, fmt.Errorf("expected 1 notification, got %d", len(params.NotificationsWithSubscribers))
					}
					n := params.NotificationsWithSubscribers[0]
					if n.Notification == nil || n.Notification.Threshold != 80 || n.Notification.NotificationType != budgetstypes.NotificationTypeActual {
						return nil, fmt.Errorf("unexpected notification %+v", n.Notification)
					}
					if len(n.Subscribers) != 1 || aws.ToString(n.Subscribers[0].Address) != "ops@example.com" {
						return nil, fmt.Errorf("unexpected subscribers")
					}
					return &awsbudgets.CreateBudgetOutput{}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeBudgets, _ ctrl.Result) {
				if !f.createCalled {
					t.Error("expected CreateBudget to be called")
				}
				got := &awsv1alpha1.Budget{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.BudgetName != "monthly-cost" {
					t.Errorf("status.budgetName = %q, want %q", got.Status.BudgetName, "monthly-cost")
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "steady state does not create or update",
			objs: []client.Object{budgetCR(func(b *awsv1alpha1.Budget) {
				b.Finalizers = []string{awsv1alpha1.FinalizerName}
				b.Status.BudgetName = "monthly-cost"
				b.Status.ObservedGeneration = 1
			})},
			fake: &fakeBudgets{
				describeBudget: func(_ context.Context, params *awsbudgets.DescribeBudgetInput) (*awsbudgets.DescribeBudgetOutput, error) {
					if aws.ToString(params.BudgetName) != "monthly-cost" {
						return nil, fmt.Errorf("unexpected budget name %q", aws.ToString(params.BudgetName))
					}
					return &awsbudgets.DescribeBudgetOutput{Budget: &budgetstypes.Budget{
						BudgetName: aws.String("monthly-cost"),
					}}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeBudgets, _ ctrl.Result) {
				if f.createCalled {
					t.Error("CreateBudget must not be called in steady state")
				}
				if f.updateCalled {
					t.Error("UpdateBudget must not be called when generation is unchanged")
				}
				got := &awsv1alpha1.Budget{}
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
			name: "spec change triggers UpdateBudget",
			objs: []client.Object{budgetCR(func(b *awsv1alpha1.Budget) {
				b.Finalizers = []string{awsv1alpha1.FinalizerName}
				b.Generation = 2
				b.Status.BudgetName = "monthly-cost"
				b.Status.ObservedGeneration = 1
				b.Spec.LimitAmount = "200"
			})},
			fake: &fakeBudgets{
				describeBudget: func(_ context.Context, _ *awsbudgets.DescribeBudgetInput) (*awsbudgets.DescribeBudgetOutput, error) {
					return &awsbudgets.DescribeBudgetOutput{Budget: &budgetstypes.Budget{
						BudgetName: aws.String("monthly-cost"),
					}}, nil
				},
				updateBudget: func(_ context.Context, params *awsbudgets.UpdateBudgetInput) (*awsbudgets.UpdateBudgetOutput, error) {
					if params.NewBudget == nil || aws.ToString(params.NewBudget.BudgetLimit.Amount) != "200" {
						return nil, fmt.Errorf("expected updated limit amount 200")
					}
					return &awsbudgets.UpdateBudgetOutput{}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeBudgets, _ ctrl.Result) {
				if !f.updateCalled {
					t.Error("expected UpdateBudget to be called")
				}
				got := &awsv1alpha1.Budget{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.ObservedGeneration != 2 {
					t.Errorf("observedGeneration = %d, want 2", got.Status.ObservedGeneration)
				}
			},
		},
		{
			name: "create failure surfaces error and Ready=False",
			objs: []client.Object{budgetCR()},
			fake: &fakeBudgets{
				describeBudget: func(_ context.Context, _ *awsbudgets.DescribeBudgetInput) (*awsbudgets.DescribeBudgetOutput, error) {
					return nil, budgetsNotFound{}
				},
				createBudget: func(_ context.Context, _ *awsbudgets.CreateBudgetInput) (*awsbudgets.CreateBudgetOutput, error) {
					return nil, fmt.Errorf("access denied")
				},
			},
			wantErr: true,
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeBudgets, _ ctrl.Result) {
				got := &awsv1alpha1.Budget{}
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
			name: "bad notification threshold fails before AWS create",
			objs: []client.Object{budgetCR(func(b *awsv1alpha1.Budget) {
				b.Spec.Notifications[0].Threshold = "not-a-number"
			})},
			fake: &fakeBudgets{
				describeBudget: func(_ context.Context, _ *awsbudgets.DescribeBudgetInput) (*awsbudgets.DescribeBudgetOutput, error) {
					return nil, budgetsNotFound{}
				},
			},
			wantErr: true,
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeBudgets, _ ctrl.Result) {
				if f.createCalled {
					t.Error("CreateBudget must not be called with an invalid threshold")
				}
			},
		},
		{
			name: "delete with finalizer calls AWS delete with account and name",
			objs: []client.Object{budgetCR(func(b *awsv1alpha1.Budget) {
				b.Finalizers = []string{awsv1alpha1.FinalizerName}
				b.Status.BudgetName = "monthly-cost"
			})},
			fake: &fakeBudgets{
				deleteBudget: func(_ context.Context, _ *awsbudgets.DeleteBudgetInput) (*awsbudgets.DeleteBudgetOutput, error) {
					return &awsbudgets.DeleteBudgetOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, budgetCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeBudgets, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeleteBudget to be called")
				}
				if aws.ToString(f.deleteInput.AccountId) != testBudgetAccount || aws.ToString(f.deleteInput.BudgetName) != "monthly-cost" {
					t.Errorf("DeleteBudget input = %+v", f.deleteInput)
				}
				got := &awsv1alpha1.Budget{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "delete tolerates budget already gone in AWS",
			objs: []client.Object{budgetCR(func(b *awsv1alpha1.Budget) {
				b.Finalizers = []string{awsv1alpha1.FinalizerName}
			})},
			fake: &fakeBudgets{
				deleteBudget: func(_ context.Context, _ *awsbudgets.DeleteBudgetInput) (*awsbudgets.DeleteBudgetOutput, error) {
					return nil, budgetsNotFound{}
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, budgetCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeBudgets, _ ctrl.Result) {
				got := &awsv1alpha1.Budget{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "abandon policy skips AWS delete",
			objs: []client.Object{budgetCR(func(b *awsv1alpha1.Budget) {
				b.Finalizers = []string{awsv1alpha1.FinalizerName}
				b.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			})},
			fake: &fakeBudgets{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, budgetCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeBudgets, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeleteBudget must not be called when abandoning")
				}
				got := &awsv1alpha1.Budget{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := newDevOpsCostScheme(t)
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&awsv1alpha1.Budget{}).
				WithObjects(tc.objs...).
				Build()
			r := &BudgetReconciler{Client: c, Scheme: scheme, BudgetsClient: tc.fake}

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
