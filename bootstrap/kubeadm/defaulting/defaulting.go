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

// Package defaulting contains defaulting code that is used in CABPK and KCP.
package defaulting

import (
	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
)

// ApplyPreviousKubeadmConfigDefaults defaults a KubeadmConfig with default values we used in the past.
// This is done in multiple places (webhooks and KCP controller) to ensure no rollouts are triggered now that
// we removed this defaulting from webhooks.
func ApplyPreviousKubeadmConfigDefaults(c *bootstrapv1.KubeadmConfigSpec) {
	if c.Format == "" {
		c.Format = bootstrapv1.CloudConfig
	}
	if c.InitConfiguration.NodeRegistration.ImagePullPolicy == "" {
		c.InitConfiguration.NodeRegistration.ImagePullPolicy = "IfNotPresent"
	}
	if c.JoinConfiguration.NodeRegistration.ImagePullPolicy == "" {
		c.JoinConfiguration.NodeRegistration.ImagePullPolicy = "IfNotPresent"
	}
	if c.JoinConfiguration.Discovery.File.KubeConfig.User.Exec.IsDefined() {
		if c.JoinConfiguration.Discovery.File.KubeConfig.User.Exec.APIVersion == "" {
			c.JoinConfiguration.Discovery.File.KubeConfig.User.Exec.APIVersion = "client.authentication.k8s.io/v1"
		}
	}
}
