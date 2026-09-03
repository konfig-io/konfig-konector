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
)

// KMSKeyPolicyReconciler reconciles KMSKeyPolicy objects.
type KMSKeyPolicyReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	KMSClient *awskms.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=kmskeypolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=kmskeypolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=kmskeypolicies/finalizers,verbs=update

func (r *KMSKeyPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	kp := &awsv1alpha1.KMSKeyPolicy{}
	if err := r.Get(ctx, req.NamespacedName, kp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !kp.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(kp, awsv1alpha1.FinalizerName) {
			if shouldAbandon(kp) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(kp, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, kp)
			}
			controllerutil.RemoveFinalizer(kp, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, kp)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(kp, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(kp, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, kp); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileKMSKeyPolicy(ctx, kp); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionKP(ctx, kp, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *KMSKeyPolicyReconciler) reconcileKMSKeyPolicy(ctx context.Context, kp *awsv1alpha1.KMSKeyPolicy) error {
	keyID, err := r.resolveKeyIDForPolicy(ctx, kp)
	if err != nil {
		return err
	}

	policyName := "default"
	if kp.Spec.PolicyName != "" {
		policyName = kp.Spec.PolicyName
	}

	_, err = r.KMSClient.PutKeyPolicy(ctx, &awskms.PutKeyPolicyInput{
		KeyId:      aws.String(keyID),
		PolicyName: aws.String(policyName),
		Policy:     aws.String(kp.Spec.PolicyDocument),
	})
	if err != nil {
		return fmt.Errorf("put key policy: %w", err)
	}

	kp.Status.ObservedGeneration = kp.Generation
	now := metav1.Now()
	kp.Status.LastSyncTime = &now
	return r.setConditionKP(ctx, kp, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "KMSKeyPolicy reconciled")
}

func (r *KMSKeyPolicyReconciler) resolveKeyIDForPolicy(ctx context.Context, kp *awsv1alpha1.KMSKeyPolicy) (string, error) {
	ref := kp.Spec.KeyRef
	if ref.KeyID != "" {
		return ref.KeyID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("keyRef requires name or keyId")
	}
	keyCR := &awsv1alpha1.KMSKey{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: kp.Namespace}, keyCR); err != nil {
		return "", err
	}
	if keyCR.Status.KeyID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("KMSKey %s/%s has no keyId yet", kp.Namespace, ref.Name)}
	}
	return keyCR.Status.KeyID, nil
}

func (r *KMSKeyPolicyReconciler) setConditionKP(ctx context.Context, kp *awsv1alpha1.KMSKeyPolicy, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&kp.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: kp.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, kp); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *KMSKeyPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.KMSKeyPolicy{}).
		Complete(r)
}
