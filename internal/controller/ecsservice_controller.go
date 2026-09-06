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

	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ecshelper "github.com/konfig-io/konfig-konector/internal/aws/ecs"
)

// ECSServiceAWSAPI is the subset of the ECS client used by the ECSService
// controller (via the ecshelper service functions). *ecs.Client satisfies it.
type ECSServiceAWSAPI interface {
	DescribeServices(ctx context.Context, params *awsecs.DescribeServicesInput, optFns ...func(*awsecs.Options)) (*awsecs.DescribeServicesOutput, error)
	CreateService(ctx context.Context, params *awsecs.CreateServiceInput, optFns ...func(*awsecs.Options)) (*awsecs.CreateServiceOutput, error)
	UpdateService(ctx context.Context, params *awsecs.UpdateServiceInput, optFns ...func(*awsecs.Options)) (*awsecs.UpdateServiceOutput, error)
	DeleteService(ctx context.Context, params *awsecs.DeleteServiceInput, optFns ...func(*awsecs.Options)) (*awsecs.DeleteServiceOutput, error)
}

// ECSServiceReconciler reconciles ECSService objects.
type ECSServiceReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	ECSClient ECSServiceAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecsservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecsservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecsservices/finalizers,verbs=update

func (r *ECSServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	svc := &awsv1alpha1.ECSService{}
	if err := r.Get(ctx, req.NamespacedName, svc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, svc); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !svc.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(svc, awsv1alpha1.FinalizerName) {
			if shouldAbandon(svc) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(svc, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, svc)
			}
			clusterName, err := resolveECSClusterName(ctx, r.Client, svc.Namespace, svc.Spec.ClusterName, svc.Spec.ClusterRef)
			if err != nil && !errors.As(err, new(*dependencyNotReady)) && !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			if clusterName != "" && svc.Status.ServiceARN != "" {
				if err := ecshelper.DeleteService(ctx, r.ECSClient, clusterName, svc.Spec.ServiceName); err != nil && !ecshelper.IsNotFound(err) {
					logger.Error(err, "failed to delete ECS service")
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(svc, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, svc)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(svc, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(svc, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, svc); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	result, err := r.reconcileService(ctx, svc)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, svc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *ECSServiceReconciler) reconcileService(ctx context.Context, svc *awsv1alpha1.ECSService) (ctrl.Result, error) {
	clusterName, err := resolveECSClusterName(ctx, r.Client, svc.Namespace, svc.Spec.ClusterName, svc.Spec.ClusterRef)
	if err != nil {
		return ctrl.Result{}, err
	}

	taskDefArn, err := resolveECSTaskDefinitionArn(ctx, r.Client, svc.Namespace, svc.Spec.TaskDefinitionArn, svc.Spec.TaskDefinitionRef)
	if err != nil {
		return ctrl.Result{}, err
	}

	in, err := r.buildServiceInput(ctx, svc, clusterName, taskDefArn)
	if err != nil {
		return ctrl.Result{}, err
	}

	if svc.Status.ServiceARN != "" {
		existing, err := ecshelper.DescribeService(ctx, r.ECSClient, clusterName, svc.Spec.ServiceName)
		if err != nil {
			return ctrl.Result{}, err
		}
		if existing != nil {
			if existing.Status != nil {
				svc.Status.Status = *existing.Status
			}
			svc.Status.RunningCount = existing.RunningCount
			svc.Status.PendingCount = existing.PendingCount

			if svc.Status.Status == "DRAINING" {
				_ = r.setCondition(ctx, svc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Draining", "ECS service is DRAINING")
				return requeueDependency, nil
			}

			if svc.Status.ObservedGeneration != svc.Generation {
				if err := ecshelper.UpdateService(ctx, r.ECSClient, in); err != nil {
					return ctrl.Result{}, fmt.Errorf("update ECS service: %w", err)
				}
			}

			svc.Status.ObservedGeneration = svc.Generation
			now := metav1.Now()
			svc.Status.LastSyncTime = &now
			return requeueResult(), r.setCondition(ctx, svc, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ECS service active")
		}
	}

	created, err := ecshelper.CreateService(ctx, r.ECSClient, in)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create ECS service: %w", err)
	}
	if created.ServiceArn != nil {
		svc.Status.ServiceARN = *created.ServiceArn
	}
	if created.Status != nil {
		svc.Status.Status = *created.Status
	}
	svc.Status.RunningCount = created.RunningCount
	svc.Status.PendingCount = created.PendingCount
	svc.Status.ObservedGeneration = svc.Generation
	now := metav1.Now()
	svc.Status.LastSyncTime = &now
	return requeueResult(), r.setCondition(ctx, svc, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ECS service created")
}

func (r *ECSServiceReconciler) buildServiceInput(ctx context.Context, svc *awsv1alpha1.ECSService, clusterName, taskDefArn string) (ecshelper.CreateServiceInput, error) {
	in := ecshelper.CreateServiceInput{
		ClusterName:       clusterName,
		ServiceName:       svc.Spec.ServiceName,
		TaskDefinitionArn: taskDefArn,
		DesiredCount:      svc.Spec.DesiredCount,
		Tags:              svc.Spec.Tags,
	}
	if svc.Spec.LaunchType != "" {
		in.LaunchType = types.LaunchType(svc.Spec.LaunchType)
	}
	if svc.Spec.HealthCheckGracePeriodSeconds != nil {
		in.HealthCheckGracePeriodSeconds = svc.Spec.HealthCheckGracePeriodSeconds
	}
	if svc.Spec.EnableExecuteCommand != nil && *svc.Spec.EnableExecuteCommand {
		in.EnableExecuteCommand = true
	}

	if nc := svc.Spec.NetworkConfiguration; nc != nil {
		subnetIDs, err := resolveSubnetIDs(ctx, r.Client, svc.Namespace, nc.SubnetRefs)
		if err != nil {
			return in, err
		}
		sgIDs, err := resolveSGIDs(ctx, r.Client, svc.Namespace, nc.SecurityGroupRefs)
		if err != nil {
			return in, err
		}
		in.SubnetIDs = subnetIDs
		in.SecurityGroupIDs = sgIDs
		if nc.AssignPublicIP != "" {
			in.AssignPublicIP = types.AssignPublicIp(nc.AssignPublicIP)
		}
	}

	for _, lb := range svc.Spec.LoadBalancers {
		lb := lb
		in.LoadBalancers = append(in.LoadBalancers, types.LoadBalancer{
			TargetGroupArn: &lb.TargetGroupArn,
			ContainerName:  &lb.ContainerName,
			ContainerPort:  &lb.ContainerPort,
		})
	}

	return in, nil
}

func (r *ECSServiceReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.ECSService, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *ECSServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ECSService{}).
		Complete(r)
}
