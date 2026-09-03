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
	awsglue "github.com/aws/aws-sdk-go-v2/service/glue"
	gluetypes "github.com/aws/aws-sdk-go-v2/service/glue/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	gluehelper "github.com/konfig-io/konfig-konector/internal/aws/glue"
)

// glueConnPasswordProperty is the ConnectionProperties key whose value must
// come exclusively from spec.passwordSecretRef, never inline in the spec.
const glueConnPasswordProperty = "PASSWORD"

// GlueConnectionAWSAPI is the subset of the Glue SDK client used by this
// controller. *awsglue.Client satisfies it.
type GlueConnectionAWSAPI interface {
	GetConnection(ctx context.Context, params *awsglue.GetConnectionInput, optFns ...func(*awsglue.Options)) (*awsglue.GetConnectionOutput, error)
	CreateConnection(ctx context.Context, params *awsglue.CreateConnectionInput, optFns ...func(*awsglue.Options)) (*awsglue.CreateConnectionOutput, error)
	UpdateConnection(ctx context.Context, params *awsglue.UpdateConnectionInput, optFns ...func(*awsglue.Options)) (*awsglue.UpdateConnectionOutput, error)
	DeleteConnection(ctx context.Context, params *awsglue.DeleteConnectionInput, optFns ...func(*awsglue.Options)) (*awsglue.DeleteConnectionOutput, error)
}

// GlueConnectionReconciler reconciles GlueConnection objects.
type GlueConnectionReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	GlueClient GlueConnectionAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=glueconnections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=glueconnections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=glueconnections/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

func (r *GlueConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	conn := &awsv1alpha1.GlueConnection{}
	if err := r.Get(ctx, req.NamespacedName, conn); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !conn.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(conn, awsv1alpha1.FinalizerName) {
			if shouldAbandon(conn) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(conn, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, conn)
			}
			if err := r.deleteConnection(ctx, conn); err != nil {
				logger.Error(err, "failed to delete Glue connection")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(conn, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, conn)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(conn, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(conn, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, conn); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileConnection(ctx, conn); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, conn, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

// connectionInput builds the ConnectionInput for create/update. The PASSWORD
// connection property is resolved from spec.passwordSecretRef; an inline
// PASSWORD in spec.connectionProperties is rejected so the secret never
// round-trips through the CR.
func (r *GlueConnectionReconciler) connectionInput(ctx context.Context, conn *awsv1alpha1.GlueConnection) (*gluetypes.ConnectionInput, error) {
	if _, ok := conn.Spec.ConnectionProperties[glueConnPasswordProperty]; ok {
		return nil, fmt.Errorf("connectionProperties must not contain %s; use spec.passwordSecretRef", glueConnPasswordProperty)
	}

	props := make(map[string]string, len(conn.Spec.ConnectionProperties)+1)
	for k, v := range conn.Spec.ConnectionProperties {
		props[k] = v
	}
	if conn.Spec.PasswordSecretRef != nil {
		// The resolved value must never be written to status, conditions,
		// or logs.
		password, err := resolveSecretValue(ctx, r.Client, conn.Namespace, *conn.Spec.PasswordSecretRef)
		if err != nil {
			return nil, err
		}
		props[glueConnPasswordProperty] = password
	}

	in := &gluetypes.ConnectionInput{
		Name:                 aws.String(conn.Spec.Name),
		ConnectionType:       gluetypes.ConnectionType(conn.Spec.ConnectionType),
		ConnectionProperties: props,
	}
	if conn.Spec.Description != "" {
		in.Description = aws.String(conn.Spec.Description)
	}
	if pcr := conn.Spec.PhysicalConnectionRequirements; pcr != nil {
		req := &gluetypes.PhysicalConnectionRequirements{}
		if pcr.SubnetRef != nil {
			ids, err := resolveSubnetIDs(ctx, r.Client, conn.Namespace, []awsv1alpha1.SubnetRef{*pcr.SubnetRef})
			if err != nil {
				return nil, err
			}
			req.SubnetId = aws.String(ids[0])
		}
		if len(pcr.SecurityGroupRefs) > 0 {
			sgIDs, err := resolveSGIDs(ctx, r.Client, conn.Namespace, pcr.SecurityGroupRefs)
			if err != nil {
				return nil, err
			}
			req.SecurityGroupIdList = sgIDs
		}
		if pcr.AvailabilityZone != "" {
			req.AvailabilityZone = aws.String(pcr.AvailabilityZone)
		}
		in.PhysicalConnectionRequirements = req
	}
	return in, nil
}

func (r *GlueConnectionReconciler) reconcileConnection(ctx context.Context, conn *awsv1alpha1.GlueConnection) error {
	// Build the input first so dependency and validation problems surface
	// before any AWS mutation.
	input, err := r.connectionInput(ctx, conn)
	if err != nil {
		return err
	}

	_, err = r.GlueClient.GetConnection(ctx, &awsglue.GetConnectionInput{
		Name:         aws.String(conn.Spec.Name),
		HidePassword: true,
	})
	if gluehelper.IsNotFound(err) {
		if _, err := r.GlueClient.CreateConnection(ctx, &awsglue.CreateConnectionInput{
			ConnectionInput: input,
			Tags:            conn.Spec.Tags,
		}); err != nil {
			return fmt.Errorf("create Glue connection: %w", err)
		}
		// Persist the identifier immediately: the AWS resource now exists.
		conn.Status.ConnectionName = conn.Spec.Name
		if err := persistStatus(ctx, r.Client, conn); err != nil {
			return fmt.Errorf("persist connection name after create: %w", err)
		}
	} else if err != nil {
		return err
	} else if conn.Status.ObservedGeneration != conn.Generation {
		if _, err := r.GlueClient.UpdateConnection(ctx, &awsglue.UpdateConnectionInput{
			Name:            aws.String(conn.Spec.Name),
			ConnectionInput: input,
		}); err != nil {
			return fmt.Errorf("update Glue connection: %w", err)
		}
	}

	conn.Status.ConnectionName = conn.Spec.Name
	conn.Status.ObservedGeneration = conn.Generation
	now := metav1.Now()
	conn.Status.LastSyncTime = &now
	return r.setCondition(ctx, conn, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Glue connection reconciled")
}

func (r *GlueConnectionReconciler) deleteConnection(ctx context.Context, conn *awsv1alpha1.GlueConnection) error {
	name := conn.Status.ConnectionName
	if name == "" {
		name = conn.Spec.Name
	}
	_, err := r.GlueClient.DeleteConnection(ctx, &awsglue.DeleteConnectionInput{
		ConnectionName: aws.String(name),
	})
	if gluehelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *GlueConnectionReconciler) setCondition(ctx context.Context, conn *awsv1alpha1.GlueConnection, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&conn.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: conn.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, conn); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *GlueConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.GlueConnection{}).
		Complete(r)
}
