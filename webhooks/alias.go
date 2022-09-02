/*
Copyright 2021 The Kubernetes Authors.

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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/internal/webhooks"
)

// Cluster implements a validating and defaulting webhook for Cluster.
type Cluster struct {
	Client client.Reader
}

// SetupWebhookWithManager sets up Cluster webhooks.
func (webhook *Cluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.Cluster{
		Client: webhook.Client,
	}).SetupWebhookWithManager(mgr)
}

// ClusterClass implements a validation and defaulting webhook for ClusterClass.
type ClusterClass struct {
	Client client.Reader
}

// SetupWebhookWithManager sets up ClusterClass webhooks.
func (webhook *ClusterClass) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.ClusterClass{
		Client: webhook.Client,
	}).SetupWebhookWithManager(mgr)
}

// Machine implements a validation and defaulting webhook for Machine.
type Machine struct {
	Client client.Reader
}

// SetupWebhookWithManager sets up Machine webhooks.
func (webhook *Machine) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.Machine{
		Client: webhook.Client,
	}).SetupWebhookWithManager(mgr)
}

// MachineDeployment implements a validation and defaulting webhook for MachineDeployment.
type MachineDeployment struct {
	Client client.Reader
}

// SetupWebhookWithManager sets up MachineDeployment webhooks.
func (webhook *MachineDeployment) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.MachineDeployment{
		Client: webhook.Client,
	}).SetupWebhookWithManager(mgr)
}
