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
	awskms "github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	kmshelper "github.com/konfig-io/konfig-konector/internal/aws/kms"
)

// KMSKeyAWSAPI is the subset of the KMS SDK client used by this controller.
// *kms.Client satisfies it.
type KMSKeyAWSAPI interface {
	DescribeKey(ctx context.Context, params *awskms.DescribeKeyInput, optFns ...func(*awskms.Options)) (*awskms.DescribeKeyOutput, error)
	CreateKey(ctx context.Context, params *awskms.CreateKeyInput, optFns ...func(*awskms.Options)) (*awskms.CreateKeyOutput, error)
	UpdateKeyDescription(ctx context.Context, params *awskms.UpdateKeyDescriptionInput, optFns ...func(*awskms.Options)) (*awskms.UpdateKeyDescriptionOutput, error)
	GetKeyPolicy(ctx context.Context, params *awskms.GetKeyPolicyInput, optFns ...func(*awskms.Options)) (*awskms.GetKeyPolicyOutput, error)
	PutKeyPolicy(ctx context.Context, params *awskms.PutKeyPolicyInput, optFns ...func(*awskms.Options)) (*awskms.PutKeyPolicyOutput, error)
	GetKeyRotationStatus(ctx context.Context, params *awskms.GetKeyRotationStatusInput, optFns ...func(*awskms.Options)) (*awskms.GetKeyRotationStatusOutput, error)
	EnableKeyRotation(ctx context.Context, params *awskms.EnableKeyRotationInput, optFns ...func(*awskms.Options)) (*awskms.EnableKeyRotationOutput, error)
	DisableKeyRotation(ctx context.Context, params *awskms.DisableKeyRotationInput, optFns ...func(*awskms.Options)) (*awskms.DisableKeyRotationOutput, error)
	ScheduleKeyDeletion(ctx context.Context, params *awskms.ScheduleKeyDeletionInput, optFns ...func(*awskms.Options)) (*awskms.ScheduleKeyDeletionOutput, error)
}

// KMSKeyReconciler reconciles KMSKey objects.
type KMSKeyReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	KMSClient KMSKeyAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=kmskeys,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=kmskeys/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=kmskeys/finalizers,verbs=update

func (r *KMSKeyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	k := &awsv1alpha1.KMSKey{}
	if err := r.Get(ctx, req.NamespacedName, k); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, k); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !k.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(k, awsv1alpha1.FinalizerName) {
			if shouldAbandon(k) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(k, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, k)
			}
			if err := r.deleteKMSKey(ctx, k); err != nil {
				logger.Error(err, "failed to delete KMSKey")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(k, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, k)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(k, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(k, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, k); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileKMSKey(ctx, k); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionKMSKey(ctx, k, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *KMSKeyReconciler) reconcileKMSKey(ctx context.Context, k *awsv1alpha1.KMSKey) error {
	if k.Status.KeyID != "" {
		out, err := r.KMSClient.DescribeKey(ctx, &awskms.DescribeKeyInput{
			KeyId: aws.String(k.Status.KeyID),
		})
		if err != nil && !kmshelper.IsNotFound(err) {
			return fmt.Errorf("describe key: %w", err)
		}
		if err == nil {
			k.Status.KeyState = string(out.KeyMetadata.KeyState)
			if k.Status.ObservedGeneration != k.Generation {
				if err := r.updateKMSKey(ctx, k, out.KeyMetadata); err != nil {
					return err
				}
			}
			k.Status.ObservedGeneration = k.Generation
			now := metav1.Now()
			k.Status.LastSyncTime = &now
			return r.setConditionKMSKey(ctx, k, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "KMSKey reconciled")
		}
		k.Status.KeyID = ""
		k.Status.ARN = ""
	}

	input := &awskms.CreateKeyInput{}
	if k.Spec.Description != "" {
		input.Description = aws.String(k.Spec.Description)
	}
	if k.Spec.KeyUsage != "" {
		input.KeyUsage = kmstypes.KeyUsageType(k.Spec.KeyUsage)
	}
	if k.Spec.KeySpec != "" {
		input.KeySpec = kmstypes.KeySpec(k.Spec.KeySpec)
	}
	if k.Spec.Policy != "" {
		input.Policy = aws.String(k.Spec.Policy)
	}
	if len(k.Spec.Tags) > 0 {
		tags := make([]kmstypes.Tag, 0, len(k.Spec.Tags))
		for key, val := range k.Spec.Tags {
			key, val := key, val
			tags = append(tags, kmstypes.Tag{TagKey: &key, TagValue: &val})
		}
		input.Tags = tags
	}

	out, err := r.KMSClient.CreateKey(ctx, input)
	if err != nil {
		return fmt.Errorf("create key: %w", err)
	}

	k.Status.KeyID = aws.ToString(out.KeyMetadata.KeyId)
	k.Status.ARN = aws.ToString(out.KeyMetadata.Arn)
	k.Status.KeyState = string(out.KeyMetadata.KeyState)
	// Persist the KeyID immediately: CreateKey has no name and creation is gated
	// on Status.KeyID == "", so a lost KeyID would mint a brand new key on every
	// retry (e.g. if EnableKeyRotation below fails).
	if err := persistStatus(ctx, r.Client, k); err != nil {
		return fmt.Errorf("persist key ID after create: %w", err)
	}

	if k.Spec.EnableKeyRotation {
		if _, err2 := r.KMSClient.EnableKeyRotation(ctx, &awskms.EnableKeyRotationInput{
			KeyId: aws.String(k.Status.KeyID),
		}); err2 != nil {
			return fmt.Errorf("enable key rotation: %w", err2)
		}
	}

	k.Status.ObservedGeneration = k.Generation
	now := metav1.Now()
	k.Status.LastSyncTime = &now
	return r.setConditionKMSKey(ctx, k, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "KMSKey created")
}

// updateKMSKey applies in-place updates for the mutable fields the spec models:
// description, key policy, and rotation.
func (r *KMSKeyReconciler) updateKMSKey(ctx context.Context, k *awsv1alpha1.KMSKey, actual *kmstypes.KeyMetadata) error {
	if k.Spec.Description != aws.ToString(actual.Description) {
		if _, err := r.KMSClient.UpdateKeyDescription(ctx, &awskms.UpdateKeyDescriptionInput{
			KeyId:       aws.String(k.Status.KeyID),
			Description: aws.String(k.Spec.Description),
		}); err != nil {
			return fmt.Errorf("update key description: %w", err)
		}
	}

	if k.Spec.Policy != "" {
		polOut, err := r.KMSClient.GetKeyPolicy(ctx, &awskms.GetKeyPolicyInput{
			KeyId:      aws.String(k.Status.KeyID),
			PolicyName: aws.String("default"),
		})
		if err != nil {
			return fmt.Errorf("get key policy: %w", err)
		}
		if aws.ToString(polOut.Policy) != k.Spec.Policy {
			if _, err := r.KMSClient.PutKeyPolicy(ctx, &awskms.PutKeyPolicyInput{
				KeyId:      aws.String(k.Status.KeyID),
				PolicyName: aws.String("default"),
				Policy:     aws.String(k.Spec.Policy),
			}); err != nil {
				return fmt.Errorf("put key policy: %w", err)
			}
		}
	}

	// Automatic rotation is only supported for symmetric encryption keys;
	// querying rotation status on other key specs returns an error.
	if k.Spec.KeySpec != "" && k.Spec.KeySpec != "SYMMETRIC_DEFAULT" {
		return nil
	}
	rotOut, err := r.KMSClient.GetKeyRotationStatus(ctx, &awskms.GetKeyRotationStatusInput{
		KeyId: aws.String(k.Status.KeyID),
	})
	if err != nil {
		return fmt.Errorf("get key rotation status: %w", err)
	}
	if k.Spec.EnableKeyRotation && !rotOut.KeyRotationEnabled {
		if _, err := r.KMSClient.EnableKeyRotation(ctx, &awskms.EnableKeyRotationInput{
			KeyId: aws.String(k.Status.KeyID),
		}); err != nil {
			return fmt.Errorf("enable key rotation: %w", err)
		}
	} else if !k.Spec.EnableKeyRotation && rotOut.KeyRotationEnabled {
		if _, err := r.KMSClient.DisableKeyRotation(ctx, &awskms.DisableKeyRotationInput{
			KeyId: aws.String(k.Status.KeyID),
		}); err != nil {
			return fmt.Errorf("disable key rotation: %w", err)
		}
	}
	return nil
}

func (r *KMSKeyReconciler) deleteKMSKey(ctx context.Context, k *awsv1alpha1.KMSKey) error {
	if k.Status.KeyID == "" {
		return nil
	}
	pendingDays := int32(30)
	if k.Spec.PendingWindowInDays > 0 {
		pendingDays = k.Spec.PendingWindowInDays
	}
	_, err := r.KMSClient.ScheduleKeyDeletion(ctx, &awskms.ScheduleKeyDeletionInput{
		KeyId:               aws.String(k.Status.KeyID),
		PendingWindowInDays: aws.Int32(pendingDays),
	})
	if kmshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *KMSKeyReconciler) setConditionKMSKey(ctx context.Context, k *awsv1alpha1.KMSKey, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&k.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: k.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, k); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *KMSKeyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.KMSKey{}).
		Complete(r)
}
