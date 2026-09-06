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
	awssm "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
	smhelper "github.com/konfig-io/konfig-konector/internal/aws/secretsmanager"
)

// SecretRotationReconciler reconciles SecretRotation objects.
type SecretRotationReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	SecretsManagerClient *multi.SecretsManager
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=secretrotations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=secretrotations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=secretrotations/finalizers,verbs=update

func (r *SecretRotationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	sr := &awsv1alpha1.SecretRotation{}
	if err := r.Get(ctx, req.NamespacedName, sr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, sr); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !sr.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(sr, awsv1alpha1.FinalizerName) {
			if shouldAbandon(sr) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(sr, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, sr)
			}
			if err := r.cancelRotation(ctx, sr); err != nil {
				logger.Error(err, "failed to cancel SecretRotation")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(sr, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, sr)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(sr, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(sr, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, sr); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileSecretRotation(ctx, sr); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionSR(ctx, sr, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *SecretRotationReconciler) reconcileSecretRotation(ctx context.Context, sr *awsv1alpha1.SecretRotation) error {
	secretARN, err := r.resolveSecretARN(ctx, sr)
	if err != nil {
		return err
	}

	out, err := r.SecretsManagerClient.RotateSecret(ctx, &awssm.RotateSecretInput{
		SecretId:          aws.String(secretARN),
		RotationLambdaARN: aws.String(sr.Spec.RotationLambdaARN),
		RotationRules: &smtypes.RotationRulesType{
			AutomaticallyAfterDays: aws.Int64(int64(sr.Spec.AutomaticallyAfterDays)),
		},
	})
	if err != nil {
		return fmt.Errorf("rotate secret: %w", err)
	}

	sr.Status.RotationEnabled = out.VersionId != nil
	sr.Status.ObservedGeneration = sr.Generation
	now := metav1.Now()
	sr.Status.LastSyncTime = &now
	return r.setConditionSR(ctx, sr, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "SecretRotation reconciled")
}

func (r *SecretRotationReconciler) resolveSecretARN(ctx context.Context, sr *awsv1alpha1.SecretRotation) (string, error) {
	ref := sr.Spec.SecretRef
	if ref.SecretARN != "" {
		return ref.SecretARN, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("secretRef requires name or secretArn")
	}
	sCR := &awsv1alpha1.Secret{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: sr.Namespace}, sCR); err != nil {
		return "", err
	}
	if sCR.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("Secret %s/%s has no ARN yet", sr.Namespace, ref.Name)}
	}
	return sCR.Status.ARN, nil
}

func (r *SecretRotationReconciler) cancelRotation(ctx context.Context, sr *awsv1alpha1.SecretRotation) error {
	secretARN, err := r.resolveSecretARN(ctx, sr)
	if err != nil {
		return nil
	}
	_, err = r.SecretsManagerClient.CancelRotateSecret(ctx, &awssm.CancelRotateSecretInput{
		SecretId: aws.String(secretARN),
	})
	if smhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SecretRotationReconciler) setConditionSR(ctx context.Context, sr *awsv1alpha1.SecretRotation, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&sr.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: sr.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, sr); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *SecretRotationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.SecretRotation{}).
		Complete(r)
}
