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

	"sigs.k8s.io/cluster-api/exp/ipam/internal/webhooks"
)

// IPAddress implements a validating and defaulting webhook for IPAddress.
type IPAddress struct {
	Client client.Reader
}

// SetupWebhookWithManager sets up IPAddress webhooks.
func (webhook *IPAddress) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.IPAddress{
		Client: webhook.Client,
	}).SetupWebhookWithManager(mgr)
}

// IPAddressClaim implements a validating and defaulting webhook for IPAddressClaim.
type IPAddressClaim struct {
}

// SetupWebhookWithManager sets up IPAddressClaim webhooks.
func (webhook *IPAddressClaim) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return (&webhooks.IPAddressClaim{}).SetupWebhookWithManager(mgr)
}
