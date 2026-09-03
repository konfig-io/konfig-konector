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

// UserPoolClientReconciler reconciles UserPoolClient objects.
type UserPoolClientReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	CognitoClient *awscognito.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=userpoolclients,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=userpoolclients/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=userpoolclients/finalizers,verbs=update

func (r *UserPoolClientReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	upc := &awsv1alpha1.UserPoolClient{}
	if err := r.Get(ctx, req.NamespacedName, upc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !upc.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(upc, awsv1alpha1.FinalizerName) {
			if shouldAbandon(upc) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(upc, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, upc)
			}
			if err := r.deleteUserPoolClient(ctx, upc); err != nil {
				logger.Error(err, "failed to delete UserPoolClient")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(upc, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, upc)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(upc, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(upc, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, upc); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileUserPoolClient(ctx, upc); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionUPC(ctx, upc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *UserPoolClientReconciler) reconcileUserPoolClient(ctx context.Context, upc *awsv1alpha1.UserPoolClient) error {
	poolID, err := r.resolveUserPoolID(ctx, upc.Spec.UserPoolRef, upc.Namespace)
	if err != nil {
		return err
	}

	if upc.Status.ClientID != "" {
		_, err2 := r.CognitoClient.DescribeUserPoolClient(ctx, &awscognito.DescribeUserPoolClientInput{
			UserPoolId: aws.String(poolID),
			ClientId:   aws.String(upc.Status.ClientID),
		})
		if err2 != nil && !cognitohelper.IsNotFound(err2) {
			return fmt.Errorf("describe user pool client: %w", err2)
		}
		if err2 == nil {
			upc.Status.ObservedGeneration = upc.Generation
			now := metav1.Now()
			upc.Status.LastSyncTime = &now
			return r.setConditionUPC(ctx, upc, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "UserPoolClient reconciled")
		}
		upc.Status.ClientID = ""
	}

	input := &awscognito.CreateUserPoolClientInput{
		UserPoolId:     aws.String(poolID),
		ClientName:     aws.String(upc.Spec.ClientName),
		GenerateSecret: upc.Spec.GenerateSecret,
	}
	if len(upc.Spec.ExplicitAuthFlows) > 0 {
		flows := make([]cognitotypes.ExplicitAuthFlowsType, 0, len(upc.Spec.ExplicitAuthFlows))
		for _, f := range upc.Spec.ExplicitAuthFlows {
			flows = append(flows, cognitotypes.ExplicitAuthFlowsType(f))
		}
		input.ExplicitAuthFlows = flows
	}
	if len(upc.Spec.AllowedOAuthFlows) > 0 {
		flows := make([]cognitotypes.OAuthFlowType, 0, len(upc.Spec.AllowedOAuthFlows))
		for _, f := range upc.Spec.AllowedOAuthFlows {
			flows = append(flows, cognitotypes.OAuthFlowType(f))
		}
		input.AllowedOAuthFlows = flows
	}
	if len(upc.Spec.AllowedOAuthScopes) > 0 {
		input.AllowedOAuthScopes = upc.Spec.AllowedOAuthScopes
	}
	if len(upc.Spec.CallbackURLs) > 0 {
		input.CallbackURLs = upc.Spec.CallbackURLs
	}
	if len(upc.Spec.LogoutURLs) > 0 {
		input.LogoutURLs = upc.Spec.LogoutURLs
	}
	if len(upc.Spec.SupportedIdentityProviders) > 0 {
		input.SupportedIdentityProviders = upc.Spec.SupportedIdentityProviders
	}
	if upc.Spec.AccessTokenValidity > 0 {
		input.AccessTokenValidity = aws.Int32(upc.Spec.AccessTokenValidity)
	}
	if upc.Spec.IdTokenValidity > 0 {
		input.IdTokenValidity = aws.Int32(upc.Spec.IdTokenValidity)
	}
	if upc.Spec.RefreshTokenValidity > 0 {
		input.RefreshTokenValidity = upc.Spec.RefreshTokenValidity
	}

	out, err := r.CognitoClient.CreateUserPoolClient(ctx, input)
	if err != nil {
		return fmt.Errorf("create user pool client: %w", err)
	}

	upc.Status.ClientID = aws.ToString(out.UserPoolClient.ClientId)
	upc.Status.ObservedGeneration = upc.Generation
	now := metav1.Now()
	upc.Status.LastSyncTime = &now
	return r.setConditionUPC(ctx, upc, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "UserPoolClient created")
}

func (r *UserPoolClientReconciler) resolveUserPoolID(ctx context.Context, ref awsv1alpha1.UserPoolRef, namespace string) (string, error) {
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

func (r *UserPoolClientReconciler) deleteUserPoolClient(ctx context.Context, upc *awsv1alpha1.UserPoolClient) error {
	if upc.Status.ClientID == "" {
		return nil
	}
	poolID, err := r.resolveUserPoolID(ctx, upc.Spec.UserPoolRef, upc.Namespace)
	if err != nil {
		return nil
	}
	_, err = r.CognitoClient.DeleteUserPoolClient(ctx, &awscognito.DeleteUserPoolClientInput{
		UserPoolId: aws.String(poolID),
		ClientId:   aws.String(upc.Status.ClientID),
	})
	if cognitohelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *UserPoolClientReconciler) setConditionUPC(ctx context.Context, upc *awsv1alpha1.UserPoolClient, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&upc.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: upc.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, upc); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *UserPoolClientReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.UserPoolClient{}).
		Complete(r)
}
