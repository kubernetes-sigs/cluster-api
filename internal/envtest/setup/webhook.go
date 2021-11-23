/*
Copyright 2020 The Kubernetes Authors.

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

package setup

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	kubeadmbootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	kcpv1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/webhooks"
	ctrl "sigs.k8s.io/controller-runtime"
)

// AllWebhooksWithManager sets up all webhooks.
func AllWebhooksWithManager(mgr ctrl.Manager) {
	if err := clusterv1.SetupWebhooksWithManager(mgr); err != nil {
		panic(fmt.Sprintf("failed to set up webhooks: %v", err))
	}
	// Set minNodeStartupTimeout for Test, so it does not need to be at least 30s
	clusterv1.SetMinNodeStartupTimeout(metav1.Duration{Duration: 1 * time.Millisecond})
	if err := webhooks.SetupWebhooksWithManager(mgr); err != nil {
		panic(fmt.Sprintf("failed to set up webhooks: %v", err))
	}
	if err := expv1.SetupWebhooksWithManager(mgr); err != nil {
		panic(fmt.Sprintf("failed to set up webhooks: %v", err))
	}
	if err := addonsv1.SetupWebhooksWithManager(mgr); err != nil {
		panic(fmt.Sprintf("failed to set up webhooks: %v", err))
	}
	if err := kubeadmbootstrapv1.SetupWebhooksWithManager(mgr); err != nil {
		panic(fmt.Sprintf("failed to set up webhooks: %v", err))
	}
	if err := kcpv1.SetupWebhooksWithManager(mgr); err != nil {
		panic(fmt.Sprintf("failed to set up webhooks: %v", err))
	}
}
