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

// DockerCluster implements a validating and defaulting webhook for DockerCluster.
type DockerCluster struct{}

// SetupWebhookWithManager sets up ClusterResourceSet webhooks.
func (webhook *DockerCluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.DockerCluster{}).SetupWebhookWithManager(mgr)
}

// DockerClusterTemplate implements a validating webhook for DockerClusterTemplate.
type DockerClusterTemplate struct{}

// SetupWebhookWithManager sets up ClusterResourceSet webhooks.
func (webhook *DockerClusterTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.DockerClusterTemplate{}).SetupWebhookWithManager(mgr)
}

// DockerMachineTemplate implements a validating webhook for DockerMachineTemplate.
type DockerMachineTemplate struct{}

// SetupWebhookWithManager sets up ClusterResourceSet webhooks.
func (webhook *DockerMachineTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.DockerMachineTemplate{}).SetupWebhookWithManager(mgr)
}
