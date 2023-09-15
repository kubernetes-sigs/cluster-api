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

	"sigs.k8s.io/cluster-api/exp/addons/internal/webhooks"
)

// ClusterResourceSet implements a validating and defaulting webhook for ClusterResourceSet.
type ClusterResourceSet struct{}

// SetupWebhookWithManager sets up ClusterResourceSet webhooks.
func (webhook *ClusterResourceSet) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.ClusterResourceSet{}).SetupWebhookWithManager(mgr)
}

// ClusterResourceSetBinding implements a validating webhook for ClusterResourceSetBinding.
type ClusterResourceSetBinding struct{}

// SetupWebhookWithManager sets up ClusterResourceSet webhooks.
func (webhook *ClusterResourceSetBinding) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.ClusterResourceSetBinding{}).SetupWebhookWithManager(mgr)
}
