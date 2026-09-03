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
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	gluehelper "github.com/konfig-io/konfig-konector/internal/aws/glue"
)

// GlueCrawlerAWSAPI is the subset of the Glue SDK client used by this
// controller. *awsglue.Client satisfies it.
type GlueCrawlerAWSAPI interface {
	GetCrawler(ctx context.Context, params *awsglue.GetCrawlerInput, optFns ...func(*awsglue.Options)) (*awsglue.GetCrawlerOutput, error)
	CreateCrawler(ctx context.Context, params *awsglue.CreateCrawlerInput, optFns ...func(*awsglue.Options)) (*awsglue.CreateCrawlerOutput, error)
	UpdateCrawler(ctx context.Context, params *awsglue.UpdateCrawlerInput, optFns ...func(*awsglue.Options)) (*awsglue.UpdateCrawlerOutput, error)
	DeleteCrawler(ctx context.Context, params *awsglue.DeleteCrawlerInput, optFns ...func(*awsglue.Options)) (*awsglue.DeleteCrawlerOutput, error)
}

// GlueCrawlerReconciler reconciles GlueCrawler objects.
type GlueCrawlerReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	GlueClient GlueCrawlerAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=gluecrawlers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=gluecrawlers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=gluecrawlers/finalizers,verbs=update

func (r *GlueCrawlerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	cr := &awsv1alpha1.GlueCrawler{}
	if err := r.Get(ctx, req.NamespacedName, cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !cr.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(cr, awsv1alpha1.FinalizerName) {
			if shouldAbandon(cr) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(cr, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, cr)
			}
			if err := r.deleteCrawler(ctx, cr); err != nil {
				logger.Error(err, "failed to delete Glue crawler")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(cr, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, cr)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(cr, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(cr, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileCrawler(ctx, cr); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, cr, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

// resolveDatabaseName resolves the crawler's target database from a direct
// name or a GlueDatabase CR reference.
func (r *GlueCrawlerReconciler) resolveDatabaseName(ctx context.Context, cr *awsv1alpha1.GlueCrawler) (string, error) {
	if cr.Spec.DatabaseName != "" {
		return cr.Spec.DatabaseName, nil
	}
	if cr.Spec.DatabaseRef == "" {
		return "", nil
	}
	dbCR := &awsv1alpha1.GlueDatabase{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: cr.Spec.DatabaseRef, Namespace: cr.Namespace}, dbCR); err != nil {
		return "", err
	}
	if dbCR.Status.DatabaseName == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("GlueDatabase %s/%s not synced yet", cr.Namespace, cr.Spec.DatabaseRef)}
	}
	return dbCR.Status.DatabaseName, nil
}

func crawlerTargets(spec *awsv1alpha1.GlueCrawlerTargets) *gluetypes.CrawlerTargets {
	targets := &gluetypes.CrawlerTargets{}
	for _, t := range spec.S3Targets {
		targets.S3Targets = append(targets.S3Targets, gluetypes.S3Target{
			Path:       aws.String(t.Path),
			Exclusions: t.Exclusions,
		})
	}
	for _, t := range spec.JDBCTargets {
		jt := gluetypes.JdbcTarget{
			ConnectionName: aws.String(t.ConnectionName),
			Exclusions:     t.Exclusions,
		}
		if t.Path != "" {
			jt.Path = aws.String(t.Path)
		}
		targets.JdbcTargets = append(targets.JdbcTargets, jt)
	}
	return targets
}

func crawlerSchemaChangePolicy(spec *awsv1alpha1.GlueSchemaChangePolicy) *gluetypes.SchemaChangePolicy {
	if spec == nil {
		return nil
	}
	return &gluetypes.SchemaChangePolicy{
		UpdateBehavior: gluetypes.UpdateBehavior(spec.UpdateBehavior),
		DeleteBehavior: gluetypes.DeleteBehavior(spec.DeleteBehavior),
	}
}

func (r *GlueCrawlerReconciler) reconcileCrawler(ctx context.Context, cr *awsv1alpha1.GlueCrawler) error {
	// Resolve dependencies up front so a missing role/database is reported
	// before any AWS mutation.
	roleARN, err := resolveIAMRoleARN(ctx, r.Client, cr.Namespace, cr.Spec.RoleRef)
	if err != nil {
		return err
	}
	dbName, err := r.resolveDatabaseName(ctx, cr)
	if err != nil {
		return err
	}

	_, err = r.GlueClient.GetCrawler(ctx, &awsglue.GetCrawlerInput{
		Name: aws.String(cr.Spec.Name),
	})
	if gluehelper.IsNotFound(err) {
		input := &awsglue.CreateCrawlerInput{
			Name:               aws.String(cr.Spec.Name),
			Role:               aws.String(roleARN),
			Targets:            crawlerTargets(&cr.Spec.Targets),
			SchemaChangePolicy: crawlerSchemaChangePolicy(cr.Spec.SchemaChangePolicy),
			Tags:               cr.Spec.Tags,
		}
		if dbName != "" {
			input.DatabaseName = aws.String(dbName)
		}
		if cr.Spec.Schedule != "" {
			input.Schedule = aws.String(cr.Spec.Schedule)
		}
		if cr.Spec.TablePrefix != "" {
			input.TablePrefix = aws.String(cr.Spec.TablePrefix)
		}
		if cr.Spec.Configuration != "" {
			input.Configuration = aws.String(cr.Spec.Configuration)
		}
		if cr.Spec.Description != "" {
			input.Description = aws.String(cr.Spec.Description)
		}
		if _, err := r.GlueClient.CreateCrawler(ctx, input); err != nil {
			return fmt.Errorf("create Glue crawler: %w", err)
		}
		// Persist the identifier immediately: the AWS resource now exists.
		cr.Status.CrawlerName = cr.Spec.Name
		if err := persistStatus(ctx, r.Client, cr); err != nil {
			return fmt.Errorf("persist crawler name after create: %w", err)
		}
	} else if err != nil {
		return err
	} else if cr.Status.ObservedGeneration != cr.Generation {
		input := &awsglue.UpdateCrawlerInput{
			Name:               aws.String(cr.Spec.Name),
			Role:               aws.String(roleARN),
			Targets:            crawlerTargets(&cr.Spec.Targets),
			SchemaChangePolicy: crawlerSchemaChangePolicy(cr.Spec.SchemaChangePolicy),
		}
		if dbName != "" {
			input.DatabaseName = aws.String(dbName)
		}
		if cr.Spec.Schedule != "" {
			input.Schedule = aws.String(cr.Spec.Schedule)
		}
		if cr.Spec.TablePrefix != "" {
			input.TablePrefix = aws.String(cr.Spec.TablePrefix)
		}
		if cr.Spec.Configuration != "" {
			input.Configuration = aws.String(cr.Spec.Configuration)
		}
		if cr.Spec.Description != "" {
			input.Description = aws.String(cr.Spec.Description)
		}
		if _, err := r.GlueClient.UpdateCrawler(ctx, input); err != nil {
			return fmt.Errorf("update Glue crawler: %w", err)
		}
	}

	cr.Status.CrawlerName = cr.Spec.Name
	cr.Status.ObservedGeneration = cr.Generation
	now := metav1.Now()
	cr.Status.LastSyncTime = &now
	return r.setCondition(ctx, cr, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Glue crawler reconciled")
}

func (r *GlueCrawlerReconciler) deleteCrawler(ctx context.Context, cr *awsv1alpha1.GlueCrawler) error {
	name := cr.Status.CrawlerName
	if name == "" {
		name = cr.Spec.Name
	}
	_, err := r.GlueClient.DeleteCrawler(ctx, &awsglue.DeleteCrawlerInput{
		Name: aws.String(name),
	})
	if gluehelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *GlueCrawlerReconciler) setCondition(ctx context.Context, cr *awsv1alpha1.GlueCrawler, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: cr.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, cr); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *GlueCrawlerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.GlueCrawler{}).
		Complete(r)
}
