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
	awsrds "github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
	rdshelper "github.com/konfig-io/konfig-konector/internal/aws/rds"
)

// DBProxyReconciler reconciles DBProxy objects.
type DBProxyReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	RDSClient *multi.RDS
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=dbproxies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dbproxies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dbproxies/finalizers,verbs=update

func (r *DBProxyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	proxy := &awsv1alpha1.DBProxy{}
	if err := r.Get(ctx, req.NamespacedName, proxy); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, proxy); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !proxy.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(proxy, awsv1alpha1.FinalizerName) {
			if shouldAbandon(proxy) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(proxy, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, proxy)
			}
			if err := r.deleteDBProxy(ctx, proxy); err != nil {
				logger.Error(err, "failed to delete DBProxy")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(proxy, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, proxy)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(proxy, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(proxy, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, proxy); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileDBProxy(ctx, proxy); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionProxy(ctx, proxy, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *DBProxyReconciler) reconcileDBProxy(ctx context.Context, proxy *awsv1alpha1.DBProxy) error {
	if proxy.Status.DBProxyARN != "" {
		out, err := r.RDSClient.DescribeDBProxies(ctx, &awsrds.DescribeDBProxiesInput{
			DBProxyName: aws.String(proxy.Spec.DBProxyName),
		})
		if err != nil && !rdshelper.IsNotFound(err) {
			return fmt.Errorf("describe db proxy: %w", err)
		}
		if err == nil && len(out.DBProxies) > 0 {
			p := out.DBProxies[0]
			proxy.Status.Status = string(p.Status)
			proxy.Status.Endpoint = aws.ToString(p.Endpoint)
			if rdshelper.IsTransient(proxy.Status.Status) {
				return nil
			}
			proxy.Status.ObservedGeneration = proxy.Generation
			now := metav1.Now()
			proxy.Status.LastSyncTime = &now
			return r.setConditionProxy(ctx, proxy, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "DBProxy reconciled")
		}
		proxy.Status.DBProxyARN = ""
	}

	auth := make([]rdstypes.UserAuthConfig, 0, len(proxy.Spec.Auth))
	for _, a := range proxy.Spec.Auth {
		ua := rdstypes.UserAuthConfig{}
		if a.Description != "" {
			ua.Description = aws.String(a.Description)
		}
		if a.IAMAuth != "" {
			ua.IAMAuth = rdstypes.IAMAuthMode(a.IAMAuth)
		}
		if a.SecretARN != "" {
			ua.SecretArn = aws.String(a.SecretARN)
		}
		if a.AuthScheme != "" {
			ua.AuthScheme = rdstypes.AuthScheme(a.AuthScheme)
		}
		auth = append(auth, ua)
	}

	tags := make([]rdstypes.Tag, 0, len(proxy.Spec.Tags))
	for k, v := range proxy.Spec.Tags {
		k, v := k, v
		tags = append(tags, rdstypes.Tag{Key: &k, Value: &v})
	}

	input := &awsrds.CreateDBProxyInput{
		DBProxyName:  aws.String(proxy.Spec.DBProxyName),
		EngineFamily: rdstypes.EngineFamily(proxy.Spec.EngineFamily),
		Auth:         auth,
		RoleArn:      aws.String(proxy.Spec.RoleARN),
		VpcSubnetIds: proxy.Spec.VPCSubnetIDs,
		Tags:         tags,
	}
	if len(proxy.Spec.VPCSecurityGroupIDs) > 0 {
		input.VpcSecurityGroupIds = proxy.Spec.VPCSecurityGroupIDs
	}
	if proxy.Spec.RequireTLS {
		input.RequireTLS = aws.Bool(true)
	}
	if proxy.Spec.IdleClientTimeout > 0 {
		input.IdleClientTimeout = aws.Int32(proxy.Spec.IdleClientTimeout)
	}
	if proxy.Spec.DebugLogging {
		input.DebugLogging = aws.Bool(true)
	}

	out, err := r.RDSClient.CreateDBProxy(ctx, input)
	if err != nil {
		return fmt.Errorf("create db proxy: %w", err)
	}

	proxy.Status.DBProxyARN = aws.ToString(out.DBProxy.DBProxyArn)
	proxy.Status.Endpoint = aws.ToString(out.DBProxy.Endpoint)
	proxy.Status.Status = string(out.DBProxy.Status)
	proxy.Status.ObservedGeneration = proxy.Generation
	now := metav1.Now()
	proxy.Status.LastSyncTime = &now
	return r.setConditionProxy(ctx, proxy, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "DBProxy created")
}

func (r *DBProxyReconciler) deleteDBProxy(ctx context.Context, proxy *awsv1alpha1.DBProxy) error {
	if proxy.Status.DBProxyARN == "" {
		return nil
	}
	_, err := r.RDSClient.DeleteDBProxy(ctx, &awsrds.DeleteDBProxyInput{
		DBProxyName: aws.String(proxy.Spec.DBProxyName),
	})
	if rdshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *DBProxyReconciler) setConditionProxy(ctx context.Context, proxy *awsv1alpha1.DBProxy, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&proxy.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: proxy.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, proxy); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *DBProxyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.DBProxy{}).
		Complete(r)
}
