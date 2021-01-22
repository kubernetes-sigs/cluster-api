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
	"time"

	. "github.com/onsi/gomega"

	"github.com/blang/semver"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	cabpkv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	kubeadmv1beta1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestUpdateKubeProxyImageInfo(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())

	tests := []struct {
		name        string
		ds          appsv1.DaemonSet
		expectErr   bool
		expectImage string
		clientGet   map[string]interface{}
		patchErr    error
		KCP         *v1alpha3.KubeadmControlPlane
	}{
		{
			name:        "succeeds if patch correctly",
			ds:          newKubeProxyDS(),
			expectErr:   false,
			expectImage: "k8s.gcr.io/kube-proxy:v1.16.3",
			KCP:         &v1alpha3.KubeadmControlPlane{Spec: v1alpha3.KubeadmControlPlaneSpec{Version: "v1.16.3"}},
		},
		{
			name:        "returns error if image in kube-proxy ds was in digest format",
			ds:          newKubeProxyDSWithImage("k8s.gcr.io/kube-proxy@sha256:47bfd"),
			expectErr:   true,
			expectImage: "k8s.gcr.io/kube-proxy@sha256:47bfd",
			KCP:         &v1alpha3.KubeadmControlPlane{Spec: v1alpha3.KubeadmControlPlaneSpec{Version: "v1.16.3"}},
		},
		{
			name:        "expects OCI compatible format of tag",
			ds:          newKubeProxyDS(),
			expectErr:   false,
			expectImage: "k8s.gcr.io/kube-proxy:v1.16.3_build1",
			KCP:         &v1alpha3.KubeadmControlPlane{Spec: v1alpha3.KubeadmControlPlaneSpec{Version: "v1.16.3+build1"}},
		},
		{
			name:      "returns error if image in kube-proxy ds was in wrong format",
			ds:        newKubeProxyDSWithImage(""),
			expectErr: true,
			KCP:       &v1alpha3.KubeadmControlPlane{Spec: v1alpha3.KubeadmControlPlaneSpec{Version: "v1.16.3"}},
		},
		{
			name:        "updates image repository if one has been set on the control plane",
			ds:          newKubeProxyDS(),
			expectErr:   false,
			expectImage: "foo.bar.example/baz/qux/kube-proxy:v1.16.3",
			KCP: &v1alpha3.KubeadmControlPlane{
				Spec: v1alpha3.KubeadmControlPlaneSpec{
					Version: "v1.16.3",
					KubeadmConfigSpec: cabpkv1.KubeadmConfigSpec{
						ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{
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
			KCP: &v1alpha3.KubeadmControlPlane{
				Spec: v1alpha3.KubeadmControlPlaneSpec{
					Version: "v1.16.3",
					KubeadmConfigSpec: cabpkv1.KubeadmConfigSpec{
						ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{
							ImageRepository: "",
						},
					},
				}},
		},
		{
			name:      "returns error if image repository is invalid",
			ds:        newKubeProxyDS(),
			expectErr: true,
			KCP: &v1alpha3.KubeadmControlPlane{
				Spec: v1alpha3.KubeadmControlPlaneSpec{
					Version: "v1.16.3",
					KubeadmConfigSpec: cabpkv1.KubeadmConfigSpec{
						ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{
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
			KCP: &v1alpha3.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1alpha3.SkipKubeProxyAnnotation: "",
					},
				},
				Spec: v1alpha3.KubeadmControlPlaneSpec{
					Version: "v1.16.3",
				}},
		},
	}

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := NewWithT(t)

			objects := []runtime.Object{
				&tt.ds,
			}
			fakeClient := fake.NewFakeClientWithScheme(scheme, objects...)
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
			clusterStatusKey: `apiEndpoints:
  ip-10-0-0-1.ec2.internal:
    advertiseAddress: 10.0.0.1
    bindPort: 6443
  ip-10-0-0-2.ec2.internal:
    advertiseAddress: 10.0.0.2
    bindPort: 6443
    someFieldThatIsAddedInTheFuture: bar
apiVersion: kubeadm.k8s.io/vNbetaM
kind: ClusterStatus`,
		},
		BinaryData: map[string][]byte{
			"": nil,
		},
	}
	kconfWithoutKey := kubeadmConfig.DeepCopy()
	delete(kconfWithoutKey.Data, clusterStatusKey)

	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	tests := []struct {
		name              string
		machine           *clusterv1.Machine
		objs              []runtime.Object
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
			name:      "returns error if unable to remove api endpoint",
			machine:   machine,
			objs:      []runtime.Object{kconfWithoutKey},
			expectErr: true,
		},
		{
			name:      "removes the machine node ref from kubeadm config",
			machine:   machine,
			objs:      []runtime.Object{kubeadmConfig},
			expectErr: false,
			expectedEndpoints: `apiEndpoints:
  ip-10-0-0-2.ec2.internal:
    advertiseAddress: 10.0.0.2
    bindPort: 6443
    someFieldThatIsAddedInTheFuture: bar
apiVersion: kubeadm.k8s.io/vNbetaM
kind: ClusterStatus
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewFakeClientWithScheme(scheme, tt.objs...)
			w := &Workload{
				Client: fakeClient,
			}
			ctx := context.TODO()
			err := w.RemoveMachineFromKubeadmConfigMap(ctx, tt.machine)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			if tt.expectedEndpoints != "" {
				var actualConfig corev1.ConfigMap
				g.Expect(w.Client.Get(
					ctx,
					ctrlclient.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem},
					&actualConfig,
				)).To(Succeed())
				g.Expect(actualConfig.Data[clusterStatusKey]).To(Equal(tt.expectedEndpoints))
			}
		})
	}
}

func TestUpdateKubeletConfigMap(t *testing.T) {
	kubeletConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "kubelet-config-1.1",
			Namespace:       metav1.NamespaceSystem,
			ResourceVersion: "some-resource-version",
		},
	}

	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	tests := []struct {
		name      string
		version   semver.Version
		objs      []runtime.Object
		expectErr bool
	}{
		{
			name:      "create new config map",
			version:   semver.Version{Major: 1, Minor: 2},
			objs:      []runtime.Object{kubeletConfig},
			expectErr: false,
		},
		{
			name:      "returns error if cannot find previous config map",
			version:   semver.Version{Major: 1, Minor: 2},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewFakeClientWithScheme(scheme, tt.objs...)
			w := &Workload{
				Client: fakeClient,
			}
			ctx := context.TODO()
			err := w.UpdateKubeletConfigMap(ctx, tt.version)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			var actualConfig corev1.ConfigMap
			g.Expect(w.Client.Get(
				ctx,
				ctrlclient.ObjectKey{Name: "kubelet-config-1.2", Namespace: metav1.NamespaceSystem},
				&actualConfig,
			)).To(Succeed())
			g.Expect(actualConfig.ResourceVersion).ToNot(Equal(kubeletConfig.ResourceVersion))
		})
	}
}

func TestUpdateKubernetesVersionInKubeadmConfigMap(t *testing.T) {
	kubeadmConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeadmConfigKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			clusterConfigurationKey: `
apiVersion: kubeadm.k8s.io/v1beta2
kind: ClusterConfiguration
kubernetesVersion: v1.16.1
`,
		},
	}

	kubeadmConfigNoKey := kubeadmConfig.DeepCopy()
	delete(kubeadmConfigNoKey.Data, clusterConfigurationKey)

	kubeadmConfigBadData := kubeadmConfig.DeepCopy()
	kubeadmConfigBadData.Data[clusterConfigurationKey] = `foobar`

	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	tests := []struct {
		name      string
		version   semver.Version
		objs      []runtime.Object
		expectErr bool
	}{
		{
			name:      "updates the config map",
			version:   semver.Version{Major: 1, Minor: 17, Patch: 2},
			objs:      []runtime.Object{kubeadmConfig},
			expectErr: false,
		},
		{
			name:      "returns error if cannot find config map",
			version:   semver.Version{Major: 1, Minor: 2},
			expectErr: true,
		},
		{
			name:      "returns error if config has bad data",
			version:   semver.Version{Major: 1, Minor: 2},
			objs:      []runtime.Object{kubeadmConfigBadData},
			expectErr: true,
		},
		{
			name:      "returns error if config doesn't have cluster config key",
			version:   semver.Version{Major: 1, Minor: 2},
			objs:      []runtime.Object{kubeadmConfigNoKey},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewFakeClientWithScheme(scheme, tt.objs...)
			w := &Workload{
				Client: fakeClient,
			}
			ctx := context.TODO()
			err := w.UpdateKubernetesVersionInKubeadmConfigMap(ctx, tt.version)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			var actualConfig corev1.ConfigMap
			g.Expect(w.Client.Get(
				ctx,
				ctrlclient.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem},
				&actualConfig,
			)).To(Succeed())
			g.Expect(actualConfig.Data[clusterConfigurationKey]).To(ContainSubstring("kubernetesVersion: v1.17.2"))
		})
	}
}

func TestUpdateImageRepositoryInKubeadmConfigMap(t *testing.T) {
	kubeadmConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeadmConfigKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			clusterConfigurationKey: `
apiVersion: kubeadm.k8s.io/v1beta2
kind: ClusterConfiguration
imageRepository: k8s.gcr.io
`,
		},
	}

	kubeadmConfigNoKey := kubeadmConfig.DeepCopy()
	delete(kubeadmConfigNoKey.Data, clusterConfigurationKey)

	kubeadmConfigBadData := kubeadmConfig.DeepCopy()
	kubeadmConfigBadData.Data[clusterConfigurationKey] = `foobar`

	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	tests := []struct {
		name            string
		imageRepository string
		objs            []runtime.Object
		expectErr       bool
	}{
		{
			name:            "updates the config map",
			imageRepository: "myspecialrepo.io",
			objs:            []runtime.Object{kubeadmConfig},
			expectErr:       false,
		},
		{
			name:      "returns error if cannot find config map",
			expectErr: true,
		},
		{
			name:            "returns error if config has bad data",
			objs:            []runtime.Object{kubeadmConfigBadData},
			imageRepository: "myspecialrepo.io",
			expectErr:       true,
		},
		{
			name:            "returns error if config doesn't have cluster config key",
			objs:            []runtime.Object{kubeadmConfigNoKey},
			imageRepository: "myspecialrepo.io",
			expectErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewFakeClientWithScheme(scheme, tt.objs...)
			w := &Workload{
				Client: fakeClient,
			}
			ctx := context.TODO()
			err := w.UpdateImageRepositoryInKubeadmConfigMap(ctx, tt.imageRepository)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			var actualConfig corev1.ConfigMap
			g.Expect(w.Client.Get(
				ctx,
				ctrlclient.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem},
				&actualConfig,
			)).To(Succeed())
			g.Expect(actualConfig.Data[clusterConfigurationKey]).To(ContainSubstring(tt.imageRepository))
		})
	}
}

func TestUpdateApiServerInKubeadmConfigMap(t *testing.T) {
	validAPIServerConfig := `apiServer:
  certSANs:
  - foo
  extraArgs:
    foo: bar
  extraVolumes:
  - hostPath: /foo/bar
    mountPath: /bar/baz
    name: mount1
  timeoutForControlPlane: 3m0s
apiVersion: kubeadm.k8s.io/v1beta2
kind: ClusterConfiguration
`
	kubeadmConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeadmConfigKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			clusterConfigurationKey: validAPIServerConfig,
		},
	}

	kubeadmConfigNoKey := kubeadmConfig.DeepCopy()
	delete(kubeadmConfigNoKey.Data, clusterConfigurationKey)

	kubeadmConfigBadData := kubeadmConfig.DeepCopy()
	kubeadmConfigBadData.Data[clusterConfigurationKey] = `badConfigAPIServer`

	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	tests := []struct {
		name              string
		apiServer         kubeadmv1beta1.APIServer
		objs              []runtime.Object
		expectErr         bool
		expectedChanged   bool
		expectedAPIServer string
	}{
		{
			name:            "updates the config map",
			apiServer:       kubeadmv1beta1.APIServer{CertSANs: []string{"foo", "bar"}},
			objs:            []runtime.Object{kubeadmConfig},
			expectErr:       false,
			expectedChanged: true,
			expectedAPIServer: `apiServer:
  certSANs:
  - foo
  - bar
apiVersion: kubeadm.k8s.io/v1beta2
kind: ClusterConfiguration
`,
		},
		{
			name:              "returns error if cannot find config map",
			expectErr:         true,
			expectedAPIServer: validAPIServerConfig,
		},
		{
			name:              "returns error if config has bad data",
			objs:              []runtime.Object{kubeadmConfigBadData},
			apiServer:         kubeadmv1beta1.APIServer{CertSANs: []string{"foo", "bar"}},
			expectErr:         true,
			expectedAPIServer: validAPIServerConfig,
		},
		{
			name:              "returns error if config doesn't have cluster config key",
			objs:              []runtime.Object{kubeadmConfigNoKey},
			apiServer:         kubeadmv1beta1.APIServer{CertSANs: []string{"foo", "bar"}},
			expectErr:         true,
			expectedAPIServer: validAPIServerConfig,
		},
		{
			name:            "should not update config map if no changes are detected",
			objs:            []runtime.Object{kubeadmConfig},
			expectedChanged: false,
			apiServer: kubeadmv1beta1.APIServer{
				ControlPlaneComponent: kubeadmv1beta1.ControlPlaneComponent{
					ExtraArgs:    map[string]string{"foo": "bar"},
					ExtraVolumes: []kubeadmv1beta1.HostPathMount{{Name: "mount1", HostPath: "/foo/bar", MountPath: "/bar/baz"}},
				},
				CertSANs:               []string{"foo"},
				TimeoutForControlPlane: &metav1.Duration{Duration: 3 * time.Minute},
			},
			expectedAPIServer: validAPIServerConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeClient := fake.NewFakeClientWithScheme(scheme, tt.objs...)
			w := &Workload{
				Client: fakeClient,
			}

			err := w.UpdateAPIServerInKubeadmConfigMap(ctx, tt.apiServer)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			var actualConfig corev1.ConfigMap
			g.Expect(w.Client.Get(
				ctx,
				ctrlclient.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem},
				&actualConfig,
			)).To(Succeed())
			g.Expect(actualConfig.Data[clusterConfigurationKey]).Should(Equal(tt.expectedAPIServer))

			// check resource version to see if client.update was called or not
			if !tt.expectedChanged {
				g.Expect(tt.objs[0].(*corev1.ConfigMap).ResourceVersion).Should(Equal(actualConfig.ResourceVersion))
			} else {
				g.Expect(tt.objs[0].(*corev1.ConfigMap).ResourceVersion).ShouldNot(Equal(actualConfig.ResourceVersion))
			}
		})
	}
}

func TestUpdateControllerManagerInKubeadmConfigMap(t *testing.T) {
	validControllerManagerConfig := `apiVersion: kubeadm.k8s.io/v1beta2
controllerManager:
  extraArgs:
    foo: bar
  extraVolumes:
  - hostPath: /foo/bar
    mountPath: /bar/baz
    name: mount1
kind: ClusterConfiguration
`
	kubeadmConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeadmConfigKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			clusterConfigurationKey: validControllerManagerConfig,
		},
	}

	kubeadmConfigNoKey := kubeadmConfig.DeepCopy()
	delete(kubeadmConfigNoKey.Data, clusterConfigurationKey)

	kubeadmConfigBadData := kubeadmConfig.DeepCopy()
	kubeadmConfigBadData.Data[clusterConfigurationKey] = `badConfigControllerManager`

	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	tests := []struct {
		name                      string
		controllerManager         kubeadmv1beta1.ControlPlaneComponent
		objs                      []runtime.Object
		expectErr                 bool
		expectedChanged           bool
		expectedControllerManager string
	}{
		{
			name:              "updates the config map",
			controllerManager: kubeadmv1beta1.ControlPlaneComponent{ExtraArgs: map[string]string{"foo": "bar"}},
			objs:              []runtime.Object{kubeadmConfig},
			expectErr:         false,
			expectedChanged:   true,
			expectedControllerManager: `apiVersion: kubeadm.k8s.io/v1beta2
controllerManager:
  extraArgs:
    foo: bar
kind: ClusterConfiguration
`,
		},
		{
			name:                      "returns error if cannot find config map",
			expectErr:                 true,
			expectedControllerManager: validControllerManagerConfig,
		},
		{
			name:                      "returns error if config has bad data",
			objs:                      []runtime.Object{kubeadmConfigBadData},
			controllerManager:         kubeadmv1beta1.ControlPlaneComponent{ExtraArgs: map[string]string{"foo": "bar"}},
			expectErr:                 true,
			expectedControllerManager: validControllerManagerConfig,
		},
		{
			name:                      "returns error if config doesn't have cluster config key",
			objs:                      []runtime.Object{kubeadmConfigNoKey},
			controllerManager:         kubeadmv1beta1.ControlPlaneComponent{ExtraArgs: map[string]string{"foo": "bar"}},
			expectErr:                 true,
			expectedControllerManager: validControllerManagerConfig,
		},
		{
			name:            "should not update config map if no changes are detected",
			objs:            []runtime.Object{kubeadmConfig},
			expectedChanged: false,
			controllerManager: kubeadmv1beta1.ControlPlaneComponent{
				ExtraArgs:    map[string]string{"foo": "bar"},
				ExtraVolumes: []kubeadmv1beta1.HostPathMount{{Name: "mount1", HostPath: "/foo/bar", MountPath: "/bar/baz"}},
			},
			expectedControllerManager: validControllerManagerConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeClient := fake.NewFakeClientWithScheme(scheme, tt.objs...)
			w := &Workload{
				Client: fakeClient,
			}
			err := w.UpdateControllerManagerInKubeadmConfigMap(ctx, tt.controllerManager)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			var actualConfig corev1.ConfigMap
			g.Expect(w.Client.Get(
				ctx,
				ctrlclient.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem},
				&actualConfig,
			)).To(Succeed())
			g.Expect(actualConfig.Data[clusterConfigurationKey]).Should(Equal(tt.expectedControllerManager))

			// check resource version to see if client.update was called or not
			if !tt.expectedChanged {
				g.Expect(tt.objs[0].(*corev1.ConfigMap).ResourceVersion).Should(Equal(actualConfig.ResourceVersion))
			} else {
				g.Expect(tt.objs[0].(*corev1.ConfigMap).ResourceVersion).ShouldNot(Equal(actualConfig.ResourceVersion))
			}
		})
	}
}

func TestUpdateSchedulerInKubeadmConfigMap(t *testing.T) {
	validSchedulerConfig := `apiVersion: kubeadm.k8s.io/v1beta2
kind: ClusterConfiguration
scheduler:
  extraArgs:
    foo: bar
  extraVolumes:
  - hostPath: /foo/bar
    mountPath: /bar/baz
    name: mount1
`
	kubeadmConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeadmConfigKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			clusterConfigurationKey: validSchedulerConfig,
		},
	}

	kubeadmConfigNoKey := kubeadmConfig.DeepCopy()
	delete(kubeadmConfigNoKey.Data, clusterConfigurationKey)

	kubeadmConfigBadData := kubeadmConfig.DeepCopy()
	kubeadmConfigBadData.Data[clusterConfigurationKey] = `badConfigScheduler`

	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	tests := []struct {
		name              string
		scheduler         kubeadmv1beta1.ControlPlaneComponent
		objs              []runtime.Object
		expectErr         bool
		expectedChanged   bool
		expectedScheduler string
	}{
		{
			name:            "updates the config map",
			scheduler:       kubeadmv1beta1.ControlPlaneComponent{ExtraArgs: map[string]string{"foo": "bar"}},
			objs:            []runtime.Object{kubeadmConfig},
			expectErr:       false,
			expectedChanged: true,
			expectedScheduler: `apiVersion: kubeadm.k8s.io/v1beta2
kind: ClusterConfiguration
scheduler:
  extraArgs:
    foo: bar
`,
		},
		{
			name:              "returns error if cannot find config map",
			expectErr:         true,
			expectedScheduler: validSchedulerConfig,
		},
		{
			name:              "returns error if config has bad data",
			objs:              []runtime.Object{kubeadmConfigBadData},
			scheduler:         kubeadmv1beta1.ControlPlaneComponent{ExtraArgs: map[string]string{"foo": "bar"}},
			expectErr:         true,
			expectedScheduler: validSchedulerConfig,
		},
		{
			name:              "returns error if config doesn't have cluster config key",
			objs:              []runtime.Object{kubeadmConfigNoKey},
			scheduler:         kubeadmv1beta1.ControlPlaneComponent{ExtraArgs: map[string]string{"foo": "bar"}},
			expectErr:         true,
			expectedScheduler: validSchedulerConfig,
		},
		{
			name:            "should not update config map if no changes are detected",
			objs:            []runtime.Object{kubeadmConfig},
			expectedChanged: false,
			scheduler: kubeadmv1beta1.ControlPlaneComponent{
				ExtraArgs:    map[string]string{"foo": "bar"},
				ExtraVolumes: []kubeadmv1beta1.HostPathMount{{Name: "mount1", HostPath: "/foo/bar", MountPath: "/bar/baz"}},
			},
			expectedScheduler: validSchedulerConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeClient := fake.NewFakeClientWithScheme(scheme, tt.objs...)
			w := &Workload{
				Client: fakeClient,
			}
			err := w.UpdateSchedulerInKubeadmConfigMap(ctx, tt.scheduler)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			var actualConfig corev1.ConfigMap
			g.Expect(w.Client.Get(
				ctx,
				ctrlclient.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem},
				&actualConfig,
			)).To(Succeed())
			g.Expect(actualConfig.Data[clusterConfigurationKey]).Should(Equal(tt.expectedScheduler))

			// check resource version to see if client.update was called or not
			if !tt.expectedChanged {
				g.Expect(tt.objs[0].(*corev1.ConfigMap).ResourceVersion).Should(Equal(actualConfig.ResourceVersion))
			} else {
				g.Expect(tt.objs[0].(*corev1.ConfigMap).ResourceVersion).ShouldNot(Equal(actualConfig.ResourceVersion))
			}
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
		objs          []runtime.Object
		expectErr     bool
		expectHasConf bool
	}{
		{
			name:          "returns cluster status",
			objs:          []runtime.Object{node1, node2},
			expectErr:     false,
			expectHasConf: false,
		},
		{
			name:          "returns cluster status with kubeadm config",
			objs:          []runtime.Object{node1, node2, kconf},
			expectErr:     false,
			expectHasConf: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			scheme := runtime.NewScheme()
			g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
			fakeClient := fake.NewFakeClientWithScheme(scheme, tt.objs...)
			w := &Workload{
				Client: fakeClient,
			}
			ctx := context.TODO()
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

func getProxyImageInfo(ctx context.Context, client ctrlclient.Client) (string, error) {
	ds := &appsv1.DaemonSet{}

	if err := client.Get(ctx, ctrlclient.ObjectKey{Name: kubeProxyKey, Namespace: metav1.NamespaceSystem}, ds); err != nil {
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
