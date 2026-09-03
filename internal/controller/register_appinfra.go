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
		if err := (&MQBrokerReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			MQClient: clients.MQ}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("MQBroker: %w", err)
		}
		if err := (&MQConfigurationReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			MQClient: clients.MQ}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("MQConfiguration: %w", err)
		}
		if err := (&BatchComputeEnvironmentReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			BatchClient: clients.Batch}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("BatchComputeEnvironment: %w", err)
		}
		if err := (&BatchJobQueueReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			BatchClient: clients.Batch}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("BatchJobQueue: %w", err)
		}
		if err := (&BatchJobDefinitionReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			BatchClient: clients.Batch}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("BatchJobDefinition: %w", err)
		}
		if err := (&AppRunnerServiceReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			AppRunnerClient: clients.AppRunner}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("AppRunnerService: %w", err)
		}
		if err := (&AppRunnerAutoScalingReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			AppRunnerClient: clients.AppRunner}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("AppRunnerAutoScaling: %w", err)
		}
		if err := (&PrivateCAReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			ACMPCAClient: clients.ACMPCA}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("PrivateCA: %w", err)
		}
		if err := (&CloudMapNamespaceReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			ServiceDiscoveryClient: clients.ServiceDiscovery}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("CloudMapNamespace: %w", err)
		}
		if err := (&CloudMapServiceReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			ServiceDiscoveryClient: clients.ServiceDiscovery}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("CloudMapService: %w", err)
		}
		return nil
	})
}
