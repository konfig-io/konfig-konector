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
	awsssm "github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ssmhelper "github.com/konfig-io/konfig-konector/internal/aws/ssm"
)

// SSMAssociationAWSAPI is the subset of the SSM API used by this controller.
type SSMAssociationAWSAPI interface {
	CreateAssociation(ctx context.Context, params *awsssm.CreateAssociationInput, optFns ...func(*awsssm.Options)) (*awsssm.CreateAssociationOutput, error)
	UpdateAssociation(ctx context.Context, params *awsssm.UpdateAssociationInput, optFns ...func(*awsssm.Options)) (*awsssm.UpdateAssociationOutput, error)
	DeleteAssociation(ctx context.Context, params *awsssm.DeleteAssociationInput, optFns ...func(*awsssm.Options)) (*awsssm.DeleteAssociationOutput, error)
}

// SSMAssociationReconciler reconciles SSMAssociation objects.
type SSMAssociationReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	SSMClient SSMAssociationAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=ssmassociations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ssmassociations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ssmassociations/finalizers,verbs=update

func (r *SSMAssociationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	assoc := &awsv1alpha1.SSMAssociation{}
	if err := r.Get(ctx, req.NamespacedName, assoc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, assoc); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !assoc.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(assoc, awsv1alpha1.FinalizerName) {
			if shouldAbandon(assoc) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(assoc, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, assoc)
			}
			if err := r.deleteAssociation(ctx, assoc); err != nil {
				logger.Error(err, "failed to delete association")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(assoc, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, assoc)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(assoc, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(assoc, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, assoc); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileAssociation(ctx, assoc); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, assoc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

// resolveDocumentName resolves the SSM document name from the direct name or
// the documentRef.
func (r *SSMAssociationReconciler) resolveDocumentName(ctx context.Context, assoc *awsv1alpha1.SSMAssociation) (string, error) {
	if assoc.Spec.Name != "" {
		return assoc.Spec.Name, nil
	}
	ref := assoc.Spec.DocumentRef
	if ref == nil {
		return "", fmt.Errorf("either name or documentRef must be set")
	}
	if ref.DocumentName != "" {
		return ref.DocumentName, nil
	}
	doc := &awsv1alpha1.SSMDocument{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: assoc.Namespace}, doc); err != nil {
		return "", err
	}
	if doc.Status.DocumentVersion == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("SSMDocument %s/%s is not yet created", assoc.Namespace, ref.Name)}
	}
	return doc.Spec.Name, nil
}

func buildSSMTargets(targets []awsv1alpha1.SSMAssociationTarget) []ssmtypes.Target {
	var out []ssmtypes.Target
	for _, t := range targets {
		out = append(out, ssmtypes.Target{
			Key:    aws.String(t.Key),
			Values: t.Values,
		})
	}
	return out
}

func (r *SSMAssociationReconciler) reconcileAssociation(ctx context.Context, assoc *awsv1alpha1.SSMAssociation) error {
	docName, err := r.resolveDocumentName(ctx, assoc)
	if err != nil {
		return err
	}

	if assoc.Status.AssociationID == "" {
		input := &awsssm.CreateAssociationInput{
			Name:       aws.String(docName),
			Targets:    buildSSMTargets(assoc.Spec.Targets),
			Parameters: assoc.Spec.Parameters,
		}
		if assoc.Spec.AssociationName != "" {
			input.AssociationName = aws.String(assoc.Spec.AssociationName)
		}
		if assoc.Spec.ScheduleExpression != "" {
			input.ScheduleExpression = aws.String(assoc.Spec.ScheduleExpression)
		}
		out, err := r.SSMClient.CreateAssociation(ctx, input)
		if err != nil {
			return fmt.Errorf("create association: %w", err)
		}
		if out.AssociationDescription != nil {
			assoc.Status.AssociationID = aws.ToString(out.AssociationDescription.AssociationId)
		}
		if err := persistStatus(ctx, r.Client, assoc); err != nil {
			return fmt.Errorf("persist association ID after create: %w", err)
		}
	} else if assoc.Status.ObservedGeneration != assoc.Generation {
		input := &awsssm.UpdateAssociationInput{
			AssociationId: aws.String(assoc.Status.AssociationID),
			Name:          aws.String(docName),
			Targets:       buildSSMTargets(assoc.Spec.Targets),
			Parameters:    assoc.Spec.Parameters,
		}
		if assoc.Spec.AssociationName != "" {
			input.AssociationName = aws.String(assoc.Spec.AssociationName)
		}
		if assoc.Spec.ScheduleExpression != "" {
			input.ScheduleExpression = aws.String(assoc.Spec.ScheduleExpression)
		}
		if _, err := r.SSMClient.UpdateAssociation(ctx, input); err != nil {
			return fmt.Errorf("update association: %w", err)
		}
	}

	assoc.Status.ObservedGeneration = assoc.Generation
	now := metav1.Now()
	assoc.Status.LastSyncTime = &now
	return r.setCondition(ctx, assoc, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "association reconciled")
}

func (r *SSMAssociationReconciler) deleteAssociation(ctx context.Context, assoc *awsv1alpha1.SSMAssociation) error {
	if assoc.Status.AssociationID == "" {
		// Association IDs are AWS-generated; without one there is no
		// unambiguous lookup (the same document may have many associations).
		return nil
	}
	_, err := r.SSMClient.DeleteAssociation(ctx, &awsssm.DeleteAssociationInput{
		AssociationId: aws.String(assoc.Status.AssociationID),
	})
	if ssmhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SSMAssociationReconciler) setCondition(ctx context.Context, assoc *awsv1alpha1.SSMAssociation, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&assoc.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: assoc.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, assoc); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *SSMAssociationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.SSMAssociation{}).
		Complete(r)
}
