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
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetupFunc wires one controller family into the manager. New families
// append themselves via RegisterSetup from an init() in their own
// register_<family>.go file, so adding a family never touches cmd/main.go.
type SetupFunc func(mgr ctrl.Manager, clients *awsclient.Clients) error

var setupFuncs []SetupFunc

// RegisterSetup queues a family setup function; called from init().
func RegisterSetup(f SetupFunc) {
	setupFuncs = append(setupFuncs, f)
}

// SetupRegistered runs every queued family setup against the manager.
// Called from cmd/main.go after the long-standing inline registrations.
func SetupRegistered(mgr ctrl.Manager, clients *awsclient.Clients) error {
	for _, f := range setupFuncs {
		if err := f(mgr, clients); err != nil {
			return err
		}
	}
	return nil
}
