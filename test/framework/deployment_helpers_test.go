/*
Copyright 2024 The Kubernetes Authors.

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

package framework

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_containerHasTerminated(t *testing.T) {
	tests := []struct {
		name          string
		pod           *corev1.Pod
		containerName string
		want          bool
	}{
		{
			name: "pod succeeded — all containers are terminated",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodSucceeded,
				},
			},
			containerName: "any-container",
			want:          true,
		},
		{
			name: "pod failed — all containers are terminated",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodFailed,
				},
			},
			containerName: "any-container",
			want:          true,
		},
		{
			name: "running pod with terminated init container",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					InitContainerStatuses: []corev1.ContainerStatus{
						{
							Name:  "init-downloader",
							State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Completed"}},
						},
					},
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name:  "main",
							State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
						},
					},
				},
			},
			containerName: "init-downloader",
			want:          true,
		},
		{
			name: "terminated regular container in pod with restartPolicy Never",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name:  "sidecar",
							State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Completed"}},
						},
						{
							Name:  "main",
							State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
						},
					},
				},
			},
			containerName: "sidecar",
			want:          true,
		},
		{
			name: "terminated regular container in pod with restartPolicy Always may still restart",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name:  "flaky",
							State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1}},
						},
						{
							Name:  "main",
							State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
						},
					},
				},
			},
			containerName: "flaky",
			want:          false,
		},
		{
			name: "terminated regular container in pod with restartPolicy OnFailure may still restart",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name:  "flaky",
							State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1}},
						},
						{
							Name:  "main",
							State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
						},
					},
				},
			},
			containerName: "flaky",
			want:          false,
		},
		{
			name: "running pod — container still running",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name:  "main",
							State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
						},
					},
				},
			},
			containerName: "main",
			want:          false,
		},
		{
			name: "running pod — container waiting",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name:  "main",
							State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
						},
					},
				},
			},
			containerName: "main",
			want:          false,
		},
		{
			name: "container not found in status",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name:  "other",
							State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
						},
					},
				},
			},
			containerName: "missing",
			want:          false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(containerHasTerminated(tt.pod, tt.containerName)).To(Equal(tt.want))
		})
	}
}

func Test_verifyMetrics(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{
			name: "no panic metric exists",
			data: []byte(`
# HELP controller_runtime_max_concurrent_reconciles Maximum number of concurrent reconciles per controller
# TYPE controller_runtime_max_concurrent_reconciles gauge
controller_runtime_max_concurrent_reconciles{controller="cluster"} 10
controller_runtime_max_concurrent_reconciles{controller="clusterclass"} 10
`),
		},
		{
			name: "no panic occurred",
			data: []byte(`
# HELP controller_runtime_max_concurrent_reconciles Maximum number of concurrent reconciles per controller
# TYPE controller_runtime_max_concurrent_reconciles gauge
controller_runtime_max_concurrent_reconciles{controller="cluster"} 10
controller_runtime_max_concurrent_reconciles{controller="clusterclass"} 10
# HELP controller_runtime_reconcile_panics_total Total number of reconciliation panics per controller
# TYPE controller_runtime_reconcile_panics_total counter
controller_runtime_reconcile_panics_total{controller="cluster"} 0
controller_runtime_reconcile_panics_total{controller="clusterclass"} 0
# HELP controller_runtime_webhook_panics_total Total number of webhook panics
# TYPE controller_runtime_webhook_panics_total counter
controller_runtime_webhook_panics_total 0
`),
		},
		{
			name: "panic occurred in controller",
			data: []byte(`
# HELP controller_runtime_max_concurrent_reconciles Maximum number of concurrent reconciles per controller
# TYPE controller_runtime_max_concurrent_reconciles gauge
controller_runtime_max_concurrent_reconciles{controller="cluster"} 10
controller_runtime_max_concurrent_reconciles{controller="clusterclass"} 10
# HELP controller_runtime_reconcile_panics_total Total number of reconciliation panics per controller
# TYPE controller_runtime_reconcile_panics_total counter
controller_runtime_reconcile_panics_total{controller="cluster"} 1
controller_runtime_reconcile_panics_total{controller="clusterclass"} 0
# HELP controller_runtime_webhook_panics_total Total number of webhook panics
# TYPE controller_runtime_webhook_panics_total counter
controller_runtime_webhook_panics_total 0
`),
			wantErr: "panics occurred in Pod default/pod1: 1 panics occurred in \"cluster\" controller (check logs for more details)",
		},
		{
			name: "panic occurred in webhooks",
			data: []byte(`
# HELP controller_runtime_max_concurrent_reconciles Maximum number of concurrent reconciles per controller
# TYPE controller_runtime_max_concurrent_reconciles gauge
controller_runtime_max_concurrent_reconciles{controller="cluster"} 10
controller_runtime_max_concurrent_reconciles{controller="clusterclass"} 10
# HELP controller_runtime_reconcile_panics_total Total number of reconciliation panics per controller
# TYPE controller_runtime_reconcile_panics_total counter
controller_runtime_reconcile_panics_total{controller="cluster"} 0
controller_runtime_reconcile_panics_total{controller="clusterclass"} 0
# HELP controller_runtime_webhook_panics_total Total number of webhook panics
# TYPE controller_runtime_webhook_panics_total counter
controller_runtime_webhook_panics_total 1
# HELP controller_runtime_conversion_webhook_panics_total Total number of conversion webhook panics
# TYPE controller_runtime_conversion_webhook_panics_total counter
controller_runtime_conversion_webhook_panics_total 0
`),
			wantErr: "panics occurred in Pod default/pod1: 1 panics occurred in webhooks (check logs for more details)",
		},
		{
			name: "panics occurred in conversion webhooks",
			data: []byte(`
# HELP controller_runtime_max_concurrent_reconciles Maximum number of concurrent reconciles per controller
# TYPE controller_runtime_max_concurrent_reconciles gauge
controller_runtime_max_concurrent_reconciles{controller="cluster"} 10
controller_runtime_max_concurrent_reconciles{controller="clusterclass"} 10
# HELP controller_runtime_reconcile_panics_total Total number of reconciliation panics per controller
# TYPE controller_runtime_reconcile_panics_total counter
controller_runtime_reconcile_panics_total{controller="cluster"} 0
controller_runtime_reconcile_panics_total{controller="clusterclass"} 0
# HELP controller_runtime_webhook_panics_total Total number of webhook panics
# TYPE controller_runtime_webhook_panics_total counter
controller_runtime_webhook_panics_total 0
# HELP controller_runtime_conversion_webhook_panics_total Total number of conversion webhook panics
# TYPE controller_runtime_conversion_webhook_panics_total counter
controller_runtime_conversion_webhook_panics_total 2
`),
			wantErr: "panics occurred in Pod default/pod1: 2 panics occurred in conversion webhooks (check logs for more details)",
		},
		{
			name: "multiple informers with the same GVR",
			data: []byte(`
informer_store_resource_version{group="bootstrap.cluster.x-k8s.io", name="cluster-api-kubeadm-control-plane-manager", resource="kubeadmconfigtemplates", version="v1beta2"} 2439
informer_store_resource_version{group="bootstrap.cluster.x-k8s.io", name="cluster-api-kubeadm-control-plane-manager-dynamic-cache", resource="kubeadmconfigtemplates", version="v1beta2"} 2439
`),
			wantErr: "there are 2 informers for GVR bootstrap.cluster.x-k8s.io/v1beta2, Resource=kubeadmconfigtemplates (names: [cluster-api-kubeadm-control-plane-manager cluster-api-kubeadm-control-plane-manager-dynamic-cache])",
		},
		{
			name: "informers with different GVR",
			data: []byte(`
informer_store_resource_version{group="controlplane.cluster.x-k8s.io", name="cluster-api-kubeadm-control-plane-manager", resource="kubeadmcontrolplanes", version="v1beta2"} 2439
informer_store_resource_version{group="bootstrap.cluster.x-k8s.io", name="cluster-api-kubeadm-control-plane-manager-dynamic-cache", resource="kubeadmconfigs", version="v1beta2"} 2439
`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := verifyMetrics(tt.data, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "pod1"}})
			if tt.wantErr == "" {
				g.Expect(err).ToNot(HaveOccurred())
			} else {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.wantErr))
			}
		})
	}
}
