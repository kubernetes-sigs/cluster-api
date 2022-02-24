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

package internal

import (
	"context"
	"errors"
	"testing"

	"github.com/blang/semver"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/yaml"
)

func TestGetControlPlaneNodes(t *testing.T) {
	tests := []struct {
		name          string
		nodes         []corev1.Node
		expectedNodes []string
	}{
		{
			name: "Return control plane nodes",
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "control-plane-node-with-old-label",
						Labels: map[string]string{
							labelNodeRoleOldControlPlane: "",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "control-plane-node-with-both-labels",
						Labels: map[string]string{
							labelNodeRoleOldControlPlane: "",
							labelNodeRoleControlPlane:    "",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "control-plane-node-with-new-label",
						Labels: map[string]string{
							labelNodeRoleControlPlane: "",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "worker-node",
						Labels: map[string]string{},
					},
				},
			},
			expectedNodes: []string{
				"control-plane-node-with-both-labels",
				"control-plane-node-with-old-label",
				"control-plane-node-with-new-label",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			objs := []client.Object{}
			for i := range tt.nodes {
				objs = append(objs, &tt.nodes[i])
			}
			fakeClient := fake.NewClientBuilder().WithObjects(objs...).Build()

			w := &Workload{
				Client: fakeClient,
			}
			nodes, err := w.getControlPlaneNodes(ctx)
			g.Expect(err).ToNot(HaveOccurred())
			var actualNodes []string
			for _, n := range nodes.Items {
				actualNodes = append(actualNodes, n.Name)
			}
			g.Expect(actualNodes).To(Equal(tt.expectedNodes))
		})
	}
}

func TestUpdateKubeProxyImageInfo(t *testing.T) {
	tests := []struct {
		name        string
		ds          appsv1.DaemonSet
		expectErr   bool
		expectImage string
		clientGet   map[string]interface{}
		patchErr    error
		KCP         *controlplanev1.KubeadmControlPlane
	}{
		{
			name:        "succeeds if patch correctly",
			ds:          newKubeProxyDS(),
			expectErr:   false,
			expectImage: "k8s.gcr.io/kube-proxy:v1.16.3",
			KCP:         &controlplanev1.KubeadmControlPlane{Spec: controlplanev1.KubeadmControlPlaneSpec{Version: "v1.16.3"}},
		},
		{
			name:        "returns error if image in kube-proxy ds was in digest format",
			ds:          newKubeProxyDSWithImage("k8s.gcr.io/kube-proxy@sha256:47bfd"),
			expectErr:   true,
			expectImage: "k8s.gcr.io/kube-proxy@sha256:47bfd",
			KCP:         &controlplanev1.KubeadmControlPlane{Spec: controlplanev1.KubeadmControlPlaneSpec{Version: "v1.16.3"}},
		},
		{
			name:        "expects OCI compatible format of tag",
			ds:          newKubeProxyDS(),
			expectErr:   false,
			expectImage: "k8s.gcr.io/kube-proxy:v1.16.3_build1",
			KCP:         &controlplanev1.KubeadmControlPlane{Spec: controlplanev1.KubeadmControlPlaneSpec{Version: "v1.16.3+build1"}},
		},
		{
			name:      "returns error if image in kube-proxy ds was in wrong format",
			ds:        newKubeProxyDSWithImage(""),
			expectErr: true,
			KCP:       &controlplanev1.KubeadmControlPlane{Spec: controlplanev1.KubeadmControlPlaneSpec{Version: "v1.16.3"}},
		},
		{
			name:        "updates image repository if one has been set on the control plane",
			ds:          newKubeProxyDS(),
			expectErr:   false,
			expectImage: "foo.bar.example/baz/qux/kube-proxy:v1.16.3",
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.16.3",
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							ImageRepository: "foo.bar.example/baz/qux",
						},
					},
				}},
		},
		{
			name:        "does not update image repository if it is blank",
			ds:          newKubeProxyDS(),
			expectErr:   false,
			expectImage: "k8s.gcr.io/kube-proxy:v1.16.3",
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.16.3",
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							ImageRepository: "",
						},
					},
				}},
		},
		{
			name:      "returns error if image repository is invalid",
			ds:        newKubeProxyDS(),
			expectErr: true,
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.16.3",
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							ImageRepository: "%%%",
						},
					},
				}},
		},
		{
			name:        "does not update image repository when no kube-proxy update is requested",
			ds:          newKubeProxyDSWithImage(""), // Using the same image name that would otherwise lead to an error
			expectErr:   false,
			expectImage: "",
			KCP: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						controlplanev1.SkipKubeProxyAnnotation: "",
					},
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.16.3",
				}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := NewWithT(t)

			objects := []client.Object{
				&tt.ds,
			}
			fakeClient := fake.NewClientBuilder().WithObjects(objects...).Build()
			w := &Workload{
				Client: fakeClient,
			}
			err := w.UpdateKubeProxyImageInfo(ctx, tt.KCP)
			if tt.expectErr {
				gs.Expect(err).To(HaveOccurred())
			} else {
				gs.Expect(err).NotTo(HaveOccurred())
			}

			proxyImage, err := getProxyImageInfo(ctx, w.Client)
			gs.Expect(err).NotTo(HaveOccurred())
			if tt.expectImage != "" {
				gs.Expect(proxyImage).To(Equal(tt.expectImage))
			}
		})
	}
}

func TestRemoveMachineFromKubeadmConfigMap(t *testing.T) {
	machine := &clusterv1.Machine{
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Name: "ip-10-0-0-1.ec2.internal",
			},
		},
	}
	kubeadmConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeadmConfigKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			clusterStatusKey: yaml.Raw(`
				apiEndpoints:
				  ip-10-0-0-1.ec2.internal:
				    advertiseAddress: 10.0.0.1
				    bindPort: 6443
				  ip-10-0-0-2.ec2.internal:
				    advertiseAddress: 10.0.0.2
				    bindPort: 6443
				apiVersion: kubeadm.k8s.io/v1beta2
				kind: ClusterStatus
				`),
		},
		BinaryData: map[string][]byte{
			"": nil,
		},
	}
	kubeadmConfigWithoutClusterStatus := kubeadmConfig.DeepCopy()
	delete(kubeadmConfigWithoutClusterStatus.Data, clusterStatusKey)

	tests := []struct {
		name              string
		kubernetesVersion semver.Version
		machine           *clusterv1.Machine
		objs              []client.Object
		expectErr         bool
		expectedEndpoints string
	}{
		{
			name:      "does not panic if machine is nil",
			expectErr: false,
		},
		{
			name: "does not panic if machine noderef is nil",
			machine: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{
					NodeRef: nil,
				},
			},
			expectErr: false,
		},
		{
			name:      "returns error if unable to find kubeadm-config",
			machine:   machine,
			expectErr: true,
		},
		{
			name:              "returns error if unable to find kubeadm-config for Kubernetes version < 1.22.0",
			kubernetesVersion: semver.MustParse("1.19.1"),
			machine:           machine,
			objs:              []client.Object{kubeadmConfigWithoutClusterStatus},
			expectErr:         true,
		},
		{
			name:              "returns error if unable to remove api endpoint for Kubernetes version < 1.22.0",
			kubernetesVersion: semver.MustParse("1.19.1"), // Kubernetes version < 1.22.0 has ClusterStatus
			machine:           machine,
			objs:              []client.Object{kubeadmConfigWithoutClusterStatus},
			expectErr:         true,
		},
		{
			name:              "removes the machine node ref from kubeadm config for Kubernetes version < 1.22.0",
			kubernetesVersion: semver.MustParse("1.19.1"), // Kubernetes version < 1.22.0 has ClusterStatus
			machine:           machine,
			objs:              []client.Object{kubeadmConfig},
			expectErr:         false,
			expectedEndpoints: yaml.Raw(`
				apiEndpoints:
				  ip-10-0-0-2.ec2.internal:
				    advertiseAddress: 10.0.0.2
				    bindPort: 6443
				apiVersion: kubeadm.k8s.io/v1beta2
				kind: ClusterStatus
				`),
		},
		{
			name:              "no op for Kubernetes version >= 1.22.0",
			kubernetesVersion: minKubernetesVersionWithoutClusterStatus, // Kubernetes version >= 1.22.0 should not manage ClusterStatus
			machine:           machine,
			objs:              []client.Object{kubeadmConfigWithoutClusterStatus},
			expectErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(tt.objs...).Build()
			w := &Workload{
				Client: fakeClient,
			}
			err := w.RemoveMachineFromKubeadmConfigMap(ctx, tt.machine, tt.kubernetesVersion)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			if tt.expectedEndpoints != "" {
				var actualConfig corev1.ConfigMap
				g.Expect(w.Client.Get(
					ctx,
					client.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem},
					&actualConfig,
				)).To(Succeed())
				g.Expect(actualConfig.Data[clusterStatusKey]).To(Equal(tt.expectedEndpoints), cmp.Diff(tt.expectedEndpoints, actualConfig.Data[clusterStatusKey]))
			}
		})
	}
}

func TestUpdateKubeletConfigMap(t *testing.T) {
	tests := []struct {
		name               string
		version            semver.Version
		objs               []client.Object
		expectErr          bool
		expectCgroupDriver string
		expectNewConfigMap bool
	}{
		{
			name:    "create new config map for 1.19 --> 1.20 (anything < 1.24); config map for previous version is copied",
			version: semver.Version{Major: 1, Minor: 20},
			objs: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "kubelet-config-1.19",
					Namespace:       metav1.NamespaceSystem,
					ResourceVersion: "some-resource-version",
				},
				Data: map[string]string{
					kubeletConfigKey: yaml.Raw(`
						apiVersion: kubelet.config.k8s.io/v1beta1
						kind: KubeletConfiguration
						foo: bar
						`),
				},
			}},
			expectNewConfigMap: true,
		},
		{
			name:    "create new config map 1.23 --> 1.24; config map for previous version is copied",
			version: semver.Version{Major: 1, Minor: 24},
			objs: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "kubelet-config-1.23",
					Namespace:       metav1.NamespaceSystem,
					ResourceVersion: "some-resource-version",
				},
				Data: map[string]string{
					kubeletConfigKey: yaml.Raw(`
						apiVersion: kubelet.config.k8s.io/v1beta1
						kind: KubeletConfiguration
						foo: bar
						`),
				},
			}},
			expectNewConfigMap: true,
		},
		{
			name:    "create new config map >=1.24 --> next; no op",
			version: semver.Version{Major: 1, Minor: 25},
			objs: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "kubelet-config",
					Namespace:       metav1.NamespaceSystem,
					ResourceVersion: "some-resource-version",
				},
				Data: map[string]string{
					kubeletConfigKey: yaml.Raw(`
						apiVersion: kubelet.config.k8s.io/v1beta1
						kind: KubeletConfiguration
						foo: bar
						`),
				},
			}},
		},
		{
			name:    "1.20 --> 1.21 sets the cgroupDriver if empty",
			version: semver.Version{Major: 1, Minor: 21},
			objs: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "kubelet-config-1.20",
					Namespace:       metav1.NamespaceSystem,
					ResourceVersion: "some-resource-version",
				},
				Data: map[string]string{
					kubeletConfigKey: yaml.Raw(`
						apiVersion: kubelet.config.k8s.io/v1beta1
						kind: KubeletConfiguration
						foo: bar
						`),
				},
			}},
			expectCgroupDriver: "systemd",
			expectNewConfigMap: true,
		},
		{
			name:    "1.20 --> 1.21 preserves cgroupDriver if already set",
			version: semver.Version{Major: 1, Minor: 21},
			objs: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "kubelet-config-1.20",
					Namespace:       metav1.NamespaceSystem,
					ResourceVersion: "some-resource-version",
				},
				Data: map[string]string{
					kubeletConfigKey: yaml.Raw(`
						apiVersion: kubelet.config.k8s.io/v1beta1
						kind: KubeletConfiguration
						cgroupDriver: cgroupfs
						foo: bar
					`),
				},
			}},
			expectCgroupDriver: "cgroupfs",
			expectNewConfigMap: true,
		},
		{
			name:      "returns error if cannot find previous config map",
			version:   semver.Version{Major: 1, Minor: 21},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(tt.objs...).Build()
			w := &Workload{
				Client: fakeClient,
			}
			err := w.UpdateKubeletConfigMap(ctx, tt.version)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			// Check if the resulting ConfigMap exists
			var actualConfig corev1.ConfigMap
			g.Expect(w.Client.Get(
				ctx,
				client.ObjectKey{Name: generateKubeletConfigName(tt.version), Namespace: metav1.NamespaceSystem},
				&actualConfig,
			)).To(Succeed())
			// Check other values are carried over for previous config map
			g.Expect(actualConfig.Data[kubeletConfigKey]).To(ContainSubstring("foo"))
			// Check the cgroupvalue has the expected value
			g.Expect(actualConfig.Data[kubeletConfigKey]).To(ContainSubstring(tt.expectCgroupDriver))
			// check if the config map is new
			if tt.expectNewConfigMap {
				g.Expect(actualConfig.ResourceVersion).ToNot(Equal("some-resource-version"))
			} else {
				g.Expect(actualConfig.ResourceVersion).To(Equal("some-resource-version"))
			}
		})
	}
}

func TestUpdateUpdateClusterConfigurationInKubeadmConfigMap(t *testing.T) {
	tests := []struct {
		name          string
		version       semver.Version
		objs          []client.Object
		mutator       func(*bootstrapv1.ClusterConfiguration)
		wantConfigMap *corev1.ConfigMap
		wantErr       bool
	}{
		{
			name:    "fails if missing config map",
			version: semver.MustParse("1.17.2"),
			objs:    nil,
			wantErr: true,
		},
		{
			name:    "fail if config map without ClusterConfiguration data",
			version: semver.MustParse("1.17.2"),
			objs: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{},
			}},
			wantErr: true,
		},
		{
			name:    "fail if config map with invalid ClusterConfiguration data",
			version: semver.MustParse("1.17.2"),
			objs: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: "foo",
				},
			}},
			wantErr: true,
		},
		{
			name:    "no op if mutator does not apply changes",
			version: semver.MustParse("1.17.2"),
			objs: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: yaml.Raw(`
						apiVersion: kubeadm.k8s.io/v1beta2
						kind: ClusterConfiguration
						kubernetesVersion: v1.16.1
						`),
				},
			}},
			mutator: func(c *bootstrapv1.ClusterConfiguration) {},
			wantConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: yaml.Raw(`
						apiVersion: kubeadm.k8s.io/v1beta2
						kind: ClusterConfiguration
						kubernetesVersion: v1.16.1
						`),
				},
			},
		},
		{
			name:    "apply changes",
			version: semver.MustParse("1.17.2"),
			objs: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: yaml.Raw(`
						apiVersion: kubeadm.k8s.io/v1beta2
						kind: ClusterConfiguration
						kubernetesVersion: v1.16.1
						`),
				},
			}},
			mutator: func(c *bootstrapv1.ClusterConfiguration) {
				c.KubernetesVersion = "v1.17.2"
			},
			wantConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: yaml.Raw(`
						apiServer: {}
						apiVersion: kubeadm.k8s.io/v1beta2
						controllerManager: {}
						dns: {}
						etcd: {}
						kind: ClusterConfiguration
						kubernetesVersion: v1.17.2
						networking: {}
						scheduler: {}
						`),
				},
			},
		},
		{
			name:    "converts kubeadm api version during mutation if required",
			version: semver.MustParse("1.17.2"),
			objs: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: yaml.Raw(`
						apiVersion: kubeadm.k8s.io/v1beta1
						kind: ClusterConfiguration
						kubernetesVersion: v1.16.1
						`),
				},
			}},
			mutator: func(c *bootstrapv1.ClusterConfiguration) {
				c.KubernetesVersion = "v1.17.2"
			},
			wantConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: yaml.Raw(`
						apiServer: {}
						apiVersion: kubeadm.k8s.io/v1beta2
						controllerManager: {}
						dns: {}
						etcd: {}
						kind: ClusterConfiguration
						kubernetesVersion: v1.17.2
						networking: {}
						scheduler: {}
						`),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(tt.objs...).Build()

			w := &Workload{
				Client: fakeClient,
			}
			err := w.updateClusterConfiguration(ctx, tt.mutator, tt.version)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			var actualConfig corev1.ConfigMap
			g.Expect(w.Client.Get(
				ctx,
				client.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem},
				&actualConfig,
			)).To(Succeed())
			g.Expect(actualConfig.Data[clusterConfigurationKey]).To(Equal(tt.wantConfigMap.Data[clusterConfigurationKey]), cmp.Diff(tt.wantConfigMap.Data[clusterConfigurationKey], actualConfig.Data[clusterConfigurationKey]))
		})
	}
}

func TestUpdateUpdateClusterStatusInKubeadmConfigMap(t *testing.T) {
	tests := []struct {
		name          string
		version       semver.Version
		objs          []client.Object
		mutator       func(status *bootstrapv1.ClusterStatus)
		wantConfigMap *corev1.ConfigMap
		wantErr       bool
	}{
		{
			name:    "fails if missing config map",
			version: semver.MustParse("1.17.2"),
			objs:    nil,
			wantErr: true,
		},
		{
			name:    "fail if config map without ClusterStatus data",
			version: semver.MustParse("1.17.2"),
			objs: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{},
			}},
			wantErr: true,
		},
		{
			name:    "fail if config map with invalid ClusterStatus data",
			version: semver.MustParse("1.17.2"),
			objs: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterStatusKey: "foo",
				},
			}},
			wantErr: true,
		},
		{
			name:    "no op if mutator does not apply changes",
			version: semver.MustParse("1.17.2"),
			objs: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterStatusKey: yaml.Raw(`
						apiEndpoints:
						  ip-10-0-0-1.ec2.internal:
						    advertiseAddress: 10.0.0.1
						    bindPort: 6443
						apiVersion: kubeadm.k8s.io/v1beta2
						kind: ClusterStatus
						`),
				},
			}},
			mutator: func(status *bootstrapv1.ClusterStatus) {},
			wantConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterStatusKey: yaml.Raw(`
						apiEndpoints:
						  ip-10-0-0-1.ec2.internal:
						    advertiseAddress: 10.0.0.1
						    bindPort: 6443
						apiVersion: kubeadm.k8s.io/v1beta2
						kind: ClusterStatus
						`),
				},
			},
		},
		{
			name:    "apply changes",
			version: semver.MustParse("1.17.2"),
			objs: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterStatusKey: yaml.Raw(`
						apiEndpoints:
						  ip-10-0-0-1.ec2.internal:
						    advertiseAddress: 10.0.0.1
						    bindPort: 6443
						apiVersion: kubeadm.k8s.io/v1beta2
						kind: ClusterStatus
						`),
				},
			}},
			mutator: func(status *bootstrapv1.ClusterStatus) {
				status.APIEndpoints["ip-10-0-0-2.ec2.internal"] = bootstrapv1.APIEndpoint{}
			},
			wantConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterStatusKey: yaml.Raw(`
						apiEndpoints:
						  ip-10-0-0-1.ec2.internal:
						    advertiseAddress: 10.0.0.1
						    bindPort: 6443
						  ip-10-0-0-2.ec2.internal: {}
						apiVersion: kubeadm.k8s.io/v1beta2
						kind: ClusterStatus
						`),
				},
			},
		},
		{
			name:    "converts kubeadm api version during mutation if required",
			version: semver.MustParse("1.17.2"),
			objs: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterStatusKey: yaml.Raw(`
						apiEndpoints:
						  ip-10-0-0-1.ec2.internal:
						    advertiseAddress: 10.0.0.1
						    bindPort: 6443
						apiVersion: kubeadm.k8s.io/v1beta1
						kind: ClusterStatus
						`),
				},
			}},
			mutator: func(status *bootstrapv1.ClusterStatus) {
				status.APIEndpoints["ip-10-0-0-2.ec2.internal"] = bootstrapv1.APIEndpoint{}
			},
			wantConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterStatusKey: yaml.Raw(`
						apiEndpoints:
						  ip-10-0-0-1.ec2.internal:
						    advertiseAddress: 10.0.0.1
						    bindPort: 6443
						  ip-10-0-0-2.ec2.internal: {}
						apiVersion: kubeadm.k8s.io/v1beta2
						kind: ClusterStatus
						`),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(tt.objs...).Build()

			w := &Workload{
				Client: fakeClient,
			}
			err := w.updateClusterStatus(ctx, tt.mutator, tt.version)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			var actualConfig corev1.ConfigMap
			g.Expect(w.Client.Get(
				ctx,
				client.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem},
				&actualConfig,
			)).To(Succeed())
			g.Expect(actualConfig.Data[clusterStatusKey]).To(Equal(tt.wantConfigMap.Data[clusterStatusKey]), cmp.Diff(tt.wantConfigMap.Data[clusterStatusKey], actualConfig.Data[clusterStatusKey]))
		})
	}
}

func TestUpdateKubernetesVersionInKubeadmConfigMap(t *testing.T) {
	tests := []struct {
		name                     string
		version                  semver.Version
		clusterConfigurationData string
	}{
		{
			name:    "updates the config map and changes the kubeadm API version",
			version: semver.MustParse("1.17.2"),
			clusterConfigurationData: yaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta1
				kind: ClusterConfiguration
				kubernetesVersion: v1.16.1`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: tt.clusterConfigurationData,
				},
			}).Build()

			w := &Workload{
				Client: fakeClient,
			}
			err := w.UpdateKubernetesVersionInKubeadmConfigMap(ctx, tt.version)
			g.Expect(err).ToNot(HaveOccurred())

			var actualConfig corev1.ConfigMap
			g.Expect(w.Client.Get(
				ctx,
				client.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem},
				&actualConfig,
			)).To(Succeed())
			g.Expect(actualConfig.Data[clusterConfigurationKey]).To(ContainSubstring(tt.version.String()))
		})
	}
}

func TestUpdateImageRepositoryInKubeadmConfigMap(t *testing.T) {
	tests := []struct {
		name                     string
		clusterConfigurationData string
		newImageRepository       string
		wantImageRepository      string
	}{
		{
			name: "it should set the image repository",
			clusterConfigurationData: yaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta2
				kind: ClusterConfiguration`),
			newImageRepository:  "example.com/k8s",
			wantImageRepository: "example.com/k8s",
		},
		{
			name: "it should preserve the existing image repository if then new value is empty",
			clusterConfigurationData: yaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta2
				kind: ClusterConfiguration
				imageRepository: foo.bar/baz.io`),
			newImageRepository:  "",
			wantImageRepository: "foo.bar/baz.io",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: tt.clusterConfigurationData,
				},
			}).Build()

			w := &Workload{
				Client: fakeClient,
			}
			err := w.UpdateImageRepositoryInKubeadmConfigMap(ctx, tt.newImageRepository, semver.MustParse("1.19.1"))
			g.Expect(err).ToNot(HaveOccurred())

			var actualConfig corev1.ConfigMap
			g.Expect(w.Client.Get(
				ctx,
				client.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem},
				&actualConfig,
			)).To(Succeed())
			g.Expect(actualConfig.Data[clusterConfigurationKey]).To(ContainSubstring(tt.wantImageRepository))
		})
	}
}

func TestUpdateApiServerInKubeadmConfigMap(t *testing.T) {
	tests := []struct {
		name                     string
		clusterConfigurationData string
		newAPIServer             bootstrapv1.APIServer
		wantClusterConfiguration string
	}{
		{
			name: "it should set the api server config",
			clusterConfigurationData: yaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta2
				kind: ClusterConfiguration
				`),
			newAPIServer: bootstrapv1.APIServer{
				ControlPlaneComponent: bootstrapv1.ControlPlaneComponent{
					ExtraArgs: map[string]string{
						"bar":     "baz",
						"someKey": "someVal",
					},
					ExtraVolumes: []bootstrapv1.HostPathMount{
						{
							Name:      "mount2",
							HostPath:  "/bar/baz",
							MountPath: "/foo/bar",
						},
					},
				},
			},
			wantClusterConfiguration: yaml.Raw(`
				apiServer:
				  extraArgs:
				    bar: baz
				    someKey: someVal
				  extraVolumes:
				  - hostPath: /bar/baz
				    mountPath: /foo/bar
				    name: mount2
				apiVersion: kubeadm.k8s.io/v1beta2
				controllerManager: {}
				dns: {}
				etcd: {}
				kind: ClusterConfiguration
				networking: {}
				scheduler: {}
				`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: tt.clusterConfigurationData,
				},
			}).Build()

			w := &Workload{
				Client: fakeClient,
			}
			err := w.UpdateAPIServerInKubeadmConfigMap(ctx, tt.newAPIServer, semver.MustParse("1.19.1"))
			g.Expect(err).ToNot(HaveOccurred())

			var actualConfig corev1.ConfigMap
			g.Expect(w.Client.Get(
				ctx,
				client.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem},
				&actualConfig,
			)).To(Succeed())
			g.Expect(actualConfig.Data[clusterConfigurationKey]).Should(Equal(tt.wantClusterConfiguration), cmp.Diff(tt.wantClusterConfiguration, actualConfig.Data[clusterConfigurationKey]))
		})
	}
}

func TestUpdateControllerManagerInKubeadmConfigMap(t *testing.T) {
	tests := []struct {
		name                     string
		clusterConfigurationData string
		newControllerManager     bootstrapv1.ControlPlaneComponent
		wantClusterConfiguration string
	}{
		{
			name: "it should set the controller manager config",
			clusterConfigurationData: yaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta2
				kind: ClusterConfiguration
				`),
			newControllerManager: bootstrapv1.ControlPlaneComponent{
				ExtraArgs: map[string]string{
					"bar":     "baz",
					"someKey": "someVal",
				},
				ExtraVolumes: []bootstrapv1.HostPathMount{
					{
						Name:      "mount2",
						HostPath:  "/bar/baz",
						MountPath: "/foo/bar",
					},
				},
			},
			wantClusterConfiguration: yaml.Raw(`
				apiServer: {}
				apiVersion: kubeadm.k8s.io/v1beta2
				controllerManager:
				  extraArgs:
				    bar: baz
				    someKey: someVal
				  extraVolumes:
				  - hostPath: /bar/baz
				    mountPath: /foo/bar
				    name: mount2
				dns: {}
				etcd: {}
				kind: ClusterConfiguration
				networking: {}
				scheduler: {}
				`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: tt.clusterConfigurationData,
				},
			}).Build()

			w := &Workload{
				Client: fakeClient,
			}
			err := w.UpdateControllerManagerInKubeadmConfigMap(ctx, tt.newControllerManager, semver.MustParse("1.19.1"))
			g.Expect(err).ToNot(HaveOccurred())

			var actualConfig corev1.ConfigMap
			g.Expect(w.Client.Get(
				ctx,
				client.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem},
				&actualConfig,
			)).To(Succeed())
			g.Expect(actualConfig.Data[clusterConfigurationKey]).Should(Equal(tt.wantClusterConfiguration), cmp.Diff(tt.wantClusterConfiguration, actualConfig.Data[clusterConfigurationKey]))
		})
	}
}

func TestUpdateSchedulerInKubeadmConfigMap(t *testing.T) {
	tests := []struct {
		name                     string
		clusterConfigurationData string
		newScheduler             bootstrapv1.ControlPlaneComponent
		wantClusterConfiguration string
	}{
		{
			name: "it should set the scheduler config",
			clusterConfigurationData: yaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta2
				kind: ClusterConfiguration
				`),
			newScheduler: bootstrapv1.ControlPlaneComponent{
				ExtraArgs: map[string]string{
					"bar":     "baz",
					"someKey": "someVal",
				},
				ExtraVolumes: []bootstrapv1.HostPathMount{
					{
						Name:      "mount2",
						HostPath:  "/bar/baz",
						MountPath: "/foo/bar",
					},
				},
			},
			wantClusterConfiguration: yaml.Raw(`
				apiServer: {}
				apiVersion: kubeadm.k8s.io/v1beta2
				controllerManager: {}
				dns: {}
				etcd: {}
				kind: ClusterConfiguration
				networking: {}
				scheduler:
				  extraArgs:
				    bar: baz
				    someKey: someVal
				  extraVolumes:
				  - hostPath: /bar/baz
				    mountPath: /foo/bar
				    name: mount2
				`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: tt.clusterConfigurationData,
				},
			}).Build()

			w := &Workload{
				Client: fakeClient,
			}
			err := w.UpdateSchedulerInKubeadmConfigMap(ctx, tt.newScheduler, semver.MustParse("1.19.1"))
			g.Expect(err).ToNot(HaveOccurred())

			var actualConfig corev1.ConfigMap
			g.Expect(w.Client.Get(
				ctx,
				client.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem},
				&actualConfig,
			)).To(Succeed())
			g.Expect(actualConfig.Data[clusterConfigurationKey]).Should(Equal(tt.wantClusterConfiguration), cmp.Diff(tt.wantClusterConfiguration, actualConfig.Data[clusterConfigurationKey]))
		})
	}
}

func TestClusterStatus(t *testing.T) {
	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node1",
			Labels: map[string]string{
				labelNodeRoleControlPlane: "",
			},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}
	node2 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node2",
			Labels: map[string]string{
				labelNodeRoleControlPlane: "",
			},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionFalse,
			}},
		},
	}
	kconf := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeadmConfigKey,
			Namespace: metav1.NamespaceSystem,
		},
	}
	tests := []struct {
		name          string
		objs          []client.Object
		expectErr     bool
		expectHasConf bool
	}{
		{
			name:          "returns cluster status",
			objs:          []client.Object{node1, node2},
			expectErr:     false,
			expectHasConf: false,
		},
		{
			name:          "returns cluster status with kubeadm config",
			objs:          []client.Object{node1, node2, kconf},
			expectErr:     false,
			expectHasConf: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(tt.objs...).Build()
			w := &Workload{
				Client: fakeClient,
			}
			status, err := w.ClusterStatus(ctx)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(status.Nodes).To(BeEquivalentTo(2))
			g.Expect(status.ReadyNodes).To(BeEquivalentTo(1))
			if tt.expectHasConf {
				g.Expect(status.HasKubeadmConfig).To(BeTrue())
				return
			}
			g.Expect(status.HasKubeadmConfig).To(BeFalse())
		})
	}
}

func getProxyImageInfo(ctx context.Context, c client.Client) (string, error) {
	ds := &appsv1.DaemonSet{}

	if err := c.Get(ctx, client.ObjectKey{Name: kubeProxyKey, Namespace: metav1.NamespaceSystem}, ds); err != nil {
		if apierrors.IsNotFound(err) {
			return "", errors.New("no image found")
		}
		return "", errors.New("failed to determine if daemonset already exists")
	}
	container := findKubeProxyContainer(ds)
	if container == nil {
		return "", errors.New("unable to find container")
	}
	return container.Image, nil
}

func newKubeProxyDS() appsv1.DaemonSet {
	return appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeProxyKey,
			Namespace: metav1.NamespaceSystem,
		},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "k8s.gcr.io/kube-proxy:v1.16.2",
							Name:  "kube-proxy",
						},
					},
				},
			},
		},
	}
}

func newKubeProxyDSWithImage(image string) appsv1.DaemonSet {
	ds := newKubeProxyDS()
	ds.Spec.Template.Spec.Containers[0].Image = image
	return ds
}
