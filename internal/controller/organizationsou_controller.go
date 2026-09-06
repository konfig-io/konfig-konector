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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsorgs "github.com/aws/aws-sdk-go-v2/service/organizations"
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	orghelper "github.com/konfig-io/konfig-konector/internal/aws/organizations"
)

// requeueOrgPolling is the family-level polling interval for asynchronous
// organization-tier operations (account creation, control enablement).
var requeueOrgPolling = ctrl.Result{RequeueAfter: 30 * time.Second}

// orgTags converts a CR tag map to Organizations SDK tags.
func orgTags(tags map[string]string) []orgtypes.Tag {
	if len(tags) == 0 {
		return nil
	}
	out := make([]orgtypes.Tag, 0, len(tags))
	for k, v := range tags {
		out = append(out, orgtypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	return out
}

// OrganizationsOUAWSAPI is the subset of the Organizations API used by this controller.
type OrganizationsOUAWSAPI interface {
	CreateOrganizationalUnit(ctx context.Context, params *awsorgs.CreateOrganizationalUnitInput, optFns ...func(*awsorgs.Options)) (*awsorgs.CreateOrganizationalUnitOutput, error)
	UpdateOrganizationalUnit(ctx context.Context, params *awsorgs.UpdateOrganizationalUnitInput, optFns ...func(*awsorgs.Options)) (*awsorgs.UpdateOrganizationalUnitOutput, error)
	DeleteOrganizationalUnit(ctx context.Context, params *awsorgs.DeleteOrganizationalUnitInput, optFns ...func(*awsorgs.Options)) (*awsorgs.DeleteOrganizationalUnitOutput, error)
	DescribeOrganizationalUnit(ctx context.Context, params *awsorgs.DescribeOrganizationalUnitInput, optFns ...func(*awsorgs.Options)) (*awsorgs.DescribeOrganizationalUnitOutput, error)
	TagResource(ctx context.Context, params *awsorgs.TagResourceInput, optFns ...func(*awsorgs.Options)) (*awsorgs.TagResourceOutput, error)
}

// OrganizationsOUReconciler reconciles OrganizationsOU objects.
type OrganizationsOUReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	OrganizationsClient OrganizationsOUAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=organizationsous,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=organizationsous/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=organizationsous/finalizers,verbs=update

func (r *OrganizationsOUReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ou := &awsv1alpha1.OrganizationsOU{}
	if err := r.Get(ctx, req.NamespacedName, ou); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, ou); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !ou.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ou, awsv1alpha1.FinalizerName) {
			if shouldAbandon(ou) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(ou, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, ou)
			}
			if err := r.deleteOU(ctx, ou); err != nil {
				logger.Error(err, "failed to delete organizational unit")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(ou, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, ou)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(ou, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(ou, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, ou); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileOU(ctx, ou); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, ou, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *OrganizationsOUReconciler) resolveParent(ctx context.Context, ou *awsv1alpha1.OrganizationsOU) (string, error) {
	if ou.Spec.ParentID != "" {
		return ou.Spec.ParentID, nil
	}
	if ou.Spec.ParentRef == nil || ou.Spec.ParentRef.Name == "" {
		return "", fmt.Errorf("either parentId or parentRef must be set")
	}
	ns := ou.Spec.ParentRef.Namespace
	if ns == "" {
		ns = ou.Namespace
	}
	parent := &awsv1alpha1.OrganizationsOU{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ou.Spec.ParentRef.Name, Namespace: ns}, parent); err != nil {
		return "", err
	}
	if parent.Status.OUID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("OrganizationsOU %s/%s has no OU ID yet", ns, ou.Spec.ParentRef.Name)}
	}
	return parent.Status.OUID, nil
}

func (r *OrganizationsOUReconciler) reconcileOU(ctx context.Context, ou *awsv1alpha1.OrganizationsOU) error {
	exists := false
	if ou.Status.OUID != "" {
		_, err := r.OrganizationsClient.DescribeOrganizationalUnit(ctx, &awsorgs.DescribeOrganizationalUnitInput{
			OrganizationalUnitId: aws.String(ou.Status.OUID),
		})
		if err == nil {
			exists = true
		} else if !orghelper.IsNotFound(err) {
			return fmt.Errorf("describe organizational unit: %w", err)
		}
	}

	if !exists {
		parentID, err := r.resolveParent(ctx, ou)
		if err != nil {
			return err
		}
		out, err := r.OrganizationsClient.CreateOrganizationalUnit(ctx, &awsorgs.CreateOrganizationalUnitInput{
			Name:     aws.String(ou.Spec.Name),
			ParentId: aws.String(parentID),
			Tags:     orgTags(ou.Spec.Tags),
		})
		if err != nil {
			return fmt.Errorf("create organizational unit: %w", err)
		}
		// Persist the OU ID immediately: the AWS resource now exists, and
		// losing the identifier would orphan it on delete.
		ou.Status.OUID = aws.ToString(out.OrganizationalUnit.Id)
		ou.Status.ARN = aws.ToString(out.OrganizationalUnit.Arn)
		if err := persistStatus(ctx, r.Client, ou); err != nil {
			return fmt.Errorf("persist OU ID after create: %w", err)
		}
	} else if ou.Status.ObservedGeneration != ou.Generation {
		if _, err := r.OrganizationsClient.UpdateOrganizationalUnit(ctx, &awsorgs.UpdateOrganizationalUnitInput{
			OrganizationalUnitId: aws.String(ou.Status.OUID),
			Name:                 aws.String(ou.Spec.Name),
		}); err != nil {
			return fmt.Errorf("update organizational unit: %w", err)
		}
		if len(ou.Spec.Tags) > 0 {
			if _, err := r.OrganizationsClient.TagResource(ctx, &awsorgs.TagResourceInput{
				ResourceId: aws.String(ou.Status.OUID),
				Tags:       orgTags(ou.Spec.Tags),
			}); err != nil {
				return fmt.Errorf("tag organizational unit: %w", err)
			}
		}
	}

	ou.Status.ObservedGeneration = ou.Generation
	now := metav1.Now()
	ou.Status.LastSyncTime = &now
	return r.setCondition(ctx, ou, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "organizational unit reconciled")
}

func (r *OrganizationsOUReconciler) deleteOU(ctx context.Context, ou *awsv1alpha1.OrganizationsOU) error {
	if ou.Status.OUID == "" {
		// Never created (or the identifier was lost before persisting). OU IDs
		// cannot be derived unambiguously from the spec, so do not guess.
		return nil
	}
	_, err := r.OrganizationsClient.DeleteOrganizationalUnit(ctx, &awsorgs.DeleteOrganizationalUnitInput{
		OrganizationalUnitId: aws.String(ou.Status.OUID),
	})
	if orghelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *OrganizationsOUReconciler) setCondition(ctx context.Context, ou *awsv1alpha1.OrganizationsOU, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&ou.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: ou.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, ou); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *OrganizationsOUReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.OrganizationsOU{}).
		Complete(r)
}
