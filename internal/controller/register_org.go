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

// Registers the organization-tier family: AWS Organizations, IAM Identity
// Center (SSO), RAM, Service Catalog, and Control Tower. These kinds only
// reconcile successfully from the organization's management or delegated
// administrator account; elsewhere the CRs simply report Ready=False.
func init() {
	RegisterSetup(func(mgr ctrl.Manager, clients *awsclient.Clients) error {
		if err := (&OrganizationsOUReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			OrganizationsClient: clients.Organizations}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("OrganizationsOU: %w", err)
		}
		if err := (&OrganizationsPolicyReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			OrganizationsClient: clients.Organizations}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("OrganizationsPolicy: %w", err)
		}
		if err := (&OrganizationsPolicyAttachmentReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			OrganizationsClient: clients.Organizations}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("OrganizationsPolicyAttachment: %w", err)
		}
		if err := (&OrganizationsAccountReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			OrganizationsClient: clients.Organizations}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("OrganizationsAccount: %w", err)
		}
		if err := (&PermissionSetReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			SSOAdminClient: clients.SSOAdmin}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("PermissionSet: %w", err)
		}
		if err := (&SSOAssignmentReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			SSOAdminClient: clients.SSOAdmin}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("SSOAssignment: %w", err)
		}
		if err := (&ResourceShareReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			RAMClient: clients.RAM}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("ResourceShare: %w", err)
		}
		if err := (&SCPortfolioReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			ServiceCatalogClient: clients.ServiceCatalog}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("SCPortfolio: %w", err)
		}
		if err := (&SCProductReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			ServiceCatalogClient: clients.ServiceCatalog}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("SCProduct: %w", err)
		}
		if err := (&SCPortfolioProductAssociationReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			ServiceCatalogClient: clients.ServiceCatalog}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("SCPortfolioProductAssociation: %w", err)
		}
		if err := (&CTEnabledControlReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			ControlTowerClient: clients.ControlTower}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("CTEnabledControl: %w", err)
		}
		return nil
	})
}
