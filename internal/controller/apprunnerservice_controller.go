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
	awsapprunner "github.com/aws/aws-sdk-go-v2/service/apprunner"
	apprunnertypes "github.com/aws/aws-sdk-go-v2/service/apprunner/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	apprunnerhelper "github.com/konfig-io/konfig-konector/internal/aws/apprunner"
)

var requeueAppRunnerPolling = ctrl.Result{RequeueAfter: 30 * time.Second}

// AppRunnerServiceAWSAPI is the subset of the App Runner API used by this controller.
type AppRunnerServiceAWSAPI interface {
	CreateService(ctx context.Context, params *awsapprunner.CreateServiceInput, optFns ...func(*awsapprunner.Options)) (*awsapprunner.CreateServiceOutput, error)
	DescribeService(ctx context.Context, params *awsapprunner.DescribeServiceInput, optFns ...func(*awsapprunner.Options)) (*awsapprunner.DescribeServiceOutput, error)
	UpdateService(ctx context.Context, params *awsapprunner.UpdateServiceInput, optFns ...func(*awsapprunner.Options)) (*awsapprunner.UpdateServiceOutput, error)
	DeleteService(ctx context.Context, params *awsapprunner.DeleteServiceInput, optFns ...func(*awsapprunner.Options)) (*awsapprunner.DeleteServiceOutput, error)
	ListServices(ctx context.Context, params *awsapprunner.ListServicesInput, optFns ...func(*awsapprunner.Options)) (*awsapprunner.ListServicesOutput, error)
}

// AppRunnerServiceReconciler reconciles AppRunnerService objects.
type AppRunnerServiceReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	AppRunnerClient AppRunnerServiceAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=apprunnerservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=apprunnerservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=apprunnerservices/finalizers,verbs=update

func (r *AppRunnerServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	svc := &awsv1alpha1.AppRunnerService{}
	if err := r.Get(ctx, req.NamespacedName, svc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !svc.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(svc, awsv1alpha1.FinalizerName) {
			if shouldAbandon(svc) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(svc, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, svc)
			}
			if err := r.deleteService(ctx, svc); err != nil {
				logger.Error(err, "failed to delete App Runner service")
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

	result, err := r.reconcileService(ctx, svc)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, svc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *AppRunnerServiceReconciler) reconcileService(ctx context.Context, svc *awsv1alpha1.AppRunnerService) (ctrl.Result, error) {
	if svc.Status.ServiceARN != "" {
		out, err := r.AppRunnerClient.DescribeService(ctx, &awsapprunner.DescribeServiceInput{
			ServiceArn: aws.String(svc.Status.ServiceARN),
		})
		if err != nil && !apprunnerhelper.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err == nil && out.Service != nil {
			s := out.Service
			svc.Status.Status = string(s.Status)
			svc.Status.ServiceID = aws.ToString(s.ServiceId)
			svc.Status.ServiceURL = aws.ToString(s.ServiceUrl)

			switch s.Status {
			case apprunnertypes.ServiceStatusCreateFailed:
				_ = r.setCondition(ctx, svc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, "App Runner service is CREATE_FAILED")
				return requeueResult(), nil
			case apprunnertypes.ServiceStatusRunning:
				// fall through to update handling below
			default:
				_ = r.setCondition(ctx, svc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning",
					fmt.Sprintf("App Runner service is %s", s.Status))
				return requeueAppRunnerPolling, nil
			}

			if svc.Status.ObservedGeneration != svc.Generation {
				sourceCfg, err := r.buildSourceConfiguration(ctx, svc)
				if err != nil {
					return ctrl.Result{}, err
				}
				instanceCfg, err := r.buildInstanceConfiguration(ctx, svc)
				if err != nil {
					return ctrl.Result{}, err
				}
				updateIn := &awsapprunner.UpdateServiceInput{
					ServiceArn:               aws.String(svc.Status.ServiceARN),
					SourceConfiguration:      sourceCfg,
					InstanceConfiguration:    instanceCfg,
					HealthCheckConfiguration: buildAppRunnerHealthCheck(svc.Spec.HealthCheckConfiguration),
				}
				if _, err := r.AppRunnerClient.UpdateService(ctx, updateIn); err != nil {
					return ctrl.Result{}, fmt.Errorf("update App Runner service: %w", err)
				}
				svc.Status.ObservedGeneration = svc.Generation
				if err := persistStatus(ctx, r.Client, svc); err != nil {
					return ctrl.Result{}, err
				}
				_ = r.setCondition(ctx, svc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Updating", "App Runner service update in progress")
				return requeueAppRunnerPolling, nil
			}

			svc.Status.ObservedGeneration = svc.Generation
			now := metav1.Now()
			svc.Status.LastSyncTime = &now
			return requeueResult(), r.setCondition(ctx, svc, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "App Runner service running")
		}
		// NotFound: fall through to create.
	}

	sourceCfg, err := r.buildSourceConfiguration(ctx, svc)
	if err != nil {
		return ctrl.Result{}, err
	}
	instanceCfg, err := r.buildInstanceConfiguration(ctx, svc)
	if err != nil {
		return ctrl.Result{}, err
	}

	createIn := &awsapprunner.CreateServiceInput{
		ServiceName:              aws.String(svc.Spec.ServiceName),
		SourceConfiguration:      sourceCfg,
		InstanceConfiguration:    instanceCfg,
		HealthCheckConfiguration: buildAppRunnerHealthCheck(svc.Spec.HealthCheckConfiguration),
	}
	for k, v := range svc.Spec.Tags {
		k, v := k, v
		createIn.Tags = append(createIn.Tags, apprunnertypes.Tag{Key: &k, Value: &v})
	}

	created, err := r.AppRunnerClient.CreateService(ctx, createIn)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create App Runner service: %w", err)
	}
	if created.Service != nil {
		svc.Status.ServiceARN = aws.ToString(created.Service.ServiceArn)
		svc.Status.ServiceID = aws.ToString(created.Service.ServiceId)
		svc.Status.ServiceURL = aws.ToString(created.Service.ServiceUrl)
		svc.Status.Status = string(created.Service.Status)
	}
	// Persist the ARN immediately: the AWS resource now exists, and losing
	// the identifier would orphan it or create a duplicate on retry.
	if err := persistStatus(ctx, r.Client, svc); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist service ARN after create: %w", err)
	}
	_ = r.setCondition(ctx, svc, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "App Runner service is being created")
	return requeueAppRunnerPolling, nil
}

func (r *AppRunnerServiceReconciler) buildSourceConfiguration(ctx context.Context, svc *awsv1alpha1.AppRunnerService) (*apprunnertypes.SourceConfiguration, error) {
	src := svc.Spec.SourceConfiguration
	imageRepo := &apprunnertypes.ImageRepository{
		ImageIdentifier:     aws.String(src.ImageRepository.ImageIdentifier),
		ImageRepositoryType: apprunnertypes.ImageRepositoryType(src.ImageRepository.ImageRepositoryType),
	}
	if ic := src.ImageRepository.ImageConfiguration; ic != nil {
		imageCfg := &apprunnertypes.ImageConfiguration{}
		if ic.Port != "" {
			imageCfg.Port = aws.String(ic.Port)
		}
		if len(ic.RuntimeEnvironmentVariables) > 0 {
			imageCfg.RuntimeEnvironmentVariables = ic.RuntimeEnvironmentVariables
		}
		if ic.StartCommand != "" {
			imageCfg.StartCommand = aws.String(ic.StartCommand)
		}
		imageRepo.ImageConfiguration = imageCfg
	}
	cfg := &apprunnertypes.SourceConfiguration{
		ImageRepository:        imageRepo,
		AutoDeploymentsEnabled: src.AutoDeploymentsEnabled,
	}
	if ac := src.AuthenticationConfiguration; ac != nil {
		auth := &apprunnertypes.AuthenticationConfiguration{}
		if ac.AccessRoleArn != "" {
			auth.AccessRoleArn = aws.String(ac.AccessRoleArn)
		} else if ac.AccessRoleRef != nil {
			arn, err := resolveIAMRoleARN(ctx, r.Client, svc.Namespace, *ac.AccessRoleRef)
			if err != nil {
				return nil, err
			}
			auth.AccessRoleArn = aws.String(arn)
		}
		cfg.AuthenticationConfiguration = auth
	}
	return cfg, nil
}

func (r *AppRunnerServiceReconciler) buildInstanceConfiguration(ctx context.Context, svc *awsv1alpha1.AppRunnerService) (*apprunnertypes.InstanceConfiguration, error) {
	ic := svc.Spec.InstanceConfiguration
	if ic == nil {
		return nil, nil
	}
	cfg := &apprunnertypes.InstanceConfiguration{}
	if ic.CPU != "" {
		cfg.Cpu = aws.String(ic.CPU)
	}
	if ic.Memory != "" {
		cfg.Memory = aws.String(ic.Memory)
	}
	if ic.InstanceRoleArn != "" {
		cfg.InstanceRoleArn = aws.String(ic.InstanceRoleArn)
	} else if ic.InstanceRoleRef != nil {
		arn, err := resolveIAMRoleARN(ctx, r.Client, svc.Namespace, *ic.InstanceRoleRef)
		if err != nil {
			return nil, err
		}
		cfg.InstanceRoleArn = aws.String(arn)
	}
	return cfg, nil
}

func buildAppRunnerHealthCheck(hc *awsv1alpha1.AppRunnerHealthCheckConfiguration) *apprunnertypes.HealthCheckConfiguration {
	if hc == nil {
		return nil
	}
	cfg := &apprunnertypes.HealthCheckConfiguration{}
	if hc.Protocol != "" {
		cfg.Protocol = apprunnertypes.HealthCheckProtocol(hc.Protocol)
	}
	if hc.Path != "" {
		cfg.Path = aws.String(hc.Path)
	}
	if hc.Interval > 0 {
		cfg.Interval = aws.Int32(hc.Interval)
	}
	if hc.Timeout > 0 {
		cfg.Timeout = aws.Int32(hc.Timeout)
	}
	if hc.HealthyThreshold > 0 {
		cfg.HealthyThreshold = aws.Int32(hc.HealthyThreshold)
	}
	if hc.UnhealthyThreshold > 0 {
		cfg.UnhealthyThreshold = aws.Int32(hc.UnhealthyThreshold)
	}
	return cfg
}

func (r *AppRunnerServiceReconciler) deleteService(ctx context.Context, svc *awsv1alpha1.AppRunnerService) error {
	serviceARN := svc.Status.ServiceARN
	if serviceARN == "" {
		// Service names are unique per account/region: look the ARN up by name.
		out, err := r.AppRunnerClient.ListServices(ctx, &awsapprunner.ListServicesInput{})
		if err != nil {
			return err
		}
		for _, s := range out.ServiceSummaryList {
			if aws.ToString(s.ServiceName) == svc.Spec.ServiceName {
				serviceARN = aws.ToString(s.ServiceArn)
				break
			}
		}
		if serviceARN == "" {
			return nil
		}
	}
	_, err := r.AppRunnerClient.DeleteService(ctx, &awsapprunner.DeleteServiceInput{
		ServiceArn: aws.String(serviceARN),
	})
	if apprunnerhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *AppRunnerServiceReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.AppRunnerService, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *AppRunnerServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.AppRunnerService{}).
		Complete(r)
}
