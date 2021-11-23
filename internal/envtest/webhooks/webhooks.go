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

package webhooks

import (
	"k8s.io/klog/v2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	kcpv1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	addonv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/webhooks"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetupAllWebhooksWithManager sets up all webhooks.
func SetupAllWebhooksWithManager(mgr ctrl.Manager) {
	SetupCoreWebhooksWithManager(mgr)
	SetupCABPKWebhooksWithManager(mgr)
	SetupKCPWebhooksWithManager(mgr)
}

// SetupCoreWebhooksWithManager sets up webhooks of the core provider.
func SetupCoreWebhooksWithManager(mgr ctrl.Manager) {
	if err := (&webhooks.Cluster{Client: mgr.GetClient()}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
	if err := (&webhooks.ClusterClass{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
	if err := (&clusterv1.Machine{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
	if err := (&clusterv1.MachineHealthCheck{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
	if err := (&clusterv1.Machine{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
	if err := (&clusterv1.MachineSet{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
	if err := (&clusterv1.MachineDeployment{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
	if err := (&addonv1.ClusterResourceSet{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook for crs: %+v", err)
	}
	if err := (&expv1.MachinePool{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook for machinepool: %+v", err)
	}
}

// SetupCABPKWebhooksWithManager sets up webhooks of the CABPK provider.
func SetupCABPKWebhooksWithManager(mgr ctrl.Manager) {
	if err := (&bootstrapv1.KubeadmConfig{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
	if err := (&bootstrapv1.KubeadmConfigTemplate{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
	if err := (&bootstrapv1.KubeadmConfigTemplateList{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
}

// SetupKCPWebhooksWithManager sets up webhooks of the KCP provider.
func SetupKCPWebhooksWithManager(mgr ctrl.Manager) {
	if err := (&kcpv1.KubeadmControlPlane{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
}
