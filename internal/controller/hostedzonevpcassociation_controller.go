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
	awsroute53 "github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/smithy-go"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	route53helper "github.com/konfig-io/konfig-konector/internal/aws/route53"
)

// HostedZoneVPCAssociationAWSAPI is the subset of the Route53 API used by this controller.
type HostedZoneVPCAssociationAWSAPI interface {
	GetHostedZone(ctx context.Context, params *awsroute53.GetHostedZoneInput, optFns ...func(*awsroute53.Options)) (*awsroute53.GetHostedZoneOutput, error)
	CreateVPCAssociationAuthorization(ctx context.Context, params *awsroute53.CreateVPCAssociationAuthorizationInput, optFns ...func(*awsroute53.Options)) (*awsroute53.CreateVPCAssociationAuthorizationOutput, error)
	DeleteVPCAssociationAuthorization(ctx context.Context, params *awsroute53.DeleteVPCAssociationAuthorizationInput, optFns ...func(*awsroute53.Options)) (*awsroute53.DeleteVPCAssociationAuthorizationOutput, error)
	AssociateVPCWithHostedZone(ctx context.Context, params *awsroute53.AssociateVPCWithHostedZoneInput, optFns ...func(*awsroute53.Options)) (*awsroute53.AssociateVPCWithHostedZoneOutput, error)
	DisassociateVPCFromHostedZone(ctx context.Context, params *awsroute53.DisassociateVPCFromHostedZoneInput, optFns ...func(*awsroute53.Options)) (*awsroute53.DisassociateVPCFromHostedZoneOutput, error)
}

// HostedZoneVPCAssociationReconciler associates VPCs, possibly from other
// accounts, with private hosted zones.
type HostedZoneVPCAssociationReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Route53Client HostedZoneVPCAssociationAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=hostedzonevpcassociations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=hostedzonevpcassociations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=hostedzonevpcassociations/finalizers,verbs=update

func (r *HostedZoneVPCAssociationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	obj := &awsv1alpha1.HostedZoneVPCAssociation{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, obj); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !obj.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
			if shouldAbandon(obj) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, obj)
			}
			if err := r.deleteAssociation(ctx, obj); err != nil {
				logger.Error(err, "failed to disassociate VPC from hosted zone")
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
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileAssociation(ctx, obj); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *HostedZoneVPCAssociationReconciler) resolveIDs(ctx context.Context, obj *awsv1alpha1.HostedZoneVPCAssociation) (zoneID, vpcID string, err error) {
	zoneID = obj.Spec.HostedZoneRef.ID
	if zoneID == "" {
		if obj.Spec.HostedZoneRef.Name == "" {
			return "", "", fmt.Errorf("hostedZoneRef requires name or id")
		}
		ns := obj.Spec.HostedZoneRef.Namespace
		if ns == "" {
			ns = obj.Namespace
		}
		hz := &awsv1alpha1.HostedZone{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: obj.Spec.HostedZoneRef.Name, Namespace: ns}, hz); err != nil {
			return "", "", err
		}
		if hz.Status.HostedZoneID == "" {
			return "", "", &dependencyNotReady{msg: fmt.Sprintf("HostedZone %s/%s has no hostedZoneId yet", ns, obj.Spec.HostedZoneRef.Name)}
		}
		zoneID = hz.Status.HostedZoneID
	}
	zoneID = route53helper.StripZonePrefix(zoneID)

	vpcID = obj.Spec.VPCRef.ID
	if vpcID == "" {
		if obj.Spec.VPCRef.Name == "" {
			return "", "", fmt.Errorf("vpcRef requires name or id")
		}
		vpc := &awsv1alpha1.VPC{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: obj.Spec.VPCRef.Name, Namespace: obj.Namespace}, vpc); err != nil {
			return "", "", err
		}
		if vpc.Status.VPCID == "" {
			return "", "", &dependencyNotReady{msg: fmt.Sprintf("VPC %s/%s has no vpcId yet", obj.Namespace, obj.Spec.VPCRef.Name)}
		}
		vpcID = vpc.Status.VPCID
	}
	return zoneID, vpcID, nil
}

func (r *HostedZoneVPCAssociationReconciler) reconcileAssociation(ctx context.Context, obj *awsv1alpha1.HostedZoneVPCAssociation) error {
	zoneID, vpcID, err := r.resolveIDs(ctx, obj)
	if err != nil {
		return err
	}
	obj.Status.HostedZoneID = zoneID
	obj.Status.VPCID = vpcID
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return err
	}

	zone, err := r.Route53Client.GetHostedZone(ctx, &awsroute53.GetHostedZoneInput{Id: aws.String(zoneID)})
	if err != nil {
		return fmt.Errorf("get hosted zone: %w", err)
	}
	associated := false
	for _, v := range zone.VPCs {
		if aws.ToString(v.VPCId) == vpcID {
			associated = true
			break
		}
	}

	if !associated {
		vpc := &route53types.VPC{VPCId: aws.String(vpcID), VPCRegion: route53types.VPCRegion(obj.Spec.VPCRegion)}
		crossAccount := obj.Spec.VPCProviderRef != nil && obj.Spec.VPCProviderRef.Name != ""
		if crossAccount {
			if _, err := r.Route53Client.CreateVPCAssociationAuthorization(ctx, &awsroute53.CreateVPCAssociationAuthorizationInput{
				HostedZoneId: aws.String(zoneID), VPC: vpc,
			}); err != nil {
				return fmt.Errorf("create VPC association authorization: %w", err)
			}
		}
		vctx := ctx
		if crossAccount {
			vctx, err = crossAccountContext(ctx, obj.Namespace, obj.Spec.VPCProviderRef, "")
			if err != nil {
				return err
			}
		}
		if _, err := r.Route53Client.AssociateVPCWithHostedZone(vctx, &awsroute53.AssociateVPCWithHostedZoneInput{
			HostedZoneId: aws.String(zoneID), VPC: vpc,
			Comment: aws.String("konfig-konector " + obj.Namespace + "/" + obj.Name),
		}); err != nil && !isRoute53Conflict(err) {
			return fmt.Errorf("associate VPC with hosted zone: %w", err)
		}
		if crossAccount {
			// Best effort: the authorization is single-use once consumed.
			_, _ = r.Route53Client.DeleteVPCAssociationAuthorization(ctx, &awsroute53.DeleteVPCAssociationAuthorizationInput{
				HostedZoneId: aws.String(zoneID), VPC: vpc,
			})
		}
	}

	obj.Status.Associated = true
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "VPC associated with hosted zone")
}

func (r *HostedZoneVPCAssociationReconciler) deleteAssociation(ctx context.Context, obj *awsv1alpha1.HostedZoneVPCAssociation) error {
	if obj.Status.HostedZoneID == "" || obj.Status.VPCID == "" {
		return nil
	}
	vctx := ctx
	if obj.Spec.VPCProviderRef != nil && obj.Spec.VPCProviderRef.Name != "" {
		var err error
		if vctx, err = crossAccountContext(ctx, obj.Namespace, obj.Spec.VPCProviderRef, ""); err != nil {
			return err
		}
	}
	_, err := r.Route53Client.DisassociateVPCFromHostedZone(vctx, &awsroute53.DisassociateVPCFromHostedZoneInput{
		HostedZoneId: aws.String(obj.Status.HostedZoneID),
		VPC:          &route53types.VPC{VPCId: aws.String(obj.Status.VPCID), VPCRegion: route53types.VPCRegion(obj.Spec.VPCRegion)},
	})
	if err == nil || route53helper.IsNotFound(err) {
		return nil
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode() == "VPCAssociationNotFound" {
		return nil
	}
	return err
}

// isRoute53Conflict reports the "already associated" family of errors.
func isRoute53Conflict(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "ConflictingDomainExists", "PublicZoneVPCAssociation":
			return false
		}
		return apiErr.ErrorCode() == "VPCAssociationAlreadyExists"
	}
	return false
}

func (r *HostedZoneVPCAssociationReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.HostedZoneVPCAssociation, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
		Type: condType, Status: status, ObservedGeneration: obj.Generation, Reason: reason, Message: message,
	})
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *HostedZoneVPCAssociationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.HostedZoneVPCAssociation{}).
		Complete(r)
}
