/*
Copyright 2023 The Kubernetes Authors.

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

// Package webhooks implements inmemory infrastructure webhooks.
package webhooks

import (
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/webhooks"
)

// InMemoryCluster implements a validating and defaulting webhook for InMemoryCluster.
type InMemoryCluster struct{}

// SetupWebhookWithManager sets up InMemoryCluster webhooks.
func (webhook *InMemoryCluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.InMemoryCluster{}).SetupWebhookWithManager(mgr)
}

// InMemoryClusterTemplate implements a validating and defaulting webhook for InMemoryClusterTemplate.
type InMemoryClusterTemplate struct{}

// SetupWebhookWithManager sets up InMemoryClusterTemplate webhooks.
func (webhook *InMemoryClusterTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.InMemoryClusterTemplate{}).SetupWebhookWithManager(mgr)
}

// InMemoryMachine implements a validating and defaulting webhook for InMemoryMachine.
type InMemoryMachine struct{}

// SetupWebhookWithManager sets up InMemoryMachine webhooks.
func (webhook *InMemoryMachine) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.InMemoryMachine{}).SetupWebhookWithManager(mgr)
}

// InMemoryMachineTemplate implements a validating and defaulting webhook for InMemoryMachineTemplate.
type InMemoryMachineTemplate struct{}

// SetupWebhookWithManager sets up InMemoryMachineTemplate webhooks.
func (webhook *InMemoryMachineTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.InMemoryMachineTemplate{}).SetupWebhookWithManager(mgr)
}
