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
		if err := (&GlueDatabaseReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			GlueClient: clients.Glue}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("GlueDatabase: %w", err)
		}
		if err := (&GlueCrawlerReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			GlueClient: clients.Glue}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("GlueCrawler: %w", err)
		}
		if err := (&GlueJobReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			GlueClient: clients.Glue}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("GlueJob: %w", err)
		}
		if err := (&GlueTriggerReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			GlueClient: clients.Glue}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("GlueTrigger: %w", err)
		}
		if err := (&GlueConnectionReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			GlueClient: clients.Glue}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("GlueConnection: %w", err)
		}
		if err := (&AthenaWorkGroupReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			AthenaClient: clients.Athena}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("AthenaWorkGroup: %w", err)
		}
		if err := (&AthenaDataCatalogReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			AthenaClient: clients.Athena}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("AthenaDataCatalog: %w", err)
		}
		if err := (&AthenaNamedQueryReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			AthenaClient: clients.Athena}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("AthenaNamedQuery: %w", err)
		}
		if err := (&RedshiftClusterReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			RedshiftClient: clients.Redshift}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("RedshiftCluster: %w", err)
		}
		if err := (&RedshiftSubnetGroupReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			RedshiftClient: clients.Redshift}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("RedshiftSubnetGroup: %w", err)
		}
		if err := (&RedshiftParameterGroupReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			RedshiftClient: clients.Redshift}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("RedshiftParameterGroup: %w", err)
		}
		return nil
	})
}
