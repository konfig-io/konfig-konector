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
		if err := (&AWSProviderReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			STSClient: clients.STS, Resolver: providerResolver}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("AWSProvider: %w", err)
		}
		if err := (&ResourceShareInvitationReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			RAMClient: clients.RAM}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("ResourceShareInvitation: %w", err)
		}
		if err := (&HostedZoneVPCAssociationReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			Route53Client: clients.Route53}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("HostedZoneVPCAssociation: %w", err)
		}
		if err := (&VPCEndpointServiceReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			EC2Client: clients.EC2}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("VPCEndpointService: %w", err)
		}
		return nil
	})
}
