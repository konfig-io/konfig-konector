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
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
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

// ECSTaskDefinitionReconciler reconciles ECSTaskDefinition objects.
type ECSTaskDefinitionReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	ECSClient *awsecs.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecstaskdefinitions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecstaskdefinitions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecstaskdefinitions/finalizers,verbs=update

func (r *ECSTaskDefinitionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	td := &awsv1alpha1.ECSTaskDefinition{}
	if err := r.Get(ctx, req.NamespacedName, td); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !td.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(td, awsv1alpha1.FinalizerName) {
			if shouldAbandon(td) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(td, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, td)
			}
			if td.Status.TaskDefinitionARN != "" {
				if err := ecshelper.DeregisterTaskDefinition(ctx, r.ECSClient, td.Status.TaskDefinitionARN); err != nil && !ecshelper.IsNotFound(err) {
					logger.Error(err, "failed to deregister task definition")
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(td, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, td)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(td, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(td, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, td); err != nil {
			return ctrl.Result{}, err
		}
	}

	result, err := r.reconcileTaskDefinition(ctx, td)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, td, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *ECSTaskDefinitionReconciler) reconcileTaskDefinition(ctx context.Context, td *awsv1alpha1.ECSTaskDefinition) (ctrl.Result, error) {
	specHash, err := hashSpec(td.Spec)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Only register a new revision when spec changes
	if td.Status.TaskDefinitionARN != "" && td.Status.SpecHash == specHash {
		td.Status.ObservedGeneration = td.Generation
		now := metav1.Now()
		td.Status.LastSyncTime = &now
		return requeueResult(), r.setCondition(ctx, td, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ECS task definition up to date")
	}

	in, err := r.buildTaskDefInput(ctx, td)
	if err != nil {
		return ctrl.Result{}, err
	}

	registered, err := ecshelper.RegisterTaskDefinition(ctx, r.ECSClient, in)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("register task definition: %w", err)
	}
	if registered.TaskDefinitionArn != nil {
		td.Status.TaskDefinitionARN = *registered.TaskDefinitionArn
	}
	if registered.Revision != 0 {
		td.Status.Revision = registered.Revision
	}
	td.Status.SpecHash = specHash
	// Persist the ARN immediately: the revision now exists in AWS, and losing
	// the identifier would orphan it and register a duplicate revision on retry.
	if err := persistStatus(ctx, r.Client, td); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist task definition ARN after register: %w", err)
	}
	td.Status.ObservedGeneration = td.Generation
	now := metav1.Now()
	td.Status.LastSyncTime = &now
	return requeueResult(), r.setCondition(ctx, td, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ECS task definition registered")
}

func (r *ECSTaskDefinitionReconciler) buildTaskDefInput(ctx context.Context, td *awsv1alpha1.ECSTaskDefinition) (ecshelper.TaskDefinitionInput, error) {
	in := ecshelper.TaskDefinitionInput{
		Family:      td.Spec.Family,
		CPU:         td.Spec.CPU,
		Memory:      td.Spec.Memory,
		Tags:        td.Spec.Tags,
		NetworkMode: types.NetworkMode(td.Spec.NetworkMode),
	}
	if in.NetworkMode == "" {
		in.NetworkMode = types.NetworkModeAwsvpc
	}

	var err error
	if td.Spec.ExecutionRoleArn != "" {
		in.ExecutionRoleArn = td.Spec.ExecutionRoleArn
	} else if td.Spec.ExecutionRoleRef != nil {
		in.ExecutionRoleArn, err = resolveIAMRoleARN(ctx, r.Client, td.Namespace, *td.Spec.ExecutionRoleRef)
		if err != nil {
			return in, err
		}
	}
	if td.Spec.TaskRoleArn != "" {
		in.TaskRoleArn = td.Spec.TaskRoleArn
	} else if td.Spec.TaskRoleRef != nil {
		in.TaskRoleArn, err = resolveIAMRoleARN(ctx, r.Client, td.Namespace, *td.Spec.TaskRoleRef)
		if err != nil {
			return in, err
		}
	}

	for _, cd := range td.Spec.ContainerDefinitions {
		c := types.ContainerDefinition{
			Name:    &cd.Name,
			Image:   &cd.Image,
			Cpu:     derefInt32(cd.CPU),
			Memory:  cd.Memory,
			Command: cd.Command,
		}
		if cd.Essential != nil {
			c.Essential = cd.Essential
		}
		for _, pm := range cd.PortMappings {
			proto := types.TransportProtocolTcp
			if pm.Protocol == "udp" {
				proto = types.TransportProtocolUdp
			}
			c.PortMappings = append(c.PortMappings, types.PortMapping{
				ContainerPort: &pm.ContainerPort,
				Protocol:      proto,
			})
		}
		if len(cd.Environment) > 0 {
			for k, v := range cd.Environment {
				k, v := k, v
				c.Environment = append(c.Environment, types.KeyValuePair{Name: &k, Value: &v})
			}
		}
		if cd.LogGroup != "" {
			c.LogConfiguration = &types.LogConfiguration{
				LogDriver: types.LogDriverAwslogs,
				Options: map[string]string{
					"awslogs-group":         cd.LogGroup,
					"awslogs-region":        "us-east-1",
					"awslogs-stream-prefix": cd.Name,
				},
			}
		}
		in.ContainerDefs = append(in.ContainerDefs, c)
	}
	return in, nil
}

func derefInt32(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

func hashSpec(spec awsv1alpha1.ECSTaskDefinitionSpec) (string, error) {
	b, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))[:16], nil
}

func (r *ECSTaskDefinitionReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.ECSTaskDefinition, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *ECSTaskDefinitionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ECSTaskDefinition{}).
		Complete(r)
}
