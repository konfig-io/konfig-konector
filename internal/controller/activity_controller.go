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

	"github.com/aws/aws-sdk-go-v2/aws"
	awssfn "github.com/aws/aws-sdk-go-v2/service/sfn"
	sfntypes "github.com/aws/aws-sdk-go-v2/service/sfn/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	sfnhelper "github.com/konfig-io/konfig-konector/internal/aws/sfn"
)

// ActivityReconciler reconciles Activity objects.
type ActivityReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	SFNClient *awssfn.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=activities,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=activities/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=activities/finalizers,verbs=update

func (r *ActivityReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.Activity{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
			if shouldAbandon(obj) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, obj)
			}
			if err := r.deleteActivity(ctx, obj); err != nil {
				logger.Error(err, "failed to delete Activity")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, obj)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(obj, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileActivity(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionAct(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ActivityReconciler) reconcileActivity(ctx context.Context, obj *awsv1alpha1.Activity) error {
	// Activities are immutable: once created, just verify existence.
	if obj.Status.ActivityARN != "" {
		descOut, err := r.SFNClient.DescribeActivity(ctx, &awssfn.DescribeActivityInput{
			ActivityArn: aws.String(obj.Status.ActivityARN),
		})
		if err != nil && !sfnhelper.IsNotFound(err) {
			return fmt.Errorf("describe activity: %w", err)
		}
		if err == nil && descOut.ActivityArn != nil {
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionAct(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Activity reconciled")
		}
		obj.Status.ActivityARN = ""
	}

	input := &awssfn.CreateActivityInput{
		Name: aws.String(obj.Spec.Name),
	}
	if len(obj.Spec.Tags) > 0 {
		tags := make([]sfntypes.Tag, 0, len(obj.Spec.Tags))
		for k, v := range obj.Spec.Tags {
			k, v := k, v
			tags = append(tags, sfntypes.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.SFNClient.CreateActivity(ctx, input)
	if err != nil {
		return fmt.Errorf("create activity: %w", err)
	}

	obj.Status.ActivityARN = aws.ToString(out.ActivityArn)
	// Persist the ARN immediately: the AWS resource now exists, and losing the
	// identifier would orphan it or create a duplicate on retry.
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist activity ARN after create: %w", err)
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionAct(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "Activity created")
}

func (r *ActivityReconciler) deleteActivity(ctx context.Context, obj *awsv1alpha1.Activity) error {
	arn := obj.Status.ActivityARN
	if arn == "" {
		// Status may have been lost before it was persisted; fall back to
		// looking the activity up by its spec name.
		found, err := r.findActivityARNByName(ctx, obj.Spec.Name)
		if err != nil {
			return err
		}
		if found == "" {
			return nil
		}
		arn = found
	}
	_, err := r.SFNClient.DeleteActivity(ctx, &awssfn.DeleteActivityInput{
		ActivityArn: aws.String(arn),
	})
	if sfnhelper.IsNotFound(err) {
		return nil
	}
	return err
}

// findActivityARNByName lists Step Functions activities and returns the ARN of
// the one whose name matches exactly, or "" if none matches.
func (r *ActivityReconciler) findActivityARNByName(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", nil
	}
	var next *string
	for {
		out, err := r.SFNClient.ListActivities(ctx, &awssfn.ListActivitiesInput{NextToken: next})
		if err != nil {
			return "", err
		}
		for _, a := range out.Activities {
			if aws.ToString(a.Name) == name {
				return aws.ToString(a.ActivityArn), nil
			}
		}
		if out.NextToken == nil {
			return "", nil
		}
		next = out.NextToken
	}
}

func (r *ActivityReconciler) setConditionAct(ctx context.Context, obj *awsv1alpha1.Activity, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: obj.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *ActivityReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.Activity{}).
		Complete(r)
}
