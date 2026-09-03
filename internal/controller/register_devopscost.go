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
		if err := (&CodeArtifactDomainReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			CodeArtifactClient: clients.CodeArtifact}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("CodeArtifactDomain: %w", err)
		}
		if err := (&CodeArtifactRepositoryReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			CodeArtifactClient: clients.CodeArtifact}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("CodeArtifactRepository: %w", err)
		}
		if err := (&XRayGroupReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			XRayClient: clients.XRay}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("XRayGroup: %w", err)
		}
		if err := (&XRaySamplingRuleReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			XRayClient: clients.XRay}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("XRaySamplingRule: %w", err)
		}
		if err := (&PrometheusWorkspaceReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			AMPClient: clients.AMP}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("PrometheusWorkspace: %w", err)
		}
		if err := (&PrometheusRuleGroupsNamespaceReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			AMPClient: clients.AMP}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("PrometheusRuleGroupsNamespace: %w", err)
		}
		if err := (&PrometheusAlertManagerDefinitionReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			AMPClient: clients.AMP}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("PrometheusAlertManagerDefinition: %w", err)
		}
		if err := (&GrafanaWorkspaceReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			GrafanaClient: clients.Grafana}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("GrafanaWorkspace: %w", err)
		}
		if err := (&BudgetReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			BudgetsClient: clients.Budgets}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("Budget: %w", err)
		}
		if err := (&CostAnomalyMonitorReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			CostExplorerClient: clients.CostExplorer}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("CostAnomalyMonitor: %w", err)
		}
		if err := (&CostAnomalySubscriptionReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			CostExplorerClient: clients.CostExplorer}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("CostAnomalySubscription: %w", err)
		}
		return nil
	})
}
