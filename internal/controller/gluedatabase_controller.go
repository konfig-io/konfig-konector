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

// GlueDatabaseAWSAPI is the subset of the Glue SDK client used by this
// controller. *awsglue.Client satisfies it.
type GlueDatabaseAWSAPI interface {
	GetDatabase(ctx context.Context, params *awsglue.GetDatabaseInput, optFns ...func(*awsglue.Options)) (*awsglue.GetDatabaseOutput, error)
	CreateDatabase(ctx context.Context, params *awsglue.CreateDatabaseInput, optFns ...func(*awsglue.Options)) (*awsglue.CreateDatabaseOutput, error)
	UpdateDatabase(ctx context.Context, params *awsglue.UpdateDatabaseInput, optFns ...func(*awsglue.Options)) (*awsglue.UpdateDatabaseOutput, error)
	DeleteDatabase(ctx context.Context, params *awsglue.DeleteDatabaseInput, optFns ...func(*awsglue.Options)) (*awsglue.DeleteDatabaseOutput, error)
}

// GlueDatabaseReconciler reconciles GlueDatabase objects.
type GlueDatabaseReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	GlueClient GlueDatabaseAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=gluedatabases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=gluedatabases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=gluedatabases/finalizers,verbs=update

func (r *GlueDatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	db := &awsv1alpha1.GlueDatabase{}
	if err := r.Get(ctx, req.NamespacedName, db); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, db); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !db.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(db, awsv1alpha1.FinalizerName) {
			if shouldAbandon(db) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(db, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, db)
			}
			if err := r.deleteDatabase(ctx, db); err != nil {
				logger.Error(err, "failed to delete Glue database")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(db, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, db)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(db, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(db, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, db); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileDatabase(ctx, db); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, db, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *GlueDatabaseReconciler) databaseInput(db *awsv1alpha1.GlueDatabase) *gluetypes.DatabaseInput {
	in := &gluetypes.DatabaseInput{
		Name: aws.String(db.Spec.Name),
	}
	if db.Spec.Description != "" {
		in.Description = aws.String(db.Spec.Description)
	}
	if db.Spec.LocationURI != "" {
		in.LocationUri = aws.String(db.Spec.LocationURI)
	}
	if len(db.Spec.Parameters) > 0 {
		in.Parameters = db.Spec.Parameters
	}
	return in
}

func (r *GlueDatabaseReconciler) reconcileDatabase(ctx context.Context, db *awsv1alpha1.GlueDatabase) error {
	_, err := r.GlueClient.GetDatabase(ctx, &awsglue.GetDatabaseInput{
		Name: aws.String(db.Spec.Name),
	})
	if gluehelper.IsNotFound(err) {
		if _, err := r.GlueClient.CreateDatabase(ctx, &awsglue.CreateDatabaseInput{
			DatabaseInput: r.databaseInput(db),
			Tags:          db.Spec.Tags,
		}); err != nil {
			return fmt.Errorf("create Glue database: %w", err)
		}
		// Persist the identifier immediately: the AWS resource now exists.
		db.Status.DatabaseName = db.Spec.Name
		if err := persistStatus(ctx, r.Client, db); err != nil {
			return fmt.Errorf("persist database name after create: %w", err)
		}
	} else if err != nil {
		return err
	} else if db.Status.ObservedGeneration != db.Generation {
		if _, err := r.GlueClient.UpdateDatabase(ctx, &awsglue.UpdateDatabaseInput{
			Name:          aws.String(db.Spec.Name),
			DatabaseInput: r.databaseInput(db),
		}); err != nil {
			return fmt.Errorf("update Glue database: %w", err)
		}
	}

	db.Status.DatabaseName = db.Spec.Name
	db.Status.ObservedGeneration = db.Generation
	now := metav1.Now()
	db.Status.LastSyncTime = &now
	return r.setCondition(ctx, db, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Glue database reconciled")
}

func (r *GlueDatabaseReconciler) deleteDatabase(ctx context.Context, db *awsv1alpha1.GlueDatabase) error {
	name := db.Status.DatabaseName
	if name == "" {
		// The database name is deterministically derivable from the spec.
		name = db.Spec.Name
	}
	_, err := r.GlueClient.DeleteDatabase(ctx, &awsglue.DeleteDatabaseInput{
		Name: aws.String(name),
	})
	if gluehelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *GlueDatabaseReconciler) setCondition(ctx context.Context, db *awsv1alpha1.GlueDatabase, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&db.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: db.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, db); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *GlueDatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.GlueDatabase{}).
		Complete(r)
}
