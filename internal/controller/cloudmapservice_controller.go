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
	awssd "github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	sdtypes "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	sdhelper "github.com/konfig-io/konfig-konector/internal/aws/servicediscovery"
)

// CloudMapServiceAWSAPI is the subset of the Cloud Map API used by this controller.
type CloudMapServiceAWSAPI interface {
	CreateService(ctx context.Context, params *awssd.CreateServiceInput, optFns ...func(*awssd.Options)) (*awssd.CreateServiceOutput, error)
	GetService(ctx context.Context, params *awssd.GetServiceInput, optFns ...func(*awssd.Options)) (*awssd.GetServiceOutput, error)
	UpdateService(ctx context.Context, params *awssd.UpdateServiceInput, optFns ...func(*awssd.Options)) (*awssd.UpdateServiceOutput, error)
	DeleteService(ctx context.Context, params *awssd.DeleteServiceInput, optFns ...func(*awssd.Options)) (*awssd.DeleteServiceOutput, error)
}

// CloudMapServiceReconciler reconciles CloudMapService objects.
type CloudMapServiceReconciler struct {
	client.Client
	Scheme                 *runtime.Scheme
	ServiceDiscoveryClient CloudMapServiceAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudmapservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudmapservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudmapservices/finalizers,verbs=update

func (r *CloudMapServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	svc := &awsv1alpha1.CloudMapService{}
	if err := r.Get(ctx, req.NamespacedName, svc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, svc); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !svc.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(svc, awsv1alpha1.FinalizerName) {
			if shouldAbandon(svc) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(svc, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, svc)
			}
			if err := r.deleteService(ctx, svc); err != nil {
				logger.Error(err, "failed to delete Cloud Map service")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(svc, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, svc)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(svc, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(svc, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, svc); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileService(ctx, svc); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, svc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *CloudMapServiceReconciler) reconcileService(ctx context.Context, svc *awsv1alpha1.CloudMapService) error {
	if svc.Status.ServiceID != "" {
		got, err := r.GetServiceExisting(ctx, svc)
		if err != nil && !sdhelper.IsNotFound(err) {
			return err
		}
		if err == nil && got != nil {
			if svc.Status.ObservedGeneration != svc.Generation {
				change := &sdtypes.ServiceChange{}
				if svc.Spec.Description != "" {
					change.Description = aws.String(svc.Spec.Description)
				}
				if svc.Spec.DnsConfig != nil {
					change.DnsConfig = &sdtypes.DnsConfigChange{
						DnsRecords: []sdtypes.DnsRecord{{
							Type: sdtypes.RecordType(svc.Spec.DnsConfig.RecordType),
							TTL:  aws.Int64(svc.Spec.DnsConfig.TTL),
						}},
					}
				}
				if _, err := r.ServiceDiscoveryClient.UpdateService(ctx, &awssd.UpdateServiceInput{
					Id:      aws.String(svc.Status.ServiceID),
					Service: change,
				}); err != nil {
					return fmt.Errorf("update Cloud Map service: %w", err)
				}
			}
			svc.Status.ObservedGeneration = svc.Generation
			now := metav1.Now()
			svc.Status.LastSyncTime = &now
			return r.setCondition(ctx, svc, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Cloud Map service reconciled")
		}
		// NotFound: fall through to create.
	}

	namespaceID, err := r.resolveNamespaceID(ctx, svc)
	if err != nil {
		return err
	}

	createIn := &awssd.CreateServiceInput{
		Name:        aws.String(svc.Spec.Name),
		NamespaceId: aws.String(namespaceID),
	}
	if svc.Spec.Description != "" {
		createIn.Description = aws.String(svc.Spec.Description)
	}
	if dc := svc.Spec.DnsConfig; dc != nil {
		dnsCfg := &sdtypes.DnsConfig{
			DnsRecords: []sdtypes.DnsRecord{{
				Type: sdtypes.RecordType(dc.RecordType),
				TTL:  aws.Int64(dc.TTL),
			}},
		}
		if dc.RoutingPolicy != "" {
			dnsCfg.RoutingPolicy = sdtypes.RoutingPolicy(dc.RoutingPolicy)
		}
		createIn.DnsConfig = dnsCfg
	}
	if hc := svc.Spec.HealthCheckCustomConfig; hc != nil {
		cfg := &sdtypes.HealthCheckCustomConfig{}
		if hc.FailureThreshold > 0 {
			cfg.FailureThreshold = aws.Int32(hc.FailureThreshold)
		}
		createIn.HealthCheckCustomConfig = cfg
	}
	createIn.Tags = cloudMapTags(svc.Spec.Tags)

	created, err := r.ServiceDiscoveryClient.CreateService(ctx, createIn)
	if err != nil {
		return fmt.Errorf("create Cloud Map service: %w", err)
	}
	if created.Service != nil {
		svc.Status.ServiceID = aws.ToString(created.Service.Id)
		svc.Status.ARN = aws.ToString(created.Service.Arn)
	}
	// Persist the service ID immediately: the AWS resource now exists, and
	// losing the identifier would orphan it or create a duplicate on retry.
	if err := persistStatus(ctx, r.Client, svc); err != nil {
		return fmt.Errorf("persist service ID after create: %w", err)
	}
	svc.Status.ObservedGeneration = svc.Generation
	now := metav1.Now()
	svc.Status.LastSyncTime = &now
	return r.setCondition(ctx, svc, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Cloud Map service created")
}

// GetServiceExisting fetches the AWS service for the stored ID.
func (r *CloudMapServiceReconciler) GetServiceExisting(ctx context.Context, svc *awsv1alpha1.CloudMapService) (*sdtypes.Service, error) {
	out, err := r.ServiceDiscoveryClient.GetService(ctx, &awssd.GetServiceInput{
		Id: aws.String(svc.Status.ServiceID),
	})
	if err != nil {
		return nil, err
	}
	return out.Service, nil
}

func (r *CloudMapServiceReconciler) resolveNamespaceID(ctx context.Context, svc *awsv1alpha1.CloudMapService) (string, error) {
	ref := svc.Spec.NamespaceRef
	if ref.ID != "" {
		return ref.ID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("namespaceRef requires either name or id")
	}
	nsCR := &awsv1alpha1.CloudMapNamespace{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: svc.Namespace}, nsCR); err != nil {
		return "", err
	}
	if nsCR.Status.NamespaceID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("CloudMapNamespace %s/%s has no ID yet", svc.Namespace, ref.Name)}
	}
	return nsCR.Status.NamespaceID, nil
}

func (r *CloudMapServiceReconciler) deleteService(ctx context.Context, svc *awsv1alpha1.CloudMapService) error {
	if svc.Status.ServiceID == "" {
		// The service ID is server-generated; without it (and without a
		// resolvable namespace) there is no unambiguous lookup, so nothing
		// to delete.
		return nil
	}
	_, err := r.ServiceDiscoveryClient.DeleteService(ctx, &awssd.DeleteServiceInput{
		Id: aws.String(svc.Status.ServiceID),
	})
	if sdhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *CloudMapServiceReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.CloudMapService, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *CloudMapServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CloudMapService{}).
		Complete(r)
}
