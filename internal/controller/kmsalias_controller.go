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
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awskms "github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	kmshelper "github.com/konfig-io/konfig-konector/internal/aws/kms"
)

// KMSAliasAWSAPI is the subset of the KMS SDK client used by this controller.
// *multi.KMS and *awskms.Client satisfy it.
type KMSAliasAWSAPI interface {
	ListAliases(ctx context.Context, params *awskms.ListAliasesInput, optFns ...func(*awskms.Options)) (*awskms.ListAliasesOutput, error)
	CreateAlias(ctx context.Context, params *awskms.CreateAliasInput, optFns ...func(*awskms.Options)) (*awskms.CreateAliasOutput, error)
	UpdateAlias(ctx context.Context, params *awskms.UpdateAliasInput, optFns ...func(*awskms.Options)) (*awskms.UpdateAliasOutput, error)
	DeleteAlias(ctx context.Context, params *awskms.DeleteAliasInput, optFns ...func(*awskms.Options)) (*awskms.DeleteAliasOutput, error)
}

// KMSAliasReconciler reconciles KMSAlias objects.
type KMSAliasReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	KMSClient KMSAliasAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=kmsalias,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=kmsalias/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=kmsalias/finalizers,verbs=update

func (r *KMSAliasReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	a := &awsv1alpha1.KMSAlias{}
	if err := r.Get(ctx, req.NamespacedName, a); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, a); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !a.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(a, awsv1alpha1.FinalizerName) {
			if shouldAbandon(a) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(a, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, a)
			}
			if err := r.deleteKMSAlias(ctx, a); err != nil {
				logger.Error(err, "failed to delete KMSAlias")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(a, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, a)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(a, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(a, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, a); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileKMSAlias(ctx, a); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionAlias(ctx, a, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *KMSAliasReconciler) reconcileKMSAlias(ctx context.Context, a *awsv1alpha1.KMSAlias) error {
	keyID, err := r.resolveKeyID(ctx, a)
	if err != nil {
		return err
	}

	// Always look the alias up by name first. The name comes from spec, so an
	// alias that already exists (created by a pass whose ARN lookup or status
	// write was lost, or pre-existing) is adopted instead of created again.
	existing, onDesiredKey, err := r.findAlias(ctx, a.Spec.AliasName, keyID)
	if err != nil {
		return err
	}

	reason, message := awsv1alpha1.ReasonSynced, "KMSAlias reconciled"
	switch {
	case existing == nil:
		_, err = r.KMSClient.CreateAlias(ctx, &awskms.CreateAliasInput{
			AliasName:   aws.String(a.Spec.AliasName),
			TargetKeyId: aws.String(keyID),
		})
		if err != nil {
			var exists *kmstypes.AlreadyExistsException
			if errors.As(err, &exists) {
				// ListAliases had not caught up with an earlier create. The
				// next pass finds the alias and adopts it.
				return &dependencyNotReady{msg: fmt.Sprintf("alias %s already exists but is not listed by KMS yet", a.Spec.AliasName)}
			}
			return fmt.Errorf("create alias: %w", err)
		}
		// Never report Ready without the ARN: wait until KMS lists the alias.
		existing, _, err = r.findAlias(ctx, a.Spec.AliasName, keyID)
		if err != nil {
			return err
		}
		if existing == nil {
			return &dependencyNotReady{msg: fmt.Sprintf("alias %s created but not listed by KMS yet", a.Spec.AliasName)}
		}
		reason, message = awsv1alpha1.ReasonCreated, "KMSAlias created"
	case !onDesiredKey:
		// The alias points at another key: this is the only case that needs
		// UpdateAlias.
		_, err = r.KMSClient.UpdateAlias(ctx, &awskms.UpdateAliasInput{
			AliasName:   aws.String(a.Spec.AliasName),
			TargetKeyId: aws.String(keyID),
		})
		if err != nil {
			return fmt.Errorf("update alias: %w", err)
		}
	}

	a.Status.ARN = aws.ToString(existing.AliasArn)
	a.Status.ObservedGeneration = a.Generation
	now := metav1.Now()
	a.Status.LastSyncTime = &now
	return r.setConditionAlias(ctx, a, awsv1alpha1.ConditionReady, metav1.ConditionTrue, reason, message)
}

// findAlias looks an alias up by exact name. It first asks for the aliases of
// the desired key (a short list; the steady-state path), and only when the
// alias is not there pages through every alias in the account and region to
// see whether it exists on another key. onDesiredKey reports whether the alias
// found already targets keyID.
func (r *KMSAliasReconciler) findAlias(ctx context.Context, name, keyID string) (alias *kmstypes.AliasListEntry, onDesiredKey bool, err error) {
	alias, err = r.listAliasByName(ctx, name, aws.String(keyID))
	if err != nil && !kmshelper.IsNotFound(err) {
		return nil, false, fmt.Errorf("list aliases for key: %w", err)
	}
	if err == nil && alias != nil {
		return alias, true, nil
	}
	alias, err = r.listAliasByName(ctx, name, nil)
	if err != nil {
		return nil, false, fmt.Errorf("list aliases: %w", err)
	}
	if alias == nil {
		return nil, false, nil
	}
	// The per-key listing can lag behind; compare the target directly too.
	return alias, aws.ToString(alias.TargetKeyId) == kmsKeyIDFromRef(keyID), nil
}

func (r *KMSAliasReconciler) listAliasByName(ctx context.Context, name string, keyID *string) (*kmstypes.AliasListEntry, error) {
	var marker *string
	for {
		out, err := r.KMSClient.ListAliases(ctx, &awskms.ListAliasesInput{KeyId: keyID, Marker: marker})
		if err != nil {
			return nil, err
		}
		for i := range out.Aliases {
			if aws.ToString(out.Aliases[i].AliasName) == name {
				return &out.Aliases[i], nil
			}
		}
		if !out.Truncated || aws.ToString(out.NextMarker) == "" {
			return nil, nil
		}
		marker = out.NextMarker
	}
}

// kmsKeyIDFromRef reduces a key ARN to the bare key ID that ListAliases
// reports in TargetKeyId; a bare key ID is returned unchanged.
func kmsKeyIDFromRef(ref string) string {
	if i := strings.LastIndex(ref, ":key/"); i >= 0 {
		return ref[i+len(":key/"):]
	}
	return ref
}

func (r *KMSAliasReconciler) resolveKeyID(ctx context.Context, a *awsv1alpha1.KMSAlias) (string, error) {
	ref := a.Spec.TargetKeyRef
	if ref.KeyID != "" {
		return ref.KeyID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("targetKeyRef requires name or keyId")
	}
	keyCR := &awsv1alpha1.KMSKey{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: a.Namespace}, keyCR); err != nil {
		return "", err
	}
	if keyCR.Status.KeyID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("KMSKey %s/%s has no keyId yet", a.Namespace, ref.Name)}
	}
	return keyCR.Status.KeyID, nil
}

func (r *KMSAliasReconciler) deleteKMSAlias(ctx context.Context, a *awsv1alpha1.KMSAlias) error {
	if a.Spec.AliasName == "" {
		return nil
	}
	_, err := r.KMSClient.DeleteAlias(ctx, &awskms.DeleteAliasInput{
		AliasName: aws.String(a.Spec.AliasName),
	})
	if kmshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *KMSAliasReconciler) setConditionAlias(ctx context.Context, a *awsv1alpha1.KMSAlias, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&a.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: a.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, a); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *KMSAliasReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return newKindController(mgr, &awsv1alpha1.KMSAlias{}).
		Complete(r)
}
