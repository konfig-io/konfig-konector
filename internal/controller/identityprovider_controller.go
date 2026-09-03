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
	awscognito "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	cognitotypes "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	cognitohelper "github.com/konfig-io/konfig-konector/internal/aws/cognito"
)

// IdentityProviderReconciler reconciles IdentityProvider objects.
type IdentityProviderReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	CognitoClient *awscognito.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=identityproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=identityproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=identityproviders/finalizers,verbs=update

func (r *IdentityProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	idp := &awsv1alpha1.IdentityProvider{}
	if err := r.Get(ctx, req.NamespacedName, idp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !idp.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(idp, awsv1alpha1.FinalizerName) {
			if shouldAbandon(idp) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(idp, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, idp)
			}
			if err := r.deleteIdentityProvider(ctx, idp); err != nil {
				logger.Error(err, "failed to delete IdentityProvider")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(idp, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, idp)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(idp, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(idp, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, idp); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileIdentityProvider(ctx, idp); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionIDP(ctx, idp, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *IdentityProviderReconciler) reconcileIdentityProvider(ctx context.Context, idp *awsv1alpha1.IdentityProvider) error {
	poolID, err := r.resolveUserPoolIDForIDP(ctx, idp.Spec.UserPoolRef, idp.Namespace)
	if err != nil {
		return err
	}

	_, err = r.CognitoClient.DescribeIdentityProvider(ctx, &awscognito.DescribeIdentityProviderInput{
		UserPoolId:   aws.String(poolID),
		ProviderName: aws.String(idp.Spec.ProviderName),
	})
	if err == nil {
		_, err2 := r.CognitoClient.UpdateIdentityProvider(ctx, &awscognito.UpdateIdentityProviderInput{
			UserPoolId:       aws.String(poolID),
			ProviderName:     aws.String(idp.Spec.ProviderName),
			ProviderDetails:  idp.Spec.ProviderDetails,
			AttributeMapping: idp.Spec.AttributeMapping,
			IdpIdentifiers:   idp.Spec.IdpIdentifiers,
		})
		if err2 != nil {
			return fmt.Errorf("update identity provider: %w", err2)
		}
		idp.Status.ObservedGeneration = idp.Generation
		now := metav1.Now()
		idp.Status.LastSyncTime = &now
		return r.setConditionIDP(ctx, idp, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "IdentityProvider reconciled")
	}

	_, err = r.CognitoClient.CreateIdentityProvider(ctx, &awscognito.CreateIdentityProviderInput{
		UserPoolId:       aws.String(poolID),
		ProviderName:     aws.String(idp.Spec.ProviderName),
		ProviderType:     cognitotypes.IdentityProviderTypeType(idp.Spec.ProviderType),
		ProviderDetails:  idp.Spec.ProviderDetails,
		AttributeMapping: idp.Spec.AttributeMapping,
		IdpIdentifiers:   idp.Spec.IdpIdentifiers,
	})
	if err != nil {
		return fmt.Errorf("create identity provider: %w", err)
	}

	idp.Status.ObservedGeneration = idp.Generation
	now := metav1.Now()
	idp.Status.LastSyncTime = &now
	return r.setConditionIDP(ctx, idp, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "IdentityProvider created")
}

func (r *IdentityProviderReconciler) resolveUserPoolIDForIDP(ctx context.Context, ref awsv1alpha1.UserPoolRef, namespace string) (string, error) {
	if ref.UserPoolID != "" {
		return ref.UserPoolID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("userPoolRef requires name or userPoolId")
	}
	upCR := &awsv1alpha1.UserPool{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, upCR); err != nil {
		return "", err
	}
	if upCR.Status.UserPoolID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("UserPool %s/%s has no userPoolId yet", namespace, ref.Name)}
	}
	return upCR.Status.UserPoolID, nil
}

func (r *IdentityProviderReconciler) deleteIdentityProvider(ctx context.Context, idp *awsv1alpha1.IdentityProvider) error {
	poolID, err := r.resolveUserPoolIDForIDP(ctx, idp.Spec.UserPoolRef, idp.Namespace)
	if err != nil {
		return nil
	}
	_, err = r.CognitoClient.DeleteIdentityProvider(ctx, &awscognito.DeleteIdentityProviderInput{
		UserPoolId:   aws.String(poolID),
		ProviderName: aws.String(idp.Spec.ProviderName),
	})
	if cognitohelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *IdentityProviderReconciler) setConditionIDP(ctx context.Context, idp *awsv1alpha1.IdentityProvider, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&idp.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: idp.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, idp); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *IdentityProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.IdentityProvider{}).
		Complete(r)
}
