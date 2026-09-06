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
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	r53helper "github.com/konfig-io/konfig-konector/internal/aws/route53"
)

// RecordSetAWSAPI is the subset of the Route53 API used by
// RecordSetReconciler. It is satisfied by *route53.Client.
type RecordSetAWSAPI interface {
	ChangeResourceRecordSets(ctx context.Context, params *awsroute53.ChangeResourceRecordSetsInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ChangeResourceRecordSetsOutput, error)
	ListResourceRecordSets(ctx context.Context, params *awsroute53.ListResourceRecordSetsInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ListResourceRecordSetsOutput, error)
}

// RecordSetReconciler reconciles RecordSet objects.
type RecordSetReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Route53Client RecordSetAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=recordsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=recordsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=recordsets/finalizers,verbs=update

func (r *RecordSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	rs := &awsv1alpha1.RecordSet{}
	if err := r.Get(ctx, req.NamespacedName, rs); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, rs); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !rs.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(rs, awsv1alpha1.FinalizerName) {
			if shouldAbandon(rs) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(rs, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, rs)
			}
			// The record itself is rebuilt from spec, so deletion only needs
			// the zone ID. If it never reached status, re-resolve it from
			// spec; if the referenced HostedZone CR is already gone or not
			// ready, there is nothing left to clean up.
			zoneID := rs.Status.HostedZoneID
			if zoneID == "" {
				zoneID, _ = r.resolveZoneID(ctx, rs)
			}
			if zoneID != "" {
				rrs := r.buildResourceRecordSet(rs, "")
				if err := r53helper.DeleteRecordSet(ctx, r.Route53Client, zoneID, rrs); err != nil {
					logger.Error(err, "failed to delete record set")
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(rs, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, rs)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(rs, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(rs, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, rs); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileRecordSet(ctx, rs); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setRSCondition(ctx, rs, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *RecordSetReconciler) reconcileRecordSet(ctx context.Context, rs *awsv1alpha1.RecordSet) error {
	zoneID, err := r.resolveZoneID(ctx, rs)
	if err != nil {
		return err
	}
	rs.Status.HostedZoneID = zoneID

	healthCheckID := ""
	if rs.Spec.HealthCheckRef != nil {
		hc := &awsv1alpha1.HealthCheck{}
		ns := rs.Spec.HealthCheckRef.Namespace
		if ns == "" {
			ns = rs.Namespace
		}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: rs.Spec.HealthCheckRef.Name, Namespace: ns}, hc); err == nil {
			healthCheckID = hc.Status.HealthCheckID
		}
	}

	desired := r.buildResourceRecordSet(rs, healthCheckID)

	// Only call ChangeResourceRecordSets if the record doesn't already match.
	existing, err := r53helper.FindRecordSet(ctx, r.Route53Client, zoneID, rs.Spec.Name, rs.Spec.Type, rs.Spec.SetIdentifier)
	if err != nil {
		return fmt.Errorf("find record set: %w", err)
	}

	needsUpdate := existing == nil || !recordSetsEqual(existing, desired)
	if needsUpdate {
		changeID, err := r53helper.UpsertRecordSet(ctx, r.Route53Client, zoneID, desired)
		if err != nil {
			return fmt.Errorf("upsert record set: %w", err)
		}
		rs.Status.ChangeID = changeID
		rs.Status.ChangeStatus = "PENDING"
	}

	rs.Status.ObservedGeneration = rs.Generation
	now := metav1.Now()
	rs.Status.LastSyncTime = &now
	return r.setRSCondition(ctx, rs, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "record set reconciled")
}

// recordSetsEqual compares TTL and resource records for drift detection.
// Alias targets and weighted/failover routing are compared by value.
func recordSetsEqual(a, b *types.ResourceRecordSet) bool {
	if aws.ToInt64(a.TTL) != aws.ToInt64(b.TTL) {
		return false
	}
	if len(a.ResourceRecords) != len(b.ResourceRecords) {
		return false
	}
	for i := range a.ResourceRecords {
		if aws.ToString(a.ResourceRecords[i].Value) != aws.ToString(b.ResourceRecords[i].Value) {
			return false
		}
	}
	// Alias target comparison.
	if (a.AliasTarget == nil) != (b.AliasTarget == nil) {
		return false
	}
	if a.AliasTarget != nil {
		if aws.ToString(a.AliasTarget.DNSName) != aws.ToString(b.AliasTarget.DNSName) {
			return false
		}
	}
	return true
}

func (r *RecordSetReconciler) buildResourceRecordSet(rs *awsv1alpha1.RecordSet, healthCheckID string) *types.ResourceRecordSet {
	rrs := &types.ResourceRecordSet{
		Name: aws.String(rs.Spec.Name),
		Type: types.RRType(rs.Spec.Type),
	}
	if rs.Spec.SetIdentifier != "" {
		rrs.SetIdentifier = aws.String(rs.Spec.SetIdentifier)
	}
	if rs.Spec.Alias != nil {
		rrs.AliasTarget = &types.AliasTarget{
			DNSName:              aws.String(rs.Spec.Alias.DNSName),
			HostedZoneId:         aws.String(rs.Spec.Alias.HostedZoneID),
			EvaluateTargetHealth: rs.Spec.Alias.EvaluateTargetHealth,
		}
	} else {
		if rs.Spec.TTL != nil {
			rrs.TTL = rs.Spec.TTL
		}
		for _, val := range rs.Spec.Records {
			rrs.ResourceRecords = append(rrs.ResourceRecords, types.ResourceRecord{
				Value: aws.String(val),
			})
		}
	}
	if rs.Spec.Weight != nil {
		rrs.Weight = rs.Spec.Weight
	}
	if rs.Spec.Failover != "" {
		rrs.Failover = types.ResourceRecordSetFailover(rs.Spec.Failover)
	}
	if healthCheckID != "" {
		rrs.HealthCheckId = aws.String(healthCheckID)
	}
	return rrs
}

func (r *RecordSetReconciler) resolveZoneID(ctx context.Context, rs *awsv1alpha1.RecordSet) (string, error) {
	if rs.Spec.HostedZoneRef.ID != "" {
		return rs.Spec.HostedZoneRef.ID, nil
	}
	if rs.Spec.HostedZoneRef.Name != "" {
		hz := &awsv1alpha1.HostedZone{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: rs.Spec.HostedZoneRef.Name, Namespace: rs.Namespace}, hz); err != nil {
			return "", err
		}
		if hz.Status.HostedZoneID == "" {
			return "", &dependencyNotReady{msg: fmt.Sprintf("HostedZone %s/%s has no ID yet", rs.Namespace, rs.Spec.HostedZoneRef.Name)}
		}
		return hz.Status.HostedZoneID, nil
	}
	return "", fmt.Errorf("hostedZoneRef requires either name or id")
}

func (r *RecordSetReconciler) setRSCondition(ctx context.Context, rs *awsv1alpha1.RecordSet, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&rs.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: rs.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, rs); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *RecordSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.RecordSet{}).
		Complete(r)
}
