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

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ec2helper "github.com/konfig-io/konfig-konector/internal/aws/ec2"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// KeyPairReconciler reconciles KeyPair objects.
type KeyPairReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client *multi.EC2
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=keypairs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=keypairs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=keypairs/finalizers,verbs=update

func (r *KeyPairReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	kp := &awsv1alpha1.KeyPair{}
	if err := r.Get(ctx, req.NamespacedName, kp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, kp); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !kp.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(kp, awsv1alpha1.FinalizerName) {
			if shouldAbandon(kp) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(kp, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, kp)
			}
			if err := r.deleteKeyPair(ctx, kp); err != nil {
				logger.Error(err, "failed to delete key pair")
				return ctrl.Result{}, err
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

	if err := r.reconcileKeyPair(ctx, kp); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, kp, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *KeyPairReconciler) reconcileKeyPair(ctx context.Context, kp *awsv1alpha1.KeyPair) error {
	// Check if already exists by name.
	out, err := r.EC2Client.DescribeKeyPairs(ctx, &awsec2.DescribeKeyPairsInput{
		KeyNames: []string{kp.Spec.KeyName},
	})
	if err != nil && !ec2helper.IsNotFound(err) {
		return err
	}

	if err == nil && len(out.KeyPairs) > 0 {
		existing := out.KeyPairs[0]
		kp.Status.KeyPairID = aws.ToString(existing.KeyPairId)

		// Sync tags.
		if err := ec2helper.SyncResourceTags(ctx, r.EC2Client, kp.Status.KeyPairID, "key-pair", kp.Spec.Tags); err != nil {
			return err
		}

		kp.Status.ObservedGeneration = kp.Generation
		now := metav1.Now()
		kp.Status.LastSyncTime = &now
		return r.setCondition(ctx, kp, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Key pair available")
	}

	// Create the key pair.
	input := &awsec2.ImportKeyPairInput{
		// ImportKeyPair doesn't exist for new key pairs; use CreateKeyPair.
		// We can't do ImportKeyPair without a public key, so we use CreateKeyPair.
		// The private key is returned once and not stored.
	}
	_ = input

	createInput := &awsec2.CreateKeyPairInput{
		KeyName: aws.String(kp.Spec.KeyName),
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeKeyPair,
				Tags:         ec2helper.TagsFromMap(kp.Spec.Tags),
			},
		},
	}
	if kp.Spec.KeyType != "" {
		createInput.KeyType = ec2types.KeyType(kp.Spec.KeyType)
	}

	createOut, err := r.EC2Client.CreateKeyPair(ctx, createInput)
	if err != nil {
		return err
	}

	kp.Status.KeyPairID = aws.ToString(createOut.KeyPairId)
	// Note: createOut.KeyMaterial contains the private key but we intentionally
	// do NOT store it in status. It is output-only at creation time.
	kp.Status.ObservedGeneration = kp.Generation
	now := metav1.Now()
	kp.Status.LastSyncTime = &now
	return r.setCondition(ctx, kp, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Key pair created")
}

func (r *KeyPairReconciler) deleteKeyPair(ctx context.Context, kp *awsv1alpha1.KeyPair) error {
	_, err := r.EC2Client.DeleteKeyPair(ctx, &awsec2.DeleteKeyPairInput{
		KeyName: aws.String(kp.Spec.KeyName),
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *KeyPairReconciler) setCondition(ctx context.Context, kp *awsv1alpha1.KeyPair, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *KeyPairReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.KeyPair{}).
		Complete(r)
}
