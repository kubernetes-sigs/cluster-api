/*
Copyright 2022 The Kubernetes Authors.

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

	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/internal/webhooks"
)

// KubeadmConfig implements a validation and defaulting webhook for KubeadmConfig.
type KubeadmConfig struct {
	Client client.Reader
}

// SetupWebhookWithManager sets up KubeadmConfig webhooks.
func (v *KubeadmConfig) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.KubeadmConfig{
		Client: v.Client,
	}).SetupWebhookWithManager(mgr)
}

// KubeadmConfigTemplate implements a validation and defaulting webhook for KubeadmConfigTemplate.
type KubeadmConfigTemplate struct {
	Client client.Reader
}

// SetupWebhookWithManager sets up KubeadmConfigTemplate webhooks.
func (v *KubeadmConfigTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.KubeadmConfigTemplate{
		Client: v.Client,
	}).SetupWebhookWithManager(mgr)
}
