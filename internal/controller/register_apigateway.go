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
		if err := (&APIGatewayV2APIReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			APIGatewayV2Client: clients.APIGatewayV2}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("APIGatewayV2API: %w", err)
		}
		if err := (&APIGatewayV2RouteReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			APIGatewayV2Client: clients.APIGatewayV2}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("APIGatewayV2Route: %w", err)
		}
		if err := (&APIGatewayV2IntegrationReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			APIGatewayV2Client: clients.APIGatewayV2}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("APIGatewayV2Integration: %w", err)
		}
		if err := (&APIGatewayV2StageReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			APIGatewayV2Client: clients.APIGatewayV2}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("APIGatewayV2Stage: %w", err)
		}
		if err := (&APIGatewayV2AuthorizerReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			APIGatewayV2Client: clients.APIGatewayV2}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("APIGatewayV2Authorizer: %w", err)
		}
		if err := (&APIGatewayV2DomainNameReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			APIGatewayV2Client: clients.APIGatewayV2}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("APIGatewayV2DomainName: %w", err)
		}
		if err := (&APIGatewayV2ApiMappingReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			APIGatewayV2Client: clients.APIGatewayV2}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("APIGatewayV2ApiMapping: %w", err)
		}
		if err := (&APIGatewayV2VpcLinkReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			APIGatewayV2Client: clients.APIGatewayV2}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("APIGatewayV2VpcLink: %w", err)
		}
		if err := (&RestAPIReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			APIGatewayClient: clients.APIGateway}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("RestAPI: %w", err)
		}
		if err := (&RestAPIDeploymentReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			APIGatewayClient: clients.APIGateway}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("RestAPIDeployment: %w", err)
		}
		if err := (&RestAPIStageReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			APIGatewayClient: clients.APIGateway}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("RestAPIStage: %w", err)
		}
		return nil
	})
}
