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

// KMSAliasReconciler reconciles KMSAlias objects.
type KMSAliasReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	KMSClient *awskms.Client
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

	if a.Status.ARN != "" {
		_, err2 := r.KMSClient.UpdateAlias(ctx, &awskms.UpdateAliasInput{
			AliasName:   aws.String(a.Spec.AliasName),
			TargetKeyId: aws.String(keyID),
		})
		if err2 != nil && !kmshelper.IsNotFound(err2) {
			return fmt.Errorf("update alias: %w", err2)
		}
		if err2 == nil {
			a.Status.ObservedGeneration = a.Generation
			now := metav1.Now()
			a.Status.LastSyncTime = &now
			return r.setConditionAlias(ctx, a, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "KMSAlias reconciled")
		}
		a.Status.ARN = ""
	}

	_, err = r.KMSClient.CreateAlias(ctx, &awskms.CreateAliasInput{
		AliasName:   aws.String(a.Spec.AliasName),
		TargetKeyId: aws.String(keyID),
	})
	if err != nil {
		return fmt.Errorf("create alias: %w", err)
	}

	out, err := r.KMSClient.ListAliases(ctx, &awskms.ListAliasesInput{
		KeyId: aws.String(keyID),
	})
	if err == nil {
		for _, al := range out.Aliases {
			if aws.ToString(al.AliasName) == a.Spec.AliasName {
				a.Status.ARN = aws.ToString(al.AliasArn)
			}
		}
	}

	a.Status.ObservedGeneration = a.Generation
	now := metav1.Now()
	a.Status.LastSyncTime = &now
	return r.setConditionAlias(ctx, a, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "KMSAlias created")
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
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.KMSAlias{}).
		Complete(r)
}
