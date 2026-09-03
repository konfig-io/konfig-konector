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
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
)

func init() {
	RegisterSetup(func(mgr ctrl.Manager, clients *awsclient.Clients) error {
		if err := (&ScalableTargetReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			AASClient: clients.AppAutoScaling}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("ScalableTarget: %w", err)
		}
		if err := (&AppScalingPolicyReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			AASClient: clients.AppAutoScaling}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("AppScalingPolicy: %w", err)
		}
		if err := (&ScheduleGroupReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			SchedulerClient: clients.Scheduler}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("ScheduleGroup: %w", err)
		}
		if err := (&ScheduleReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			SchedulerClient: clients.Scheduler}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("Schedule: %w", err)
		}
		if err := (&SSMMaintenanceWindowReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			SSMClient: clients.SSM}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("SSMMaintenanceWindow: %w", err)
		}
		if err := (&SSMPatchBaselineReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			SSMClient: clients.SSM}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("SSMPatchBaseline: %w", err)
		}
		if err := (&SSMAssociationReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			SSMClient: clients.SSM}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("SSMAssociation: %w", err)
		}
		if err := (&RDSGlobalClusterReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			RDSClient: clients.RDS}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("RDSGlobalCluster: %w", err)
		}
		if err := (&RDSEventSubscriptionReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			RDSClient: clients.RDS}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("RDSEventSubscription: %w", err)
		}
		if err := (&LambdaProvisionedConcurrencyReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			LambdaClient: clients.Lambda}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("LambdaProvisionedConcurrency: %w", err)
		}
		if err := (&LambdaEventInvokeConfigReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			LambdaClient: clients.Lambda}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("LambdaEventInvokeConfig: %w", err)
		}
		if err := (&S3AccessPointReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			S3ControlClient: clients.S3Control}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("S3AccessPoint: %w", err)
		}
		if err := (&IAMInstanceProfileReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			IAMClient: clients.IAM}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("IAMInstanceProfile: %w", err)
		}
		return nil
	})
}
