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
	awss3control "github.com/aws/aws-sdk-go-v2/service/s3control"
	s3controltypes "github.com/aws/aws-sdk-go-v2/service/s3control/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	s3controlhelper "github.com/konfig-io/konfig-konector/internal/aws/s3control"
)

// S3AccessPointAWSAPI is the subset of the S3 Control API used by this controller.
type S3AccessPointAWSAPI interface {
	GetAccessPoint(ctx context.Context, params *awss3control.GetAccessPointInput, optFns ...func(*awss3control.Options)) (*awss3control.GetAccessPointOutput, error)
	CreateAccessPoint(ctx context.Context, params *awss3control.CreateAccessPointInput, optFns ...func(*awss3control.Options)) (*awss3control.CreateAccessPointOutput, error)
	DeleteAccessPoint(ctx context.Context, params *awss3control.DeleteAccessPointInput, optFns ...func(*awss3control.Options)) (*awss3control.DeleteAccessPointOutput, error)
}

// S3AccessPointReconciler reconciles S3AccessPoint objects.
type S3AccessPointReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	S3ControlClient S3AccessPointAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3accesspoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3accesspoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3accesspoints/finalizers,verbs=update

func (r *S3AccessPointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ap := &awsv1alpha1.S3AccessPoint{}
	if err := r.Get(ctx, req.NamespacedName, ap); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, ap); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !ap.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ap, awsv1alpha1.FinalizerName) {
			if shouldAbandon(ap) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(ap, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, ap)
			}
			if err := r.deleteAccessPoint(ctx, ap); err != nil {
				logger.Error(err, "failed to delete access point")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(ap, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, ap)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(ap, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(ap, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, ap); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileAccessPoint(ctx, ap); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, ap, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

// resolveBucketName resolves the S3BucketRef to a bucket name.
func (r *S3AccessPointReconciler) resolveBucketName(ctx context.Context, namespace string, ref awsv1alpha1.S3BucketRef) (string, error) {
	if ref.BucketName != "" {
		return ref.BucketName, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("bucketRef must specify either name or bucketName")
	}
	bucket := &awsv1alpha1.S3Bucket{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, bucket); err != nil {
		return "", err
	}
	if bucket.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("S3Bucket %s/%s has no ARN yet", namespace, ref.Name)}
	}
	return bucket.Spec.BucketName, nil
}

// resolveVPCID resolves the VPCResourceRef to a VPC ID.
func (r *S3AccessPointReconciler) resolveVPCID(ctx context.Context, namespace string, ref awsv1alpha1.VPCResourceRef) (string, error) {
	if ref.ID != "" {
		return ref.ID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("vpcRef must specify either name or id")
	}
	vpc := &awsv1alpha1.VPC{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, vpc); err != nil {
		return "", err
	}
	if vpc.Status.VPCID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("VPC %s/%s has no ID yet", namespace, ref.Name)}
	}
	return vpc.Status.VPCID, nil
}

func (r *S3AccessPointReconciler) reconcileAccessPoint(ctx context.Context, ap *awsv1alpha1.S3AccessPoint) error {
	bucketName, err := r.resolveBucketName(ctx, ap.Namespace, ap.Spec.BucketRef)
	if err != nil {
		return err
	}

	getOut, err := r.S3ControlClient.GetAccessPoint(ctx, &awss3control.GetAccessPointInput{
		AccountId: aws.String(ap.Spec.AccountID),
		Name:      aws.String(ap.Spec.Name),
	})
	if s3controlhelper.IsNotFound(err) {
		input := &awss3control.CreateAccessPointInput{
			AccountId: aws.String(ap.Spec.AccountID),
			Name:      aws.String(ap.Spec.Name),
			Bucket:    aws.String(bucketName),
		}
		if ap.Spec.VPCConfiguration != nil {
			vpcID, err := r.resolveVPCID(ctx, ap.Namespace, ap.Spec.VPCConfiguration.VPCRef)
			if err != nil {
				return err
			}
			input.VpcConfiguration = &s3controltypes.VpcConfiguration{VpcId: aws.String(vpcID)}
		}
		if pab := ap.Spec.PublicAccessBlock; pab != nil {
			input.PublicAccessBlockConfiguration = &s3controltypes.PublicAccessBlockConfiguration{
				BlockPublicAcls:       aws.Bool(pab.BlockPublicAcls),
				BlockPublicPolicy:     aws.Bool(pab.BlockPublicPolicy),
				IgnorePublicAcls:      aws.Bool(pab.IgnorePublicAcls),
				RestrictPublicBuckets: aws.Bool(pab.RestrictPublicBuckets),
			}
		}
		createOut, err := r.S3ControlClient.CreateAccessPoint(ctx, input)
		if err != nil {
			return fmt.Errorf("create access point: %w", err)
		}
		ap.Status.ARN = aws.ToString(createOut.AccessPointArn)
		ap.Status.Alias = aws.ToString(createOut.Alias)
		if err := persistStatus(ctx, r.Client, ap); err != nil {
			return fmt.Errorf("persist access point ARN after create: %w", err)
		}
	} else if err != nil {
		return err
	} else {
		ap.Status.ARN = aws.ToString(getOut.AccessPointArn)
		ap.Status.Alias = aws.ToString(getOut.Alias)
		// Access point configuration (bucket, VPC config) is immutable in AWS;
		// public access block changes require recreation too, so there is no
		// update path.
	}

	ap.Status.ObservedGeneration = ap.Generation
	now := metav1.Now()
	ap.Status.LastSyncTime = &now
	return r.setCondition(ctx, ap, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "access point reconciled")
}

func (r *S3AccessPointReconciler) deleteAccessPoint(ctx context.Context, ap *awsv1alpha1.S3AccessPoint) error {
	// The access point is identified by account ID + name, both in spec.
	_, err := r.S3ControlClient.DeleteAccessPoint(ctx, &awss3control.DeleteAccessPointInput{
		AccountId: aws.String(ap.Spec.AccountID),
		Name:      aws.String(ap.Spec.Name),
	})
	if s3controlhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *S3AccessPointReconciler) setCondition(ctx context.Context, ap *awsv1alpha1.S3AccessPoint, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&ap.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: ap.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, ap); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *S3AccessPointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.S3AccessPoint{}).
		Complete(r)
}
