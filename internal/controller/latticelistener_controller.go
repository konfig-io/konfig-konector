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

// LatticeListenerAWSAPI is the subset of the VPC Lattice API used by this controller.
type LatticeListenerAWSAPI interface {
	GetListener(ctx context.Context, params *awslattice.GetListenerInput, optFns ...func(*awslattice.Options)) (*awslattice.GetListenerOutput, error)
	CreateListener(ctx context.Context, params *awslattice.CreateListenerInput, optFns ...func(*awslattice.Options)) (*awslattice.CreateListenerOutput, error)
	UpdateListener(ctx context.Context, params *awslattice.UpdateListenerInput, optFns ...func(*awslattice.Options)) (*awslattice.UpdateListenerOutput, error)
	DeleteListener(ctx context.Context, params *awslattice.DeleteListenerInput, optFns ...func(*awslattice.Options)) (*awslattice.DeleteListenerOutput, error)
}

// LatticeListenerReconciler reconciles LatticeListener objects.
type LatticeListenerReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	LatticeClient LatticeListenerAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=latticelisteners,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=latticelisteners/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=latticelisteners/finalizers,verbs=update

func (r *LatticeListenerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.LatticeListener{}
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
			if err := r.deleteListener(ctx, obj); err != nil {
				logger.Error(err, "failed to delete LatticeListener")
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

	if err := r.reconcileListener(ctx, obj); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

// buildDefaultAction converts the CR default action into the SDK union type.
func (r *LatticeListenerReconciler) buildDefaultAction(ctx context.Context, obj *awsv1alpha1.LatticeListener) (latticetypes.RuleAction, error) {
	da := obj.Spec.DefaultAction
	if da.FixedResponse != nil {
		return &latticetypes.RuleActionMemberFixedResponse{
			Value: latticetypes.FixedResponseAction{StatusCode: aws.Int32(da.FixedResponse.StatusCode)},
		}, nil
	}
	if len(da.Forward) == 0 {
		return nil, fmt.Errorf("defaultAction requires forward or fixedResponse")
	}
	fwd := latticetypes.ForwardAction{}
	for _, t := range da.Forward {
		tgID, err := r.resolveTargetGroupID(ctx, obj.Namespace, t.TargetGroupRef)
		if err != nil {
			return nil, err
		}
		wtg := latticetypes.WeightedTargetGroup{TargetGroupIdentifier: aws.String(tgID)}
		if t.Weight > 0 {
			wtg.Weight = aws.Int32(t.Weight)
		}
		fwd.TargetGroups = append(fwd.TargetGroups, wtg)
	}
	return &latticetypes.RuleActionMemberForward{Value: fwd}, nil
}

func (r *LatticeListenerReconciler) resolveTargetGroupID(ctx context.Context, namespace string, ref awsv1alpha1.LatticeTargetGroupRef) (string, error) {
	if ref.ID != "" {
		return ref.ID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("targetGroupRef requires name or id")
	}
	tg := &awsv1alpha1.LatticeTargetGroup{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, tg); err != nil {
		return "", err
	}
	if tg.Status.ID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("LatticeTargetGroup %s/%s has no ID yet", namespace, ref.Name)}
	}
	return tg.Status.ID, nil
}

func (r *LatticeListenerReconciler) reconcileListener(ctx context.Context, obj *awsv1alpha1.LatticeListener) error {
	svcID, err := resolveLatticeServiceID(ctx, r.Client, obj.Namespace, obj.Spec.ServiceRef)
	if err != nil {
		return err
	}
	action, err := r.buildDefaultAction(ctx, obj)
	if err != nil {
		return err
	}

	if obj.Status.ID == "" {
		input := &awslattice.CreateListenerInput{
			ServiceIdentifier: aws.String(svcID),
			Name:              aws.String(obj.Spec.Name),
			Protocol:          latticetypes.ListenerProtocol(obj.Spec.Protocol),
			DefaultAction:     action,
			Tags:              obj.Spec.Tags,
		}
		if obj.Spec.Port > 0 {
			input.Port = aws.Int32(obj.Spec.Port)
		}
		out, err := r.LatticeClient.CreateListener(ctx, input)
		if err != nil {
			return fmt.Errorf("create listener: %w", err)
		}
		obj.Status.ID = aws.ToString(out.Id)
		obj.Status.ARN = aws.ToString(out.Arn)
		obj.Status.ServiceID = aws.ToString(out.ServiceId)
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return fmt.Errorf("persist listener ID after create: %w", err)
		}
		obj.Status.ObservedGeneration = obj.Generation
		now := metav1.Now()
		obj.Status.LastSyncTime = &now
		return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "LatticeListener created")
	}

	getOut, err := r.LatticeClient.GetListener(ctx, &awslattice.GetListenerInput{
		ServiceIdentifier:  aws.String(svcID),
		ListenerIdentifier: aws.String(obj.Status.ID),
	})
	if latticehelper.IsNotFound(err) {
		obj.Status.ID = ""
		obj.Status.ARN = ""
		return r.reconcileListener(ctx, obj)
	}
	if err != nil {
		return fmt.Errorf("get listener: %w", err)
	}
	obj.Status.ARN = aws.ToString(getOut.Arn)
	obj.Status.ServiceID = aws.ToString(getOut.ServiceId)

	if obj.Status.ObservedGeneration != obj.Generation {
		if _, err := r.LatticeClient.UpdateListener(ctx, &awslattice.UpdateListenerInput{
			ServiceIdentifier:  aws.String(svcID),
			ListenerIdentifier: aws.String(obj.Status.ID),
			DefaultAction:      action,
		}); err != nil {
			return fmt.Errorf("update listener: %w", err)
		}
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "LatticeListener reconciled")
}

func (r *LatticeListenerReconciler) deleteListener(ctx context.Context, obj *awsv1alpha1.LatticeListener) error {
	if obj.Status.ID == "" {
		return nil
	}
	svcID := obj.Status.ServiceID
	if svcID == "" {
		var err error
		svcID, err = resolveLatticeServiceID(ctx, r.Client, obj.Namespace, obj.Spec.ServiceRef)
		if err != nil {
			// The owning service may already be gone; deleting the service
			// deletes its listeners.
			return nil
		}
	}
	_, err := r.LatticeClient.DeleteListener(ctx, &awslattice.DeleteListenerInput{
		ServiceIdentifier:  aws.String(svcID),
		ListenerIdentifier: aws.String(obj.Status.ID),
	})
	if latticehelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *LatticeListenerReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.LatticeListener, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *LatticeListenerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.LatticeListener{}).
		Complete(r)
}
