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

	"github.com/aws/aws-sdk-go-v2/aws"
	awslattice "github.com/aws/aws-sdk-go-v2/service/vpclattice"
	latticetypes "github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	latticehelper "github.com/konfig-io/konfig-konector/internal/aws/vpclattice"
)

// LatticeTargetGroupAWSAPI is the subset of the VPC Lattice API used by this controller.
type LatticeTargetGroupAWSAPI interface {
	GetTargetGroup(ctx context.Context, params *awslattice.GetTargetGroupInput, optFns ...func(*awslattice.Options)) (*awslattice.GetTargetGroupOutput, error)
	CreateTargetGroup(ctx context.Context, params *awslattice.CreateTargetGroupInput, optFns ...func(*awslattice.Options)) (*awslattice.CreateTargetGroupOutput, error)
	DeleteTargetGroup(ctx context.Context, params *awslattice.DeleteTargetGroupInput, optFns ...func(*awslattice.Options)) (*awslattice.DeleteTargetGroupOutput, error)
}

// LatticeTargetGroupReconciler reconciles LatticeTargetGroup objects.
type LatticeTargetGroupReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	LatticeClient LatticeTargetGroupAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=latticetargetgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=latticetargetgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=latticetargetgroups/finalizers,verbs=update

func (r *LatticeTargetGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.LatticeTargetGroup{}
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
			if err := r.deleteTargetGroup(ctx, obj); err != nil {
				logger.Error(err, "failed to delete LatticeTargetGroup")
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

	result, err := r.reconcileTargetGroup(ctx, obj)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *LatticeTargetGroupReconciler) reconcileTargetGroup(ctx context.Context, obj *awsv1alpha1.LatticeTargetGroup) (ctrl.Result, error) {
	if obj.Status.ID == "" {
		config, err := r.buildConfig(ctx, obj)
		if err != nil {
			return ctrl.Result{}, err
		}
		out, err := r.LatticeClient.CreateTargetGroup(ctx, &awslattice.CreateTargetGroupInput{
			Name:   aws.String(obj.Spec.Name),
			Type:   latticetypes.TargetGroupType(obj.Spec.Type),
			Config: config,
			Tags:   obj.Spec.Tags,
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("create target group: %w", err)
		}
		obj.Status.ID = aws.ToString(out.Id)
		obj.Status.ARN = aws.ToString(out.Arn)
		obj.Status.Status = string(out.Status)
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return ctrl.Result{}, fmt.Errorf("persist target group ID after create: %w", err)
		}
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Creating", "target group is being created")
		return requeueLatticePolling, nil
	}

	getOut, err := r.LatticeClient.GetTargetGroup(ctx, &awslattice.GetTargetGroupInput{
		TargetGroupIdentifier: aws.String(obj.Status.ID),
	})
	if latticehelper.IsNotFound(err) {
		obj.Status.ID = ""
		obj.Status.ARN = ""
		obj.Status.Status = ""
		return r.reconcileTargetGroup(ctx, obj)
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("get target group: %w", err)
	}
	obj.Status.ARN = aws.ToString(getOut.Arn)
	obj.Status.Status = string(getOut.Status)

	if getOut.Status != latticetypes.TargetGroupStatusActive {
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Creating", fmt.Sprintf("target group status is %s", getOut.Status))
		return requeueLatticePolling, nil
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	if err := r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "target group active"); err != nil {
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *LatticeTargetGroupReconciler) buildConfig(ctx context.Context, obj *awsv1alpha1.LatticeTargetGroup) (*latticetypes.TargetGroupConfig, error) {
	if obj.Spec.Config == nil {
		return nil, nil
	}
	cfg := &latticetypes.TargetGroupConfig{}
	if obj.Spec.Config.Port > 0 {
		cfg.Port = aws.Int32(obj.Spec.Config.Port)
	}
	if obj.Spec.Config.Protocol != "" {
		cfg.Protocol = latticetypes.TargetGroupProtocol(obj.Spec.Config.Protocol)
	}
	if obj.Spec.Config.VPCRef != nil {
		vpcID, err := r.resolveVPCIDLatticeTG(ctx, obj.Namespace, obj.Spec.Config.VPCRef)
		if err != nil {
			return nil, err
		}
		cfg.VpcIdentifier = aws.String(vpcID)
	}
	if hc := obj.Spec.Config.HealthCheck; hc != nil {
		hcCfg := &latticetypes.HealthCheckConfig{Enabled: aws.Bool(true)}
		if hc.Path != "" {
			hcCfg.Path = aws.String(hc.Path)
		}
		if hc.Protocol != "" {
			hcCfg.Protocol = latticetypes.TargetGroupProtocol(hc.Protocol)
		}
		cfg.HealthCheck = hcCfg
	}
	return cfg, nil
}

func (r *LatticeTargetGroupReconciler) resolveVPCIDLatticeTG(ctx context.Context, namespace string, ref *awsv1alpha1.VPCResourceRef) (string, error) {
	if ref.ID != "" {
		return ref.ID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("vpcRef requires name or id")
	}
	vpc := &awsv1alpha1.VPC{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, vpc); err != nil {
		return "", err
	}
	if vpc.Status.VPCID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("VPC %s/%s has no vpcId yet", namespace, ref.Name)}
	}
	return vpc.Status.VPCID, nil
}

func (r *LatticeTargetGroupReconciler) deleteTargetGroup(ctx context.Context, obj *awsv1alpha1.LatticeTargetGroup) error {
	if obj.Status.ID == "" {
		return nil
	}
	_, err := r.LatticeClient.DeleteTargetGroup(ctx, &awslattice.DeleteTargetGroupInput{
		TargetGroupIdentifier: aws.String(obj.Status.ID),
	})
	if latticehelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *LatticeTargetGroupReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.LatticeTargetGroup, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *LatticeTargetGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.LatticeTargetGroup{}).
		Complete(r)
}
