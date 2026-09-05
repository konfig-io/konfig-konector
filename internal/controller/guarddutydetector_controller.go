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
	awsguardduty "github.com/aws/aws-sdk-go-v2/service/guardduty"
	guarddutytypes "github.com/aws/aws-sdk-go-v2/service/guardduty/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	guarddutyhelper "github.com/konfig-io/konfig-konector/internal/aws/guardduty"
)

// GuardDutyDetectorAWSAPI is the subset of the GuardDuty API used by this controller.
type GuardDutyDetectorAWSAPI interface {
	ListDetectors(ctx context.Context, params *awsguardduty.ListDetectorsInput, optFns ...func(*awsguardduty.Options)) (*awsguardduty.ListDetectorsOutput, error)
	GetDetector(ctx context.Context, params *awsguardduty.GetDetectorInput, optFns ...func(*awsguardduty.Options)) (*awsguardduty.GetDetectorOutput, error)
	CreateDetector(ctx context.Context, params *awsguardduty.CreateDetectorInput, optFns ...func(*awsguardduty.Options)) (*awsguardduty.CreateDetectorOutput, error)
	UpdateDetector(ctx context.Context, params *awsguardduty.UpdateDetectorInput, optFns ...func(*awsguardduty.Options)) (*awsguardduty.UpdateDetectorOutput, error)
	DeleteDetector(ctx context.Context, params *awsguardduty.DeleteDetectorInput, optFns ...func(*awsguardduty.Options)) (*awsguardduty.DeleteDetectorOutput, error)
}

// GuardDutyDetectorReconciler reconciles GuardDutyDetector objects.
type GuardDutyDetectorReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	GuardDutyClient GuardDutyDetectorAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=guarddutydetectors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=guarddutydetectors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=guarddutydetectors/finalizers,verbs=update

func (r *GuardDutyDetectorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	det := &awsv1alpha1.GuardDutyDetector{}
	if err := r.Get(ctx, req.NamespacedName, det); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, det); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !det.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(det, awsv1alpha1.FinalizerName) {
			if shouldAbandon(det) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(det, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, det)
			}
			if err := r.deleteDetector(ctx, det); err != nil {
				logger.Error(err, "failed to delete GuardDuty detector")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(det, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, det)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(det, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(det, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, det); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileDetector(ctx, det); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, det, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *GuardDutyDetectorReconciler) reconcileDetector(ctx context.Context, det *awsv1alpha1.GuardDutyDetector) error {
	detectorID := det.Status.DetectorID
	if detectorID == "" {
		// GuardDuty allows one detector per account/region; adopt it if it exists.
		id, err := r.findDetector(ctx)
		if err != nil {
			return err
		}
		detectorID = id
	}

	enable := det.Spec.Enable == nil || *det.Spec.Enable

	if detectorID == "" {
		createOut, err := r.GuardDutyClient.CreateDetector(ctx, &awsguardduty.CreateDetectorInput{
			Enable:                     aws.Bool(enable),
			FindingPublishingFrequency: guarddutytypes.FindingPublishingFrequency(det.Spec.FindingPublishingFrequency),
			Features:                   guarddutyFeatures(det.Spec.Features),
			Tags:                       det.Spec.Tags,
		})
		if err != nil {
			return fmt.Errorf("create detector: %w", err)
		}
		detectorID = aws.ToString(createOut.DetectorId)
		// Persist the detector ID immediately after create.
		det.Status.DetectorID = detectorID
		if err := persistStatus(ctx, r.Client, det); err != nil {
			return fmt.Errorf("persist detector ID after create: %w", err)
		}
	} else {
		if det.Status.DetectorID == "" {
			det.Status.DetectorID = detectorID
			if err := persistStatus(ctx, r.Client, det); err != nil {
				return fmt.Errorf("persist adopted detector ID: %w", err)
			}
		}
		if det.Status.ObservedGeneration != det.Generation {
			if _, err := r.GuardDutyClient.UpdateDetector(ctx, &awsguardduty.UpdateDetectorInput{
				DetectorId:                 aws.String(detectorID),
				Enable:                     aws.Bool(enable),
				FindingPublishingFrequency: guarddutytypes.FindingPublishingFrequency(det.Spec.FindingPublishingFrequency),
				Features:                   guarddutyFeatures(det.Spec.Features),
			}); err != nil {
				return fmt.Errorf("update detector: %w", err)
			}
		}
	}

	getOut, err := r.GuardDutyClient.GetDetector(ctx, &awsguardduty.GetDetectorInput{
		DetectorId: aws.String(detectorID),
	})
	if err != nil {
		return fmt.Errorf("get detector: %w", err)
	}

	det.Status.DetectorID = detectorID
	det.Status.DetectorStatus = string(getOut.Status)
	det.Status.ObservedGeneration = det.Generation
	now := metav1.Now()
	det.Status.LastSyncTime = &now
	return r.setCondition(ctx, det, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "GuardDuty detector reconciled")
}

// findDetector returns the account's detector ID for this region, or "".
func (r *GuardDutyDetectorReconciler) findDetector(ctx context.Context) (string, error) {
	out, err := r.GuardDutyClient.ListDetectors(ctx, &awsguardduty.ListDetectorsInput{})
	if err != nil {
		return "", fmt.Errorf("list detectors: %w", err)
	}
	if len(out.DetectorIds) > 0 {
		return out.DetectorIds[0], nil
	}
	return "", nil
}

func (r *GuardDutyDetectorReconciler) deleteDetector(ctx context.Context, det *awsv1alpha1.GuardDutyDetector) error {
	detectorID := det.Status.DetectorID
	if detectorID == "" {
		// The region's single detector is a deterministic lookup.
		id, err := r.findDetector(ctx)
		if err != nil {
			return err
		}
		if id == "" {
			return nil
		}
		detectorID = id
	}
	_, err := r.GuardDutyClient.DeleteDetector(ctx, &awsguardduty.DeleteDetectorInput{
		DetectorId: aws.String(detectorID),
	})
	if guarddutyhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *GuardDutyDetectorReconciler) setCondition(ctx context.Context, det *awsv1alpha1.GuardDutyDetector, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&det.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: det.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, det); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func guarddutyFeatures(features []awsv1alpha1.GuardDutyFeature) []guarddutytypes.DetectorFeatureConfiguration {
	if len(features) == 0 {
		return nil
	}
	out := make([]guarddutytypes.DetectorFeatureConfiguration, 0, len(features))
	for _, f := range features {
		out = append(out, guarddutytypes.DetectorFeatureConfiguration{
			Name:   guarddutytypes.DetectorFeature(f.Name),
			Status: guarddutytypes.FeatureStatus(f.Status),
		})
	}
	return out
}

func (r *GuardDutyDetectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.GuardDutyDetector{}).
		Complete(r)
}
