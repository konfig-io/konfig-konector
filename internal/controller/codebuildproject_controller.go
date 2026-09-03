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
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscb "github.com/aws/aws-sdk-go-v2/service/codebuild"
	cbtypes "github.com/aws/aws-sdk-go-v2/service/codebuild/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// CodeBuildProjectReconciler reconciles CodeBuildProject objects.
type CodeBuildProjectReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	CodeBuildClient *awscb.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=codebuildprojects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=codebuildprojects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=codebuildprojects/finalizers,verbs=update

func (r *CodeBuildProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.CodeBuildProject{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
			if shouldAbandon(obj) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, obj)
			}
			if err := r.deleteProject(ctx, obj); err != nil {
				logger.Error(err, "failed to delete CodeBuildProject")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, obj)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(obj, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileProject(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionCB(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *CodeBuildProjectReconciler) reconcileProject(ctx context.Context, obj *awsv1alpha1.CodeBuildProject) error {
	batchOut, err := r.CodeBuildClient.BatchGetProjects(ctx, &awscb.BatchGetProjectsInput{
		Names: []string{obj.Spec.Name},
	})
	if err != nil {
		return fmt.Errorf("batch get codebuild projects: %w", err)
	}

	environment := &cbtypes.ProjectEnvironment{
		Type:           cbtypes.EnvironmentType(obj.Spec.Environment.Type),
		Image:          aws.String(obj.Spec.Environment.Image),
		ComputeType:    cbtypes.ComputeType(obj.Spec.Environment.ComputeType),
		PrivilegedMode: aws.Bool(obj.Spec.Environment.PrivilegedMode),
	}
	source := &cbtypes.ProjectSource{
		Type: cbtypes.SourceType(obj.Spec.Source.Type),
	}
	if obj.Spec.Source.Location != "" {
		source.Location = aws.String(obj.Spec.Source.Location)
	}
	if obj.Spec.Source.Buildspec != "" {
		source.Buildspec = aws.String(obj.Spec.Source.Buildspec)
	}
	artifacts := &cbtypes.ProjectArtifacts{
		Type: cbtypes.ArtifactsType(obj.Spec.Artifacts.Type),
	}
	if obj.Spec.Artifacts.Location != "" {
		artifacts.Location = aws.String(obj.Spec.Artifacts.Location)
	}

	if len(batchOut.Projects) > 0 {
		obj.Status.ProjectARN = aws.ToString(batchOut.Projects[0].Arn)

		if _, err := r.CodeBuildClient.UpdateProject(ctx, &awscb.UpdateProjectInput{
			Name:        aws.String(obj.Spec.Name),
			Description: aws.String(obj.Spec.Description),
			ServiceRole: aws.String(obj.Spec.ServiceRoleARN),
			Source:      source,
			Artifacts:   artifacts,
			Environment: environment,
		}); err != nil {
			return fmt.Errorf("update codebuild project: %w", err)
		}
		obj.Status.ObservedGeneration = obj.Generation
		now := metav1.Now()
		obj.Status.LastSyncTime = &now
		return r.setConditionCB(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "CodeBuildProject reconciled")
	}

	tags := make([]cbtypes.Tag, 0, len(obj.Spec.Tags))
	for k, v := range obj.Spec.Tags {
		k, v := k, v
		tags = append(tags, cbtypes.Tag{Key: &k, Value: &v})
	}

	out, err := r.CodeBuildClient.CreateProject(ctx, &awscb.CreateProjectInput{
		Name:        aws.String(obj.Spec.Name),
		Description: aws.String(obj.Spec.Description),
		ServiceRole: aws.String(obj.Spec.ServiceRoleARN),
		Source:      source,
		Artifacts:   artifacts,
		Environment: environment,
		Tags:        tags,
	})
	if err != nil {
		return fmt.Errorf("create codebuild project: %w", err)
	}

	if out.Project != nil {
		obj.Status.ProjectARN = aws.ToString(out.Project.Arn)
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionCB(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "CodeBuildProject created")
}

func (r *CodeBuildProjectReconciler) deleteProject(ctx context.Context, obj *awsv1alpha1.CodeBuildProject) error {
	_, err := r.CodeBuildClient.DeleteProject(ctx, &awscb.DeleteProjectInput{
		Name: aws.String(obj.Spec.Name),
	})
	if err != nil {
		// CodeBuild DeleteProject returns success even if project doesn't exist
		return nil
	}
	return nil
}

func (r *CodeBuildProjectReconciler) setConditionCB(ctx context.Context, obj *awsv1alpha1.CodeBuildProject, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: obj.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *CodeBuildProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CodeBuildProject{}).
		Complete(r)
}
