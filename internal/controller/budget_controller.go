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
	"errors"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsbudgets "github.com/aws/aws-sdk-go-v2/service/budgets"
	budgetstypes "github.com/aws/aws-sdk-go-v2/service/budgets/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	budgetshelper "github.com/konfig-io/konfig-konector/internal/aws/budgets"
)

// BudgetAWSAPI is the subset of the Budgets API used by this controller.
type BudgetAWSAPI interface {
	DescribeBudget(ctx context.Context, params *awsbudgets.DescribeBudgetInput, optFns ...func(*awsbudgets.Options)) (*awsbudgets.DescribeBudgetOutput, error)
	CreateBudget(ctx context.Context, params *awsbudgets.CreateBudgetInput, optFns ...func(*awsbudgets.Options)) (*awsbudgets.CreateBudgetOutput, error)
	UpdateBudget(ctx context.Context, params *awsbudgets.UpdateBudgetInput, optFns ...func(*awsbudgets.Options)) (*awsbudgets.UpdateBudgetOutput, error)
	DeleteBudget(ctx context.Context, params *awsbudgets.DeleteBudgetInput, optFns ...func(*awsbudgets.Options)) (*awsbudgets.DeleteBudgetOutput, error)
}

// BudgetReconciler reconciles Budget objects.
type BudgetReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	BudgetsClient BudgetAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=budgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=budgets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=budgets/finalizers,verbs=update

func (r *BudgetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	b := &awsv1alpha1.Budget{}
	if err := r.Get(ctx, req.NamespacedName, b); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, b); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !b.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(b, awsv1alpha1.FinalizerName) {
			if shouldAbandon(b) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(b, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, b)
			}
			if err := r.deleteBudget(ctx, b); err != nil {
				logger.Error(err, "failed to delete budget")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(b, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, b)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(b, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(b, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, b); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileBudget(ctx, b); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, b, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *BudgetReconciler) buildBudget(b *awsv1alpha1.Budget) *budgetstypes.Budget {
	budget := &budgetstypes.Budget{
		BudgetName: aws.String(b.Spec.BudgetName),
		BudgetType: budgetstypes.BudgetType(b.Spec.BudgetType),
		TimeUnit:   budgetstypes.TimeUnit(b.Spec.TimeUnit),
		BudgetLimit: &budgetstypes.Spend{
			Amount: aws.String(b.Spec.LimitAmount),
			Unit:   aws.String(b.Spec.LimitUnit),
		},
	}
	if len(b.Spec.CostFilters) > 0 {
		budget.CostFilters = b.Spec.CostFilters
	}
	return budget
}

func (r *BudgetReconciler) buildNotifications(b *awsv1alpha1.Budget) ([]budgetstypes.NotificationWithSubscribers, error) {
	var out []budgetstypes.NotificationWithSubscribers
	for _, n := range b.Spec.Notifications {
		threshold, err := strconv.ParseFloat(n.Threshold, 64)
		if err != nil {
			return nil, fmt.Errorf("parse notification threshold %q: %w", n.Threshold, err)
		}
		notif := budgetstypes.Notification{
			NotificationType:   budgetstypes.NotificationType(n.NotificationType),
			ComparisonOperator: budgetstypes.ComparisonOperator(n.ComparisonOperator),
			Threshold:          threshold,
		}
		if n.ThresholdType != "" {
			notif.ThresholdType = budgetstypes.ThresholdType(n.ThresholdType)
		}
		subs := make([]budgetstypes.Subscriber, 0, len(n.Subscribers))
		for _, s := range n.Subscribers {
			subs = append(subs, budgetstypes.Subscriber{
				Address:          aws.String(s.Address),
				SubscriptionType: budgetstypes.SubscriptionType(s.SubscriptionType),
			})
		}
		out = append(out, budgetstypes.NotificationWithSubscribers{
			Notification: &notif,
			Subscribers:  subs,
		})
	}
	return out, nil
}

func (r *BudgetReconciler) reconcileBudget(ctx context.Context, b *awsv1alpha1.Budget) error {
	_, err := r.BudgetsClient.DescribeBudget(ctx, &awsbudgets.DescribeBudgetInput{
		AccountId:  aws.String(b.Spec.AccountID),
		BudgetName: aws.String(b.Spec.BudgetName),
	})
	if budgetshelper.IsNotFound(err) {
		notifications, nerr := r.buildNotifications(b)
		if nerr != nil {
			return nerr
		}
		if _, err := r.BudgetsClient.CreateBudget(ctx, &awsbudgets.CreateBudgetInput{
			AccountId:                    aws.String(b.Spec.AccountID),
			Budget:                       r.buildBudget(b),
			NotificationsWithSubscribers: notifications,
		}); err != nil {
			return fmt.Errorf("create budget: %w", err)
		}
		b.Status.BudgetName = b.Spec.BudgetName
		// Persist the identifier immediately: the AWS resource now exists.
		if err := persistStatus(ctx, r.Client, b); err != nil {
			return fmt.Errorf("persist budget name after create: %w", err)
		}
	} else if err != nil {
		return err
	} else {
		b.Status.BudgetName = b.Spec.BudgetName
		if b.Status.ObservedGeneration != b.Generation {
			if _, err := r.BudgetsClient.UpdateBudget(ctx, &awsbudgets.UpdateBudgetInput{
				AccountId: aws.String(b.Spec.AccountID),
				NewBudget: r.buildBudget(b),
			}); err != nil {
				return fmt.Errorf("update budget: %w", err)
			}
			// Notification changes are not synced after creation: UpdateBudget
			// only covers the budget itself, and reconciling notification and
			// subscriber sets would require the Create/Update/Delete
			// Notification and Subscriber APIs plus a diff against AWS state.
			// Notifications provided at create time remain in effect.
		}
	}

	b.Status.ObservedGeneration = b.Generation
	now := metav1.Now()
	b.Status.LastSyncTime = &now
	return r.setCondition(ctx, b, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "budget reconciled")
}

func (r *BudgetReconciler) deleteBudget(ctx context.Context, b *awsv1alpha1.Budget) error {
	// Account ID + budget name in spec are the deterministic AWS identifier.
	_, err := r.BudgetsClient.DeleteBudget(ctx, &awsbudgets.DeleteBudgetInput{
		AccountId:  aws.String(b.Spec.AccountID),
		BudgetName: aws.String(b.Spec.BudgetName),
	})
	if budgetshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *BudgetReconciler) setCondition(ctx context.Context, b *awsv1alpha1.Budget, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&b.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: b.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, b); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *BudgetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.Budget{}).
		Complete(r)
}
