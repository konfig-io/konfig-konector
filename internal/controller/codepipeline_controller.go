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
	awscp "github.com/aws/aws-sdk-go-v2/service/codepipeline"
	cptypes "github.com/aws/aws-sdk-go-v2/service/codepipeline/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	cphelper "github.com/konfig-io/konfig-konector/internal/aws/codepipeline"
)

// CodePipelineReconciler reconciles CodePipeline objects.
type CodePipelineReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	CodePipelineClient *awscp.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=codepipelines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=codepipelines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=codepipelines/finalizers,verbs=update

func (r *CodePipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.CodePipeline{}
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
			if err := r.deletePipeline(ctx, obj); err != nil {
				logger.Error(err, "failed to delete CodePipeline")
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

	if err := r.reconcilePipeline(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionCP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *CodePipelineReconciler) reconcilePipeline(ctx context.Context, obj *awsv1alpha1.CodePipeline) error {
	declaration := r.buildPipelineDeclaration(obj)

	getOut, err := r.CodePipelineClient.GetPipeline(ctx, &awscp.GetPipelineInput{
		Name: aws.String(obj.Spec.PipelineName),
	})
	if err != nil && !cphelper.IsNotFound(err) {
		return fmt.Errorf("get codepipeline: %w", err)
	}

	if err == nil && getOut.Pipeline != nil {
		obj.Status.PipelineARN = aws.ToString(getOut.Metadata.PipelineArn)

		if _, err := r.CodePipelineClient.UpdatePipeline(ctx, &awscp.UpdatePipelineInput{
			Pipeline: declaration,
		}); err != nil {
			return fmt.Errorf("update codepipeline: %w", err)
		}
		obj.Status.ObservedGeneration = obj.Generation
		now := metav1.Now()
		obj.Status.LastSyncTime = &now
		return r.setConditionCP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "CodePipeline reconciled")
	}

	tags := make([]cptypes.Tag, 0, len(obj.Spec.Tags))
	for k, v := range obj.Spec.Tags {
		k, v := k, v
		tags = append(tags, cptypes.Tag{Key: &k, Value: &v})
	}

	out, err := r.CodePipelineClient.CreatePipeline(ctx, &awscp.CreatePipelineInput{
		Pipeline: declaration,
		Tags:     tags,
	})
	if err != nil {
		return fmt.Errorf("create codepipeline: %w", err)
	}

	if out.Pipeline != nil {
		obj.Status.PipelineARN = aws.ToString(out.Pipeline.Name)
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionCP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "CodePipeline created")
}

func (r *CodePipelineReconciler) buildPipelineDeclaration(obj *awsv1alpha1.CodePipeline) *cptypes.PipelineDeclaration {
	stages := make([]cptypes.StageDeclaration, 0, len(obj.Spec.Stages))
	for _, s := range obj.Spec.Stages {
		actions := make([]cptypes.ActionDeclaration, 0, len(s.Actions))
		for _, a := range s.Actions {
			act := cptypes.ActionDeclaration{
				Name: aws.String(a.Name),
				ActionTypeId: &cptypes.ActionTypeId{
					Category: cptypes.ActionCategory(a.ActionTypeID.Category),
					Owner:    cptypes.ActionOwner(a.ActionTypeID.Owner),
					Provider: aws.String(a.ActionTypeID.Provider),
					Version:  aws.String(a.ActionTypeID.Version),
				},
			}
			if len(a.Configuration) > 0 {
				act.Configuration = a.Configuration
			}
			for _, in := range a.InputArtifacts {
				in := in
				act.InputArtifacts = append(act.InputArtifacts, cptypes.InputArtifact{Name: &in})
			}
			for _, out := range a.OutputArtifacts {
				out := out
				act.OutputArtifacts = append(act.OutputArtifacts, cptypes.OutputArtifact{Name: &out})
			}
			if a.RunOrder != nil {
				act.RunOrder = a.RunOrder
			}
			actions = append(actions, act)
		}
		stages = append(stages, cptypes.StageDeclaration{
			Name:    aws.String(s.Name),
			Actions: actions,
		})
	}

	return &cptypes.PipelineDeclaration{
		Name:    aws.String(obj.Spec.PipelineName),
		RoleArn: aws.String(obj.Spec.RoleARN),
		ArtifactStore: &cptypes.ArtifactStore{
			Type:     cptypes.ArtifactStoreType(obj.Spec.ArtifactStore.Type),
			Location: aws.String(obj.Spec.ArtifactStore.Location),
		},
		Stages: stages,
	}
}

func (r *CodePipelineReconciler) deletePipeline(ctx context.Context, obj *awsv1alpha1.CodePipeline) error {
	_, err := r.CodePipelineClient.DeletePipeline(ctx, &awscp.DeletePipelineInput{
		Name: aws.String(obj.Spec.PipelineName),
	})
	if cphelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *CodePipelineReconciler) setConditionCP(ctx context.Context, obj *awsv1alpha1.CodePipeline, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *CodePipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CodePipeline{}).
		Complete(r)
}
