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

	"github.com/blang/semver/v4"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"
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
						Name: "control-plane-node-with-label",
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
				"control-plane-node-with-label",
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

	for i := range tests {
		tt := tests[i]
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
				gs.Expect(err).ToNot(HaveOccurred())
			}

			proxyImage, err := getProxyImageInfo(ctx, w.Client)
			gs.Expect(err).ToNot(HaveOccurred())
			if tt.expectImage != "" {
				gs.Expect(proxyImage).To(Equal(tt.expectImage))
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
			name:    "no op if mutator does not apply changes and Kubernetes version is up-to-date",
			version: semver.MustParse("1.23.2"),
			objs: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: utilyaml.Raw(`
						apiVersion: kubeadm.k8s.io/v1beta3
						kind: ClusterConfiguration
						kubernetesVersion: v1.23.2
						`),
				},
			}},
			mutator: func(*bootstrapv1.ClusterConfiguration) {},
			wantConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: utilyaml.Raw(`
						apiVersion: kubeadm.k8s.io/v1beta3
						kind: ClusterConfiguration
						kubernetesVersion: v1.23.2
						`),
				},
			},
		},
		{
			name:    "update if mutator does not apply changes and Kubernetes version is outdated",
			version: semver.MustParse("1.23.2"),
			objs: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: utilyaml.Raw(`
						apiVersion: kubeadm.k8s.io/v1beta3
						kind: ClusterConfiguration
						kubernetesVersion: v1.16.1
						`),
				},
			}},
			mutator: func(*bootstrapv1.ClusterConfiguration) {},
			wantConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: utilyaml.Raw(`
						apiServer: {}
						apiVersion: kubeadm.k8s.io/v1beta3
						controllerManager: {}
						dns: {}
						etcd: {}
						kind: ClusterConfiguration
						kubernetesVersion: v1.23.2
						networking: {}
						scheduler: {}
						`),
				},
			},
		},
		{
			name:    "apply changes",
			version: semver.MustParse("1.23.2"),
			objs: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: utilyaml.Raw(`
						apiVersion: kubeadm.k8s.io/v1beta3
						kind: ClusterConfiguration
						kubernetesVersion: v1.16.1
						certificatesDir: bar
						`),
				},
			}},
			mutator: func(c *bootstrapv1.ClusterConfiguration) {
				c.CertificatesDir = "foo"
			},
			wantConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: utilyaml.Raw(`
						apiServer: {}
						apiVersion: kubeadm.k8s.io/v1beta3
						certificatesDir: foo
						controllerManager: {}
						dns: {}
						etcd: {}
						kind: ClusterConfiguration
						kubernetesVersion: v1.23.2
						networking: {}
						scheduler: {}
						`),
				},
			},
		},
		{
			name:    "converts kubeadm api version during mutation if required while preserving upstream only data",
			version: semver.MustParse("1.32.0"),
			objs: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: utilyaml.Raw(`
						apiVersion: kubeadm.k8s.io/v1beta3
						kind: ClusterConfiguration
						certificatesDir: bar
						clusterName: mycluster
						controlPlaneEndpoint: myControlPlaneEndpoint:6443
						kubernetesVersion: v1.23.1
						networking:
						  dnsDomain: myDNSDomain
						  podSubnet: myPodSubnet
						  serviceSubnet: myServiceSubnet
						`),
				},
			}},
			mutator: func(c *bootstrapv1.ClusterConfiguration) {
				c.CertificatesDir = "foo"
			},
			wantConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: utilyaml.Raw(`
						apiServer: {}
						apiVersion: kubeadm.k8s.io/v1beta4
						certificatesDir: foo
						clusterName: mycluster
						controlPlaneEndpoint: myControlPlaneEndpoint:6443
						controllerManager: {}
						dns: {}
						etcd: {}
						kind: ClusterConfiguration
						kubernetesVersion: v1.32.0
						networking:
						  dnsDomain: myDNSDomain
						  podSubnet: myPodSubnet
						  serviceSubnet: myServiceSubnet
						proxy: {}
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
			err := w.UpdateClusterConfiguration(ctx, tt.version, tt.mutator)
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

func TestUpdateImageRepositoryInKubeadmConfigMap(t *testing.T) {
	tests := []struct {
		name                     string
		clusterConfigurationData string
		newImageRepository       string
		wantImageRepository      string
	}{
		{
			name: "it should set the image repository",
			clusterConfigurationData: utilyaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta3
				kind: ClusterConfiguration`),
			newImageRepository:  "example.com/k8s",
			wantImageRepository: "example.com/k8s",
		},
		{
			name: "it should preserve the existing image repository if then new value is empty",
			clusterConfigurationData: utilyaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta3
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
			err := w.UpdateClusterConfiguration(ctx, semver.MustParse("1.23.1"), w.UpdateImageRepositoryInKubeadmConfigMap(tt.newImageRepository))
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
		version                  semver.Version
		clusterConfigurationData string
		newAPIServer             bootstrapv1.APIServer
		wantClusterConfiguration string
	}{
		{
			name:    "it should set the api server config (< 1.31)",
			version: semver.MustParse("1.23.1"),
			clusterConfigurationData: utilyaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta3
				kind: ClusterConfiguration
				`),
			newAPIServer: bootstrapv1.APIServer{
				ExtraArgs: []bootstrapv1.Arg{
					{
						Name:  "bar",
						Value: "baz",
					},
					{
						Name:  "someKey",
						Value: "someVal",
					},
				},
				ExtraVolumes: []bootstrapv1.HostPathMount{
					{
						Name:      "mount2",
						HostPath:  "/bar/baz",
						MountPath: "/foo/bar",
					},
				},
			},
			wantClusterConfiguration: utilyaml.Raw(`
				apiServer:
				  extraArgs:
				    bar: baz
				    someKey: someVal
				  extraVolumes:
				  - hostPath: /bar/baz
				    mountPath: /foo/bar
				    name: mount2
				apiVersion: kubeadm.k8s.io/v1beta3
				controllerManager: {}
				dns: {}
				etcd: {}
				kind: ClusterConfiguration
				kubernetesVersion: v1.23.1
				networking: {}
				scheduler: {}
				`),
		},
		{
			name:    "it should set the api server config (>=1.31)",
			version: semver.MustParse("1.31.1"),
			clusterConfigurationData: utilyaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta4
				kind: ClusterConfiguration
				`),
			newAPIServer: bootstrapv1.APIServer{
				ExtraArgs: []bootstrapv1.Arg{
					{
						Name:  "bar",
						Value: "baz",
					},
					{
						Name:  "someKey",
						Value: "someVal",
					},
				},
				ExtraVolumes: []bootstrapv1.HostPathMount{
					{
						Name:      "mount2",
						HostPath:  "/bar/baz",
						MountPath: "/foo/bar",
					},
				},
			},
			wantClusterConfiguration: utilyaml.Raw(`
				apiServer:
				  extraArgs:
				  - name: bar
				    value: baz
				  - name: someKey
				    value: someVal
				  extraVolumes:
				  - hostPath: /bar/baz
				    mountPath: /foo/bar
				    name: mount2
				apiVersion: kubeadm.k8s.io/v1beta4
				controllerManager: {}
				dns: {}
				etcd: {}
				kind: ClusterConfiguration
				kubernetesVersion: v1.31.1
				networking: {}
				proxy: {}
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
			err := w.UpdateClusterConfiguration(ctx, tt.version, w.UpdateAPIServerInKubeadmConfigMap(tt.newAPIServer))
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
		newControllerManager     bootstrapv1.ControllerManager
		wantClusterConfiguration string
	}{
		{
			name: "it should set the controller manager config",
			clusterConfigurationData: utilyaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta3
				kind: ClusterConfiguration
				`),
			newControllerManager: bootstrapv1.ControllerManager{
				ExtraArgs: []bootstrapv1.Arg{
					{
						Name:  "bar",
						Value: "baz",
					},
					{
						Name:  "someKey",
						Value: "someVal",
					},
				},
				ExtraVolumes: []bootstrapv1.HostPathMount{
					{
						Name:      "mount2",
						HostPath:  "/bar/baz",
						MountPath: "/foo/bar",
					},
				},
			},
			wantClusterConfiguration: utilyaml.Raw(`
				apiServer: {}
				apiVersion: kubeadm.k8s.io/v1beta3
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
				kubernetesVersion: v1.23.1
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
			err := w.UpdateClusterConfiguration(ctx, semver.MustParse("1.23.1"), w.UpdateControllerManagerInKubeadmConfigMap(tt.newControllerManager))
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
		newScheduler             bootstrapv1.Scheduler
		wantClusterConfiguration string
	}{
		{
			name: "it should set the scheduler config",
			clusterConfigurationData: utilyaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta3
				kind: ClusterConfiguration
				`),
			newScheduler: bootstrapv1.Scheduler{
				ExtraArgs: []bootstrapv1.Arg{
					{
						Name:  "bar",
						Value: "baz",
					},
					{
						Name:  "someKey",
						Value: "someVal",
					},
				},
				ExtraVolumes: []bootstrapv1.HostPathMount{
					{
						Name:      "mount2",
						HostPath:  "/bar/baz",
						MountPath: "/foo/bar",
					},
				},
			},
			wantClusterConfiguration: utilyaml.Raw(`
				apiServer: {}
				apiVersion: kubeadm.k8s.io/v1beta3
				controllerManager: {}
				dns: {}
				etcd: {}
				kind: ClusterConfiguration
				kubernetesVersion: v1.23.1
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
			err := w.UpdateClusterConfiguration(ctx, semver.MustParse("1.23.1"), w.UpdateSchedulerInKubeadmConfigMap(tt.newScheduler))
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

func TestUpdateFeatureGatesInKubeadmConfigMap(t *testing.T) {
	tests := []struct {
		name                     string
		clusterConfigurationData string
		kubernetesVersion        semver.Version
		newClusterConfiguration  *bootstrapv1.ClusterConfiguration
		wantClusterConfiguration *bootstrapv1.ClusterConfiguration
	}{
		{
			name: "it updates feature gates",
			clusterConfigurationData: utilyaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta3
				kind: ClusterConfiguration`),
			kubernetesVersion: semver.MustParse("1.23.1"),
			newClusterConfiguration: &bootstrapv1.ClusterConfiguration{
				FeatureGates: map[string]bool{
					"EtcdLearnerMode": true,
				},
			},
			wantClusterConfiguration: &bootstrapv1.ClusterConfiguration{
				FeatureGates: map[string]bool{
					"EtcdLearnerMode": true,
				},
			},
		},
		{
			name: "it should override feature gates even if new value is nil",
			clusterConfigurationData: utilyaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta3
				kind: ClusterConfiguration
				featureGates:
				  EtcdLearnerMode: true
				`),
			kubernetesVersion: semver.MustParse("1.23.1"),
			newClusterConfiguration: &bootstrapv1.ClusterConfiguration{
				FeatureGates: nil,
			},
			wantClusterConfiguration: &bootstrapv1.ClusterConfiguration{
				FeatureGates: nil,
			},
		},
		{
			name: "it should not add ControlPlaneKubeletLocalMode feature gate for 1.30",
			clusterConfigurationData: utilyaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta3
				kind: ClusterConfiguration`),
			kubernetesVersion: semver.MustParse("1.30.0"),
			newClusterConfiguration: &bootstrapv1.ClusterConfiguration{
				FeatureGates: nil,
			},
			wantClusterConfiguration: &bootstrapv1.ClusterConfiguration{
				FeatureGates: nil,
			},
		},
		{
			name: "it should add ControlPlaneKubeletLocalMode feature gate for 1.31.0",
			clusterConfigurationData: utilyaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta4
				kind: ClusterConfiguration`),
			kubernetesVersion: semver.MustParse("1.31.0"),
			newClusterConfiguration: &bootstrapv1.ClusterConfiguration{
				FeatureGates: nil,
			},
			wantClusterConfiguration: &bootstrapv1.ClusterConfiguration{
				FeatureGates: map[string]bool{
					ControlPlaneKubeletLocalMode: true,
				},
			},
		},
		{
			name: "it should add ControlPlaneKubeletLocalMode feature gate for 1.31.0 if other feature gate is set",
			clusterConfigurationData: utilyaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta4
				kind: ClusterConfiguration`),
			kubernetesVersion: semver.MustParse("1.31.0"),
			newClusterConfiguration: &bootstrapv1.ClusterConfiguration{
				FeatureGates: map[string]bool{
					"EtcdLearnerMode": true,
				},
			},
			wantClusterConfiguration: &bootstrapv1.ClusterConfiguration{
				FeatureGates: map[string]bool{
					ControlPlaneKubeletLocalMode: true,
					"EtcdLearnerMode":            true,
				},
			},
		},
		{
			name: "it should preserve ControlPlaneKubeletLocalMode false feature gate for 1.31.0",
			clusterConfigurationData: utilyaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta4
				kind: ClusterConfiguration`),
			kubernetesVersion: semver.MustParse("1.31.0"),
			newClusterConfiguration: &bootstrapv1.ClusterConfiguration{
				FeatureGates: map[string]bool{
					"EtcdLearnerMode":            true,
					ControlPlaneKubeletLocalMode: false,
				},
			},
			wantClusterConfiguration: &bootstrapv1.ClusterConfiguration{
				FeatureGates: map[string]bool{
					ControlPlaneKubeletLocalMode: false,
					"EtcdLearnerMode":            true,
				},
			},
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
			err := w.UpdateClusterConfiguration(ctx, tt.kubernetesVersion, w.UpdateFeatureGatesInKubeadmConfigMap(bootstrapv1.KubeadmConfigSpec{ClusterConfiguration: tt.newClusterConfiguration}, tt.kubernetesVersion))
			g.Expect(err).ToNot(HaveOccurred())

			var actualConfig corev1.ConfigMap
			g.Expect(w.Client.Get(
				ctx,
				client.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem},
				&actualConfig,
			)).To(Succeed())

			actualConfiguration := &bootstrapv1.ClusterConfiguration{}
			err = yaml.Unmarshal([]byte(actualConfig.Data[clusterConfigurationKey]), actualConfiguration)
			if err != nil {
				return
			}
			g.Expect(actualConfiguration).Should(BeComparableTo(tt.wantClusterConfiguration))
		})
	}
}

func TestUpdateCertificateValidityPeriodDaysInKubeadmConfigMap(t *testing.T) {
	tests := []struct {
		name                     string
		clusterConfigurationData string
		certificateValidityDays  int32
		wantClusterConfiguration string
	}{
		{
			name:                    "it should set the certificateValidityDays in config",
			certificateValidityDays: int32(5),
			clusterConfigurationData: utilyaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta3
				kind: ClusterConfiguration
				`),
			wantClusterConfiguration: utilyaml.Raw(`
				apiServer: {}
				apiVersion: kubeadm.k8s.io/v1beta4
				certificateValidityPeriod: 120h0m0s
				controllerManager: {}
				dns: {}
				etcd: {}
				kind: ClusterConfiguration
				kubernetesVersion: v1.33.1
				networking: {}
				proxy: {}
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
			err := w.UpdateClusterConfiguration(ctx, semver.MustParse("1.33.1"), w.UpdateCertificateValidityPeriodDays(tt.certificateValidityDays))
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

func TestDefaultFeatureGates(t *testing.T) {
	tests := []struct {
		name                  string
		kubernetesVersion     semver.Version
		kubeadmConfigSpec     *bootstrapv1.KubeadmConfigSpec
		wantKubeadmConfigSpec *bootstrapv1.KubeadmConfigSpec
	}{
		{
			name:              "don't default ControlPlaneKubeletLocalMode for 1.30",
			kubernetesVersion: semver.MustParse("1.30.99"),
			kubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{
						"EtcdLearnerMode": true,
					},
				},
			},
			wantKubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{
						"EtcdLearnerMode": true,
					},
				},
			},
		},
		{
			name:              "default ControlPlaneKubeletLocalMode for 1.31",
			kubernetesVersion: semver.MustParse("1.31.0"),
			kubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: nil,
			},
			wantKubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{
						ControlPlaneKubeletLocalMode: true,
					},
				},
			},
		},
		{
			name:              "default ControlPlaneKubeletLocalMode for 1.31",
			kubernetesVersion: semver.MustParse("1.31.0"),
			kubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
					FeatureGates: nil,
				},
			},
			wantKubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{
						ControlPlaneKubeletLocalMode: true,
					},
				},
			},
		},
		{
			name:              "default ControlPlaneKubeletLocalMode for 1.31",
			kubernetesVersion: semver.MustParse("1.31.0"),
			kubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{},
				},
			},
			wantKubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{
						ControlPlaneKubeletLocalMode: true,
					},
				},
			},
		},
		{
			name:              "default ControlPlaneKubeletLocalMode for 1.31",
			kubernetesVersion: semver.MustParse("1.31.0"),
			kubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{
						"EtcdLearnerMode": true,
					},
				},
			},
			wantKubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{
						ControlPlaneKubeletLocalMode: true,
						"EtcdLearnerMode":            true,
					},
				},
			},
		},
		{
			name:              "don't default ControlPlaneKubeletLocalMode for 1.31 if already set to false",
			kubernetesVersion: semver.MustParse("1.31.0"),
			kubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{
						ControlPlaneKubeletLocalMode: false,
					},
				},
			},
			wantKubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{
						ControlPlaneKubeletLocalMode: false,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			DefaultFeatureGates(tt.kubeadmConfigSpec, tt.kubernetesVersion)
			g.Expect(tt.wantKubeadmConfigSpec).Should(BeComparableTo(tt.kubeadmConfigSpec))
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
