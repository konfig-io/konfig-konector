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
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsroute53 "github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	r53helper "github.com/konfig-io/konfig-konector/internal/aws/route53"
)

// HostedZoneAWSAPI is the subset of the Route53 API used by
// HostedZoneReconciler. It is satisfied by *route53.Client.
type HostedZoneAWSAPI interface {
	CreateHostedZone(ctx context.Context, params *awsroute53.CreateHostedZoneInput, optFns ...func(*awsroute53.Options)) (*awsroute53.CreateHostedZoneOutput, error)
	GetHostedZone(ctx context.Context, params *awsroute53.GetHostedZoneInput, optFns ...func(*awsroute53.Options)) (*awsroute53.GetHostedZoneOutput, error)
	ListHostedZonesByName(ctx context.Context, params *awsroute53.ListHostedZonesByNameInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ListHostedZonesByNameOutput, error)
	DeleteHostedZone(ctx context.Context, params *awsroute53.DeleteHostedZoneInput, optFns ...func(*awsroute53.Options)) (*awsroute53.DeleteHostedZoneOutput, error)
	ListResourceRecordSets(ctx context.Context, params *awsroute53.ListResourceRecordSetsInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ListResourceRecordSetsOutput, error)
	ChangeResourceRecordSets(ctx context.Context, params *awsroute53.ChangeResourceRecordSetsInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ChangeResourceRecordSetsOutput, error)
	ListTagsForResource(ctx context.Context, params *awsroute53.ListTagsForResourceInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ListTagsForResourceOutput, error)
	ChangeTagsForResource(ctx context.Context, params *awsroute53.ChangeTagsForResourceInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ChangeTagsForResourceOutput, error)
}

// HostedZoneReconciler reconciles HostedZone objects.
type HostedZoneReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	Route53Client   HostedZoneAWSAPI
	CallerReference string // set once at startup (e.g. operator pod name)
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=hostedzones,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=hostedzones/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=hostedzones/finalizers,verbs=update

func (r *HostedZoneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	hz := &awsv1alpha1.HostedZone{}
	if err := r.Get(ctx, req.NamespacedName, hz); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, hz); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !hz.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(hz, awsv1alpha1.FinalizerName) {
			if shouldAbandon(hz) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(hz, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, hz)
			}
			if err := r.deleteHostedZone(ctx, hz); err != nil {
				logger.Error(err, "failed to delete hosted zone", "zoneId", hz.Status.HostedZoneID)
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(hz, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, hz)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(hz, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(hz, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, hz); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileHostedZone(ctx, hz); err != nil {
		logger.Error(err, "reconcile error", "zoneName", hz.Spec.Name)
		_ = r.setHZCondition(ctx, hz, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *HostedZoneReconciler) reconcileHostedZone(ctx context.Context, hz *awsv1alpha1.HostedZone) error {
	var zoneID string

	if hz.Status.HostedZoneID != "" {
		existing, err := r53helper.GetHostedZone(ctx, r.Route53Client, hz.Status.HostedZoneID)
		if err != nil {
			return err
		}
		if existing != nil {
			zoneID = hz.Status.HostedZoneID
		}
	}

	if zoneID == "" {
		// Create the hosted zone.
		input := &awsroute53.CreateHostedZoneInput{
			Name:            aws.String(hz.Spec.Name),
			CallerReference: aws.String(hostedZoneCallerRef(string(hz.UID))),
			HostedZoneConfig: &types.HostedZoneConfig{
				Comment:     aws.String(hz.Spec.Comment),
				PrivateZone: hz.Spec.Private,
			},
		}
		if hz.Spec.DelegationSetID != "" {
			input.DelegationSetId = aws.String(hz.Spec.DelegationSetID)
		}
		if hz.Spec.Private && hz.Spec.VPCRef != nil {
			input.VPC = &types.VPC{
				VPCId:     aws.String(hz.Spec.VPCRef.ID),
				VPCRegion: types.VPCRegion(hz.Spec.VPCRef.Region),
			}
		}
		out, err := r.Route53Client.CreateHostedZone(ctx, input)
		if err != nil {
			return fmt.Errorf("create hosted zone: %w", err)
		}
		zoneID = r53helper.StripZonePrefix(aws.ToString(out.HostedZone.Id))

		// Collect name servers.
		if out.DelegationSet != nil {
			hz.Status.NameServers = out.DelegationSet.NameServers
		}

		// The AWS resource now exists; losing the ID would orphan it.
		hz.Status.HostedZoneID = zoneID
		if err := persistStatus(ctx, r.Client, hz); err != nil {
			return fmt.Errorf("persist status after create: %w", err)
		}
	}

	hz.Status.HostedZoneID = zoneID

	// Sync tags.
	if err := r53helper.SyncTags(ctx, r.Route53Client, zoneID, hz.Spec.Tags); err != nil {
		return fmt.Errorf("sync tags: %w", err)
	}

	// Refresh name servers if missing.
	if len(hz.Status.NameServers) == 0 {
		existing, err := r53helper.GetHostedZone(ctx, r.Route53Client, zoneID)
		if err != nil {
			return err
		}
		if existing != nil {
			nsOut, err := r.Route53Client.GetHostedZone(ctx, &awsroute53.GetHostedZoneInput{Id: aws.String(zoneID)})
			if err == nil && nsOut.DelegationSet != nil {
				hz.Status.NameServers = nsOut.DelegationSet.NameServers
			}
		}
	}

	hz.Status.ObservedGeneration = hz.Generation
	now := metav1.Now()
	hz.Status.LastSyncTime = &now
	return r.setHZCondition(ctx, hz, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "hosted zone reconciled")
}

// hostedZoneCallerRef produces the deterministic CallerReference used at
// create time, derived from the CR UID.
func hostedZoneCallerRef(uid string) string {
	return "kk-" + fmt.Sprintf("%x", sha256.Sum256([]byte(uid)))[:16]
}

// deleteHostedZone deletes the zone recorded in status. If the status ID was
// lost after a successful create, it falls back to ListHostedZonesByName and
// deletes only a zone whose name matches spec.name exactly AND whose
// CallerReference matches the one derived from this CR's UID; anything
// ambiguous is skipped.
func (r *HostedZoneReconciler) deleteHostedZone(ctx context.Context, hz *awsv1alpha1.HostedZone) error {
	zoneID := hz.Status.HostedZoneID
	if zoneID == "" {
		wantName := hz.Spec.Name
		if !strings.HasSuffix(wantName, ".") {
			wantName += "."
		}
		wantRef := hostedZoneCallerRef(string(hz.UID))
		listOut, err := r.Route53Client.ListHostedZonesByName(ctx, &awsroute53.ListHostedZonesByNameInput{
			DNSName: aws.String(hz.Spec.Name),
		})
		if err != nil {
			return fmt.Errorf("list hosted zones by name: %w", err)
		}
		for _, zone := range listOut.HostedZones {
			if aws.ToString(zone.Name) != wantName {
				continue
			}
			if aws.ToString(zone.CallerReference) != wantRef {
				// Same name but not created by this CR; skip.
				continue
			}
			zoneID = r53helper.StripZonePrefix(aws.ToString(zone.Id))
			break
		}
		if zoneID == "" {
			return nil
		}
	}
	return r53helper.DeleteHostedZone(ctx, r.Route53Client, zoneID)
}

func (r *HostedZoneReconciler) setHZCondition(ctx context.Context, hz *awsv1alpha1.HostedZone, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&hz.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: hz.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, hz); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *HostedZoneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.HostedZone{}).
		Complete(r)
}
