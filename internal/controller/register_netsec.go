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
		if err := (&FirewallRuleGroupReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			NetworkFirewallClient: clients.NetworkFirewall}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("FirewallRuleGroup: %w", err)
		}
		if err := (&FirewallPolicyReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			NetworkFirewallClient: clients.NetworkFirewall}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("FirewallPolicy: %w", err)
		}
		if err := (&FirewallReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			NetworkFirewallClient: clients.NetworkFirewall}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("Firewall: %w", err)
		}
		if err := (&LatticeServiceNetworkReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			LatticeClient: clients.VPCLattice}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("LatticeServiceNetwork: %w", err)
		}
		if err := (&LatticeServiceReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			LatticeClient: clients.VPCLattice}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("LatticeService: %w", err)
		}
		if err := (&LatticeServiceNetworkVpcAssociationReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			LatticeClient: clients.VPCLattice}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("LatticeServiceNetworkVpcAssociation: %w", err)
		}
		if err := (&LatticeServiceNetworkServiceAssociationReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			LatticeClient: clients.VPCLattice}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("LatticeServiceNetworkServiceAssociation: %w", err)
		}
		if err := (&LatticeTargetGroupReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			LatticeClient: clients.VPCLattice}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("LatticeTargetGroup: %w", err)
		}
		if err := (&LatticeListenerReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			LatticeClient: clients.VPCLattice}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("LatticeListener: %w", err)
		}
		if err := (&CustomerGatewayReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			EC2Client: clients.EC2}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("CustomerGateway: %w", err)
		}
		if err := (&VPNGatewayReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			EC2Client: clients.EC2}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("VPNGateway: %w", err)
		}
		if err := (&VPNConnectionReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			EC2Client: clients.EC2}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("VPNConnection: %w", err)
		}
		if err := (&VPNConnectionRouteReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			EC2Client: clients.EC2}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("VPNConnectionRoute: %w", err)
		}
		if err := (&ManagedPrefixListReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			EC2Client: clients.EC2}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("ManagedPrefixList: %w", err)
		}
		if err := (&CapacityReservationReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			EC2Client: clients.EC2}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("CapacityReservation: %w", err)
		}
		return nil
	})
}
