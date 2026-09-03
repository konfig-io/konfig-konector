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

	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	ctrl "sigs.k8s.io/controller-runtime"
)

func init() {
	RegisterSetup(func(mgr ctrl.Manager, clients *awsclient.Clients) error {
		if err := (&TrailReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			CloudTrailClient: clients.CloudTrail}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("Trail: %w", err)
		}
		if err := (&ConfigRecorderReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			ConfigClient: clients.ConfigService}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("ConfigRecorder: %w", err)
		}
		if err := (&ConfigDeliveryChannelReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			ConfigClient: clients.ConfigService}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("ConfigDeliveryChannel: %w", err)
		}
		if err := (&ConfigRuleReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			ConfigClient: clients.ConfigService}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("ConfigRule: %w", err)
		}
		if err := (&BackupVaultReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			BackupClient: clients.Backup}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("BackupVault: %w", err)
		}
		if err := (&BackupPlanReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			BackupClient: clients.Backup}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("BackupPlan: %w", err)
		}
		if err := (&BackupSelectionReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			BackupClient: clients.Backup}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("BackupSelection: %w", err)
		}
		if err := (&GuardDutyDetectorReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			GuardDutyClient: clients.GuardDuty}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("GuardDutyDetector: %w", err)
		}
		if err := (&SecurityHubAccountReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			SecurityHubClient: clients.SecurityHub}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("SecurityHubAccount: %w", err)
		}
		if err := (&SecurityHubStandardReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			SecurityHubClient: clients.SecurityHub}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("SecurityHubStandard: %w", err)
		}
		if err := (&InspectorEnablerReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
			InspectorClient: clients.Inspector2}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("InspectorEnabler: %w", err)
		}
		return nil
	})
}
