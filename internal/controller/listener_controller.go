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
	awselbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	elbv2helper "github.com/konfig-io/konfig-konector/internal/aws/elbv2"
)

// ListenerReconciler reconciles Listener objects.
type ListenerReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	ELBv2Client *awselbv2.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=listeners,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=listeners/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=listeners/finalizers,verbs=update

func (r *ListenerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	l := &awsv1alpha1.Listener{}
	if err := r.Get(ctx, req.NamespacedName, l); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !l.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(l, awsv1alpha1.FinalizerName) {
			if shouldAbandon(l) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(l, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, l)
			}
			if err := r.deleteListener(ctx, l); err != nil {
				logger.Error(err, "failed to delete Listener")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(l, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, l)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(l, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(l, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, l); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileListener(ctx, l); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionListener(ctx, l, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ListenerReconciler) reconcileListener(ctx context.Context, l *awsv1alpha1.Listener) error {
	if l.Status.ARN != "" {
		out, err := r.ELBv2Client.DescribeListeners(ctx, &awselbv2.DescribeListenersInput{
			ListenerArns: []string{l.Status.ARN},
		})
		if err != nil && !elbv2helper.IsNotFound(err) {
			return fmt.Errorf("describe listener: %w", err)
		}
		if err == nil && len(out.Listeners) > 0 {
			l.Status.ObservedGeneration = l.Generation
			now := metav1.Now()
			l.Status.LastSyncTime = &now
			return r.setConditionListener(ctx, l, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Listener reconciled")
		}
		l.Status.ARN = ""
	}

	lbARN, err := r.resolveLBARN(ctx, l)
	if err != nil {
		return err
	}

	actions, err := r.buildActions(ctx, l.Spec.DefaultActions, l.Namespace)
	if err != nil {
		return err
	}

	input := &awselbv2.CreateListenerInput{
		LoadBalancerArn: aws.String(lbARN),
		Protocol:        elbv2types.ProtocolEnum(l.Spec.Protocol),
		Port:            aws.Int32(l.Spec.Port),
		DefaultActions:  actions,
	}
	for _, certARN := range l.Spec.CertificateARNs {
		certARN := certARN
		input.Certificates = append(input.Certificates, elbv2types.Certificate{CertificateArn: &certARN})
	}
	if l.Spec.SSLPolicy != "" {
		input.SslPolicy = aws.String(l.Spec.SSLPolicy)
	}
	if len(l.Spec.Tags) > 0 {
		tags := make([]elbv2types.Tag, 0, len(l.Spec.Tags))
		for k, v := range l.Spec.Tags {
			k, v := k, v
			tags = append(tags, elbv2types.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.ELBv2Client.CreateListener(ctx, input)
	if err != nil {
		return fmt.Errorf("create listener: %w", err)
	}
	if len(out.Listeners) == 0 {
		return fmt.Errorf("create listener: empty response")
	}

	l.Status.ARN = aws.ToString(out.Listeners[0].ListenerArn)
	// The AWS resource now exists; losing the ARN would orphan it.
	if err := persistStatus(ctx, r.Client, l); err != nil {
		return fmt.Errorf("persist status after create: %w", err)
	}
	l.Status.ObservedGeneration = l.Generation
	now := metav1.Now()
	l.Status.LastSyncTime = &now
	return r.setConditionListener(ctx, l, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "Listener created")
}

func (r *ListenerReconciler) resolveLBARN(ctx context.Context, l *awsv1alpha1.Listener) (string, error) {
	ref := l.Spec.LoadBalancerRef
	if ref.ARN != "" {
		return ref.ARN, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("loadBalancerRef requires name or arn")
	}
	lbCR := &awsv1alpha1.LoadBalancer{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: l.Namespace}, lbCR); err != nil {
		return "", err
	}
	if lbCR.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("LoadBalancer %s/%s has no ARN yet", l.Namespace, ref.Name)}
	}
	return lbCR.Status.ARN, nil
}

func (r *ListenerReconciler) buildActions(ctx context.Context, specs []awsv1alpha1.ListenerDefaultAction, namespace string) ([]elbv2types.Action, error) {
	actions := make([]elbv2types.Action, 0, len(specs))
	for i, s := range specs {
		a := elbv2types.Action{
			Type:  elbv2types.ActionTypeEnum(s.Type),
			Order: aws.Int32(int32(i + 1)),
		}
		if s.TargetGroupRef != nil {
			tgARN, err := r.resolveTGARN(ctx, s.TargetGroupRef, namespace)
			if err != nil {
				return nil, err
			}
			a.ForwardConfig = &elbv2types.ForwardActionConfig{
				TargetGroups: []elbv2types.TargetGroupTuple{{TargetGroupArn: aws.String(tgARN)}},
			}
		}
		if rc := s.RedirectConfig; rc != nil {
			a.RedirectConfig = &elbv2types.RedirectActionConfig{
				StatusCode: elbv2types.RedirectActionStatusCodeEnum(rc.StatusCode),
				Host:       aws.String(rc.Host),
				Path:       aws.String(rc.Path),
				Port:       aws.String(rc.Port),
				Protocol:   aws.String(rc.Protocol),
			}
		}
		if fc := s.FixedResponseConfig; fc != nil {
			a.FixedResponseConfig = &elbv2types.FixedResponseActionConfig{
				StatusCode:  aws.String(fc.StatusCode),
				ContentType: aws.String(fc.ContentType),
				MessageBody: aws.String(fc.MessageBody),
			}
		}
		actions = append(actions, a)
	}
	return actions, nil
}

func (r *ListenerReconciler) resolveTGARN(ctx context.Context, ref *awsv1alpha1.TargetGroupRef, namespace string) (string, error) {
	if ref.ARN != "" {
		return ref.ARN, nil
	}
	tgCR := &awsv1alpha1.TargetGroup{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, tgCR); err != nil {
		return "", err
	}
	if tgCR.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("TargetGroup %s/%s has no ARN yet", namespace, ref.Name)}
	}
	return tgCR.Status.ARN, nil
}

func (r *ListenerReconciler) deleteListener(ctx context.Context, l *awsv1alpha1.Listener) error {
	if l.Status.ARN == "" {
		return nil
	}
	_, err := r.ELBv2Client.DeleteListener(ctx, &awselbv2.DeleteListenerInput{
		ListenerArn: aws.String(l.Status.ARN),
	})
	if elbv2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ListenerReconciler) setConditionListener(ctx context.Context, l *awsv1alpha1.Listener, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&l.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: l.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, l); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *ListenerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.Listener{}).
		Complete(r)
}
