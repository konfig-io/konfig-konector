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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	smhelper "github.com/konfig-io/konfig-konector/internal/aws/secretsmanager"
)

// SecretAWSAPI is the subset of the Secrets Manager API used by this controller.
type SecretAWSAPI interface {
	DescribeSecret(ctx context.Context, params *awssm.DescribeSecretInput, optFns ...func(*awssm.Options)) (*awssm.DescribeSecretOutput, error)
	CreateSecret(ctx context.Context, params *awssm.CreateSecretInput, optFns ...func(*awssm.Options)) (*awssm.CreateSecretOutput, error)
	GetSecretValue(ctx context.Context, params *awssm.GetSecretValueInput, optFns ...func(*awssm.Options)) (*awssm.GetSecretValueOutput, error)
	PutSecretValue(ctx context.Context, params *awssm.PutSecretValueInput, optFns ...func(*awssm.Options)) (*awssm.PutSecretValueOutput, error)
	DeleteSecret(ctx context.Context, params *awssm.DeleteSecretInput, optFns ...func(*awssm.Options)) (*awssm.DeleteSecretOutput, error)
}

// SecretReconciler reconciles Secret objects.
type SecretReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	SecretsManagerClient SecretAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=secrets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=secrets/finalizers,verbs=update

func (r *SecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	s := &awsv1alpha1.Secret{}
	if err := r.Get(ctx, req.NamespacedName, s); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, s); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !s.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(s, awsv1alpha1.FinalizerName) {
			if shouldAbandon(s) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(s, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, s)
			}
			if err := r.deleteSecret(ctx, s); err != nil {
				logger.Error(err, "failed to delete Secret")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(s, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, s)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(s, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(s, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, s); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileSecret(ctx, s); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionSecret(ctx, s, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *SecretReconciler) reconcileSecret(ctx context.Context, s *awsv1alpha1.Secret) error {
	if s.Status.ARN != "" {
		out, err := r.SecretsManagerClient.DescribeSecret(ctx, &awssm.DescribeSecretInput{
			SecretId: aws.String(s.Status.ARN),
		})
		if err != nil && !smhelper.IsNotFound(err) {
			return fmt.Errorf("describe secret: %w", err)
		}
		if err == nil && out.ARN != nil {
			if err := r.syncSecretValue(ctx, s); err != nil {
				return err
			}
			s.Status.ObservedGeneration = s.Generation
			now := metav1.Now()
			s.Status.LastSyncTime = &now
			return r.setConditionSecret(ctx, s, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Secret reconciled")
		}
		s.Status.ARN = ""
	}

	input := &awssm.CreateSecretInput{
		Name: aws.String(s.Spec.SecretName),
	}
	if s.Spec.SecretStringRef != nil {
		val, err := resolveSecretValue(ctx, r.Client, s.Namespace, *s.Spec.SecretStringRef)
		if err != nil {
			return fmt.Errorf("resolve secretStringRef: %w", err)
		}
		input.SecretString = aws.String(val)
	}
	if s.Spec.Description != "" {
		input.Description = aws.String(s.Spec.Description)
	}
	if s.Spec.KMSKeyARN != "" {
		input.KmsKeyId = aws.String(s.Spec.KMSKeyARN)
	}
	if len(s.Spec.Tags) > 0 {
		tags := make([]smtypes.Tag, 0, len(s.Spec.Tags))
		for k, v := range s.Spec.Tags {
			k, v := k, v
			tags = append(tags, smtypes.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.SecretsManagerClient.CreateSecret(ctx, input)
	if err != nil {
		return fmt.Errorf("create secret: %w", err)
	}

	s.Status.ARN = aws.ToString(out.ARN)
	s.Status.ObservedGeneration = s.Generation
	now := metav1.Now()
	s.Status.LastSyncTime = &now
	return r.setConditionSecret(ctx, s, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "Secret created")
}

// syncSecretValue pushes the referenced Kubernetes Secret value to Secrets
// Manager when it differs from the current version, so rotations of the
// source Secret propagate without creating a new AWS version every resync.
func (r *SecretReconciler) syncSecretValue(ctx context.Context, s *awsv1alpha1.Secret) error {
	if s.Spec.SecretStringRef == nil {
		return nil
	}
	desired, err := resolveSecretValue(ctx, r.Client, s.Namespace, *s.Spec.SecretStringRef)
	if err != nil {
		return fmt.Errorf("resolve secretStringRef: %w", err)
	}
	cur, err := r.SecretsManagerClient.GetSecretValue(ctx, &awssm.GetSecretValueInput{
		SecretId: aws.String(s.Status.ARN),
	})
	// A secret created without a value has no version yet; treat as empty.
	if err != nil && !smhelper.IsNotFound(err) {
		return fmt.Errorf("get secret value: %w", err)
	}
	if err == nil && aws.ToString(cur.SecretString) == desired {
		return nil
	}
	if _, err := r.SecretsManagerClient.PutSecretValue(ctx, &awssm.PutSecretValueInput{
		SecretId:     aws.String(s.Status.ARN),
		SecretString: aws.String(desired),
	}); err != nil {
		return fmt.Errorf("put secret value: %w", err)
	}
	return nil
}

func (r *SecretReconciler) deleteSecret(ctx context.Context, s *awsv1alpha1.Secret) error {
	if s.Status.ARN == "" {
		return nil
	}
	input := &awssm.DeleteSecretInput{
		SecretId: aws.String(s.Status.ARN),
	}
	// Immediate unrecoverable deletion only when explicitly requested;
	// otherwise use the spec window or fall back to the AWS 30-day default.
	if s.Spec.ForceDelete {
		input.ForceDeleteWithoutRecovery = aws.Bool(true)
	} else if s.Spec.RecoveryWindowInDays > 0 {
		input.RecoveryWindowInDays = aws.Int64(int64(s.Spec.RecoveryWindowInDays))
	}
	_, err := r.SecretsManagerClient.DeleteSecret(ctx, input)
	if smhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SecretReconciler) setConditionSecret(ctx context.Context, s *awsv1alpha1.Secret, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: s.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, s); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *SecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.Secret{}).
		Complete(r)
}
