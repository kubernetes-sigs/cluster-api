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
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

func SetupWebhooksWithManager(mgr ctrl.Manager) error {
	// NOTE: ClusterClass and managed topologies are behind ClusterTopology feature gate flag; the webhook
	// is going to prevent creating or updating new objects in case the feature flag is disabled.
	if err := (&ClusterClass{}).SetupWebhookWithManager(mgr); err != nil {
		return errors.Wrapf(err, "failed to create ClusterClass webhook")
	}

	// NOTE: ClusterClass and managed topologies are behind ClusterTopology feature gate flag; the webhook
	// is going to prevent usage of Cluster.Topology in case the feature flag is disabled.
	if err := (&Cluster{Client: mgr.GetClient()}).SetupWebhookWithManager(mgr); err != nil {
		return errors.Wrapf(err, "failed to create Cluster webhook")
	}

	return nil
}
