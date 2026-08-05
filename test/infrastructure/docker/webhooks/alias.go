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

package webhooks

import (
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/webhooks"
)

// DevCluster implements a validating and defaulting webhook for DevCluster.
type DevCluster struct{}

// SetupWebhookWithManager sets up DevCluster webhooks.
func (webhook *DevCluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.DevCluster{}).SetupWebhookWithManager(mgr)
}

// DevClusterTemplate implements a validating and defaulting webhook for DevClusterTemplate.
type DevClusterTemplate struct{}

// SetupWebhookWithManager sets up DevClusterTemplate webhooks.
func (webhook *DevClusterTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.DevClusterTemplate{}).SetupWebhookWithManager(mgr)
}

// DevMachine implements a validating and defaulting webhook for DevMachine.
type DevMachine struct{}

// SetupWebhookWithManager sets up DevMachine webhooks.
func (webhook *DevMachine) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.DevMachine{}).SetupWebhookWithManager(mgr)
}

// DevMachineTemplate implements a validating webhook for DevMachineTemplate.
type DevMachineTemplate struct{}

// SetupWebhookWithManager sets up DevMachineTemplate webhooks.
func (webhook *DevMachineTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.DevMachineTemplate{}).SetupWebhookWithManager(mgr)
}
