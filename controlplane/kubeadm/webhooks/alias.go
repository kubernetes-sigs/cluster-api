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

	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/webhooks"
)

// ScaleValidator validates KCP for replicas.
type ScaleValidator struct {
	Client client.Reader
}

// SetupWebhookWithManager sets up Cluster webhooks.
func (v *ScaleValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.ScaleValidator{
		Client: v.Client,
	}).SetupWebhookWithManager(mgr)
}
