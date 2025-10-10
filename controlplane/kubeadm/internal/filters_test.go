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
	"testing"
	"time"

	"github.com/blang/semver/v4"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	kubeadmtypes "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstream"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/desiredstate"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestMatchesKubeadmConfig(t *testing.T) {
	t.Run("returns true if Machine configRef is not defined", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "machine",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					// ConfigRef not defined
				},
			},
		}
		reason, _, _, match, err := matchesKubeadmConfig(map[string]*bootstrapv1.KubeadmConfig{}, nil, &clusterv1.Cluster{}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeTrue())
		g.Expect(reason).To(BeEmpty())
	})
	t.Run("returns true if Machine KubeadmConfig is not found", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "machine",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: bootstrapv1.GroupVersion.Group,
						Kind:     "KubeadmConfig",
						Name:     "test",
					},
				},
			},
		}
		reason, _, _, match, err := matchesKubeadmConfig(map[string]*bootstrapv1.KubeadmConfig{}, nil, &clusterv1.Cluster{}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeTrue())
		g.Expect(reason).To(BeEmpty())
	})
	t.Run("returns true if ClusterConfiguration is equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: bootstrapv1.ClusterConfiguration{
						CertificatesDir: "foo",
					},
				},
				Version: "v1.30.0",
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "machine",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: bootstrapv1.GroupVersion.Group,
						Kind:     "KubeadmConfig",
						Name:     "test",
					},
				},
			},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{
					CertificatesDir: "foo",
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: machineConfig,
		}
		reason, currentKubeadmConfig, desiredKubeadmConfig, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentKubeadmConfig).ToNot(BeNil())
		g.Expect(desiredKubeadmConfig).ToNot(BeNil())
		g.Expect(match).To(BeTrue())
		g.Expect(reason).To(BeEmpty())
	})
	t.Run("returns true if ClusterConfiguration is equal (empty)", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: bootstrapv1.ClusterConfiguration{},
				},
				Version: "v1.30.0",
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "machine",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: bootstrapv1.GroupVersion.Group,
						Kind:     "KubeadmConfig",
						Name:     "test",
					},
				},
			},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: machineConfig,
		}
		reason, currentKubeadmConfig, desiredKubeadmConfig, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentKubeadmConfig).ToNot(BeNil())
		g.Expect(desiredKubeadmConfig).ToNot(BeNil())
		g.Expect(match).To(BeTrue())
		g.Expect(reason).To(BeEmpty())
	})
	t.Run("returns true if ClusterConfiguration is equal apart from defaulted FeatureGates field", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: bootstrapv1.ClusterConfiguration{},
				},
				Version: "v1.31.0",
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "machine",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: bootstrapv1.GroupVersion.Group,
						Kind:     "KubeadmConfig",
						Name:     "test",
					},
				},
			},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{
						desiredstate.ControlPlaneKubeletLocalMode: true,
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: machineConfig,
		}
		reason, currentKubeadmConfig, desiredKubeadmConfig, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentKubeadmConfig).ToNot(BeNil())
		g.Expect(desiredKubeadmConfig).ToNot(BeNil())
		g.Expect(match).To(BeTrue())
		g.Expect(reason).To(BeEmpty())
	})
	t.Run("returns true if ClusterConfiguration is equal apart from ControlPlaneEndpoint and DNS fields", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: bootstrapv1.ClusterConfiguration{
						DNS: bootstrapv1.DNS{
							ImageTag:        "v1.10.1",
							ImageRepository: "gcr.io/capi-test",
						},
					},
				},
				Version: "v1.30.0",
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "machine",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: bootstrapv1.GroupVersion.Group,
						Kind:     "KubeadmConfig",
						Name:     "test",
					},
				},
			},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{
					ControlPlaneEndpoint: "1.2.3.4:6443",
					DNS: bootstrapv1.DNS{
						ImageTag:        "v1.9.3",
						ImageRepository: "gcr.io/capi-test",
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: machineConfig,
		}
		reason, currentKubeadmConfig, desiredKubeadmConfig, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentKubeadmConfig).ToNot(BeNil())
		g.Expect(desiredKubeadmConfig).ToNot(BeNil())
		g.Expect(match).To(BeTrue())
		g.Expect(reason).To(BeEmpty())
	})
	t.Run("returns false if ClusterConfiguration is NOT equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: bootstrapv1.ClusterConfiguration{
						CertificatesDir: "foo",
					},
				},
				Version: "v1.30.0",
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "machine",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: bootstrapv1.GroupVersion.Group,
						Kind:     "KubeadmConfig",
						Name:     "test",
					},
				},
			},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{
					CertificatesDir: "bar",
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: machineConfig,
		}
		reason, currentKubeadmConfig, desiredKubeadmConfig, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentKubeadmConfig).ToNot(BeNil())
		g.Expect(desiredKubeadmConfig).ToNot(BeNil())
		g.Expect(match).To(BeFalse())
		g.Expect(reason).To(BeComparableTo(`Machine KubeadmConfig is outdated: diff: &v1beta2.KubeadmConfigSpec{
    ClusterConfiguration: v1beta2.ClusterConfiguration{
      ... // 4 identical fields
      Scheduler:       {},
      DNS:             {},
-     CertificatesDir: "bar",
+     CertificatesDir: "foo",
      ImageRepository: "",
      FeatureGates:    nil,
      ... // 2 identical fields
    },
    InitConfiguration: {NodeRegistration: {ImagePullPolicy: "IfNotPresent"}},
    JoinConfiguration: {NodeRegistration: {ImagePullPolicy: "IfNotPresent"}},
    ... // 11 identical fields
  }`))
	})
	t.Run("returns true if InitConfiguration is equal after conversion to JoinConfiguration", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: bootstrapv1.ClusterConfiguration{},
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						Timeouts: bootstrapv1.Timeouts{
							ControlPlaneComponentHealthCheckSeconds: ptr.To[int32](5),
							KubernetesAPICallSeconds:                ptr.To[int32](7),
						},
						Patches: bootstrapv1.Patches{
							Directory: "/test/patches",
						},
						SkipPhases: []string{"skip-phase"},
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "name",
							KubeletExtraArgs: []bootstrapv1.Arg{
								{
									Name:  "v",
									Value: ptr.To("8"),
								},
							},
						},
						ControlPlane: &bootstrapv1.JoinControlPlane{
							LocalAPIEndpoint: bootstrapv1.APIEndpoint{
								AdvertiseAddress: "1.2.3.4",
								BindPort:         6443,
							},
						},
						CACertPath: "/tmp/cacert", // This field doesn't exist in InitConfiguration, so it should not lead to a rollout.
					},
				},
				Version: "v1.30.0",
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					// InitConfiguration will be converted to JoinConfiguration and then compared against the JoinConfiguration from KCP.
					InitConfiguration: bootstrapv1.InitConfiguration{
						Timeouts: bootstrapv1.Timeouts{
							ControlPlaneComponentHealthCheckSeconds: ptr.To[int32](5),
							KubernetesAPICallSeconds:                ptr.To[int32](7),
						},
						Patches: bootstrapv1.Patches{
							Directory: "/test/patches",
						},
						SkipPhases: []string{"skip-phase"},
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "name",
							KubeletExtraArgs: []bootstrapv1.Arg{
								{
									Name:  "v",
									Value: ptr.To("8"),
								},
							},
						},
						LocalAPIEndpoint: bootstrapv1.APIEndpoint{
							AdvertiseAddress: "1.2.3.4",
							BindPort:         6443,
						},
					},
				},
			},
		}
		reason, currentKubeadmConfig, desiredKubeadmConfig, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentKubeadmConfig).ToNot(BeNil())
		g.Expect(desiredKubeadmConfig).ToNot(BeNil())
		g.Expect(isKubeadmConfigForJoin(desiredKubeadmConfig)).To(BeTrue())
		g.Expect(match).To(BeTrue())
		g.Expect(reason).To(BeEmpty())
	})
	t.Run("returns true if JoinConfiguration is not equal, but InitConfiguration is", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: bootstrapv1.ClusterConfiguration{},
					InitConfiguration: bootstrapv1.InitConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "name",
						},
					},
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "Different name",
						},
					},
				},
				Version: "v1.30.0",
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					InitConfiguration: bootstrapv1.InitConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "name",
						},
					},
				},
			},
		}
		reason, currentKubeadmConfig, desiredKubeadmConfig, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentKubeadmConfig).ToNot(BeNil())
		g.Expect(desiredKubeadmConfig).ToNot(BeNil())
		g.Expect(isKubeadmConfigForJoin(desiredKubeadmConfig)).To(BeTrue())
		g.Expect(match).To(BeTrue())
		g.Expect(reason).To(BeEmpty())
	})
	t.Run("returns false if InitConfiguration is NOT equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: bootstrapv1.ClusterConfiguration{},
					InitConfiguration: bootstrapv1.InitConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "A new name", // This is a change
						},
					},
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "A new name", // This is a change
						},
					},
				},
				Version: "v1.30.0",
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					InitConfiguration: bootstrapv1.InitConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "An old name", // This is a change
						},
					},
				},
			},
		}
		reason, currentKubeadmConfig, desiredKubeadmConfig, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentKubeadmConfig).ToNot(BeNil())
		g.Expect(desiredKubeadmConfig).ToNot(BeNil())
		g.Expect(isKubeadmConfigForJoin(desiredKubeadmConfig)).To(BeTrue())
		g.Expect(match).To(BeFalse())
		g.Expect(reason).To(BeComparableTo(`Machine KubeadmConfig is outdated: diff: &v1beta2.KubeadmConfigSpec{
    ClusterConfiguration: {},
    InitConfiguration: v1beta2.InitConfiguration{
      BootstrapTokens: nil,
      NodeRegistration: v1beta2.NodeRegistrationOptions{
-       Name:      "An old name",
+       Name:      "A new name",
        CRISocket: "",
        Taints:    nil,
        ... // 4 identical fields
      },
      LocalAPIEndpoint: {},
      SkipPhases:       nil,
      ... // 2 identical fields
    },
    JoinConfiguration: {NodeRegistration: {ImagePullPolicy: "IfNotPresent"}},
    Files:             nil,
    ... // 10 identical fields
  }`))
	})
	t.Run("returns true if JoinConfiguration is equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    bootstrapv1.InitConfiguration{},
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "A new name",
						},
					},
				},
				Version: "v1.30.0",
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "A new name",
						},
					},
				},
			},
		}
		reason, currentKubeadmConfig, desiredKubeadmConfig, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentKubeadmConfig).ToNot(BeNil())
		g.Expect(desiredKubeadmConfig).ToNot(BeNil())
		g.Expect(isKubeadmConfigForJoin(desiredKubeadmConfig)).To(BeTrue())
		g.Expect(match).To(BeTrue())
		g.Expect(reason).To(BeEmpty())
	})
	t.Run("returns true if JoinConfiguration is equal apart from discovery", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    bootstrapv1.InitConfiguration{},
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "A new name",
						},
						// Discovery gets removed because Discovery is not relevant for the rollout decision.
						Discovery: bootstrapv1.Discovery{TLSBootstrapToken: "aaa"},
					},
				},
				Version: "v1.30.0",
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "A new name",
						},
						// Discovery gets removed because Discovery is not relevant for the rollout decision.
						Discovery: bootstrapv1.Discovery{TLSBootstrapToken: "bbb"},
					},
				},
			},
		}
		reason, currentKubeadmConfig, desiredKubeadmConfig, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentKubeadmConfig).ToNot(BeNil())
		g.Expect(desiredKubeadmConfig).ToNot(BeNil())
		g.Expect(isKubeadmConfigForJoin(desiredKubeadmConfig)).To(BeTrue())
		g.Expect(match).To(BeTrue())
		g.Expect(reason).To(BeEmpty())
	})
	t.Run("returns true if JoinConfiguration is equal apart from JoinControlPlane", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    bootstrapv1.InitConfiguration{},
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "A new name",
						},
						ControlPlane: nil, // Control plane configuration missing in KCP
					},
				},
				Version: "v1.30.0",
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "A new name",
						},
						ControlPlane: &bootstrapv1.JoinControlPlane{}, // Machine gets a default JoinConfiguration.ControlPlane from CABPK
					},
				},
			},
		}
		reason, currentKubeadmConfig, desiredKubeadmConfig, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentKubeadmConfig).ToNot(BeNil())
		g.Expect(desiredKubeadmConfig).ToNot(BeNil())
		g.Expect(isKubeadmConfigForJoin(desiredKubeadmConfig)).To(BeTrue())
		g.Expect(match).To(BeTrue())
		g.Expect(reason).To(BeEmpty())
	})
	t.Run("returns false if JoinConfiguration is not equal, and InitConfiguration is also not equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: bootstrapv1.ClusterConfiguration{},
					InitConfiguration: bootstrapv1.InitConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "Different name",
						},
					},
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "Different name",
						},
					},
				},
				Version: "v1.30.0",
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					InitConfiguration: bootstrapv1.InitConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "name",
						},
					},
				},
			},
		}
		reason, currentKubeadmConfig, desiredKubeadmConfig, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentKubeadmConfig).ToNot(BeNil())
		g.Expect(desiredKubeadmConfig).ToNot(BeNil())
		g.Expect(isKubeadmConfigForJoin(desiredKubeadmConfig)).To(BeTrue())
		g.Expect(match).To(BeFalse())
		g.Expect(reason).To(Equal(`Machine KubeadmConfig is outdated: diff: &v1beta2.KubeadmConfigSpec{
    ClusterConfiguration: {},
    InitConfiguration: v1beta2.InitConfiguration{
      BootstrapTokens: nil,
      NodeRegistration: v1beta2.NodeRegistrationOptions{
-       Name:      "name",
+       Name:      "Different name",
        CRISocket: "",
        Taints:    nil,
        ... // 4 identical fields
      },
      LocalAPIEndpoint: {},
      SkipPhases:       nil,
      ... // 2 identical fields
    },
    JoinConfiguration: {NodeRegistration: {ImagePullPolicy: "IfNotPresent"}},
    Files:             nil,
    ... // 10 identical fields
  }`))
	})
	t.Run("returns false if JoinConfiguration has other differences in ControlPlane", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    bootstrapv1.InitConfiguration{},
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "name",
						},
						ControlPlane: nil, // Control plane configuration missing in KCP
					},
				},
				Version: "v1.30.0",
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "name",
						},
						ControlPlane: &bootstrapv1.JoinControlPlane{
							LocalAPIEndpoint: bootstrapv1.APIEndpoint{
								AdvertiseAddress: "1.2.3.4",
								BindPort:         6443,
							},
						},
					},
				},
			},
		}
		reason, currentKubeadmConfig, desiredKubeadmConfig, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentKubeadmConfig).ToNot(BeNil())
		g.Expect(desiredKubeadmConfig).ToNot(BeNil())
		g.Expect(isKubeadmConfigForJoin(desiredKubeadmConfig)).To(BeTrue())
		g.Expect(match).To(BeFalse())
		g.Expect(reason).To(Equal(`Machine KubeadmConfig is outdated: diff: &v1beta2.KubeadmConfigSpec{
    ClusterConfiguration: {},
    InitConfiguration:    {NodeRegistration: {ImagePullPolicy: "IfNotPresent"}},
    JoinConfiguration: v1beta2.JoinConfiguration{
      NodeRegistration: {Name: "name", ImagePullPolicy: "IfNotPresent"},
      CACertPath:       "",
      Discovery:        {},
-     ControlPlane: &v1beta2.JoinControlPlane{
-       LocalAPIEndpoint: v1beta2.APIEndpoint{AdvertiseAddress: "1.2.3.4", BindPort: 6443},
-     },
+     ControlPlane: nil,
      SkipPhases:   nil,
      Patches:      {},
      Timeouts:     {},
    },
    Files:     nil,
    DiskSetup: {},
    ... // 9 identical fields
  }`))
	})
	t.Run("returns false if JoinConfiguration is NOT equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    bootstrapv1.InitConfiguration{},
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "A new name", // This is a change
						},
					},
				},
				Version: "v1.30.0",
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "An old name", // This is a change
						},
					},
				},
			},
		}
		reason, currentKubeadmConfig, desiredKubeadmConfig, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentKubeadmConfig).ToNot(BeNil())
		g.Expect(desiredKubeadmConfig).ToNot(BeNil())
		g.Expect(isKubeadmConfigForJoin(desiredKubeadmConfig)).To(BeTrue())
		g.Expect(match).To(BeFalse())
		g.Expect(reason).To(BeComparableTo(`Machine KubeadmConfig is outdated: diff: &v1beta2.KubeadmConfigSpec{
    ClusterConfiguration: {},
    InitConfiguration:    {NodeRegistration: {ImagePullPolicy: "IfNotPresent"}},
    JoinConfiguration: v1beta2.JoinConfiguration{
      NodeRegistration: v1beta2.NodeRegistrationOptions{
-       Name:      "An old name",
+       Name:      "A new name",
        CRISocket: "",
        Taints:    nil,
        ... // 4 identical fields
      },
      CACertPath: "",
      Discovery:  {},
      ... // 4 identical fields
    },
    Files:     nil,
    DiskSetup: {},
    ... // 9 identical fields
  }`))
	})
	t.Run("returns false if JoinConfiguration is NOT equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    bootstrapv1.InitConfiguration{},
					// JoinConfiguration not set anymore.
				},
				Version: "v1.30.0",
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "An old name", // This is a change
						},
					},
				},
			},
		}
		reason, currentKubeadmConfig, desiredKubeadmConfig, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentKubeadmConfig).ToNot(BeNil())
		g.Expect(desiredKubeadmConfig).ToNot(BeNil())
		// Can't check if desiredKubeadmConfig is for join because the test case is that JoinConfiguration is not set anymore.
		g.Expect(match).To(BeFalse())
		g.Expect(reason).To(BeComparableTo(`Machine KubeadmConfig is outdated: diff: &v1beta2.KubeadmConfigSpec{
    ClusterConfiguration: {},
    InitConfiguration:    {NodeRegistration: {ImagePullPolicy: "IfNotPresent"}},
    JoinConfiguration: v1beta2.JoinConfiguration{
      NodeRegistration: v1beta2.NodeRegistrationOptions{
-       Name:      "An old name",
+       Name:      "",
        CRISocket: "",
        Taints:    nil,
        ... // 4 identical fields
      },
      CACertPath: "",
      Discovery:  {},
      ... // 4 identical fields
    },
    Files:     nil,
    DiskSetup: {},
    ... // 9 identical fields
  }`))
	})
	t.Run("returns true if only omittable configurations are not equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: bootstrapv1.ClusterConfiguration{
						FeatureGates: map[string]bool{}, // This is a change, but it is an omittable field
					},
					InitConfiguration: bootstrapv1.InitConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name:             "name",
							KubeletExtraArgs: []bootstrapv1.Arg{},
						},
					},
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name:             "name",
							KubeletExtraArgs: []bootstrapv1.Arg{},
						},
					},
					Files: []bootstrapv1.File{}, // This is a change, but it is an omittable field and the diff between nil and empty array is not relevant.
				},
				Version: "v1.30.0",
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    bootstrapv1.InitConfiguration{},
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "name",
						},
					},
				},
			},
		}
		reason, currentKubeadmConfig, desiredKubeadmConfig, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentKubeadmConfig).ToNot(BeNil())
		g.Expect(desiredKubeadmConfig).ToNot(BeNil())
		g.Expect(isKubeadmConfigForJoin(desiredKubeadmConfig)).To(BeTrue())
		g.Expect(match).To(BeTrue())
		g.Expect(reason).To(BeEmpty())
	})
	t.Run("returns true if KubeadmConfig is equal apart from defaulted format field", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					Format: bootstrapv1.CloudConfig,
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "name",
						},
					},
				},
				Version: "v1.30.0",
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Format: "",
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "name",
						},
					},
				},
			},
		}
		reason, currentKubeadmConfig, desiredKubeadmConfig, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentKubeadmConfig).ToNot(BeNil())
		g.Expect(desiredKubeadmConfig).ToNot(BeNil())
		g.Expect(isKubeadmConfigForJoin(desiredKubeadmConfig)).To(BeTrue())
		g.Expect(match).To(BeTrue())
		g.Expect(reason).To(BeEmpty())
	})
	t.Run("returns false if KubeadmConfig is not equal (other configurations)", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    bootstrapv1.InitConfiguration{},
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "name",
						},
					},
					Files: []bootstrapv1.File{{Path: "/tmp/foo"}}, // This is a change
				},
				Version: "v1.30.0",
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "name",
						},
					},
				},
			},
		}
		reason, currentKubeadmConfig, desiredKubeadmConfig, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentKubeadmConfig).ToNot(BeNil())
		g.Expect(desiredKubeadmConfig).ToNot(BeNil())
		g.Expect(isKubeadmConfigForJoin(desiredKubeadmConfig)).To(BeTrue())
		g.Expect(match).To(BeFalse())
		g.Expect(reason).To(BeComparableTo(`Machine KubeadmConfig is outdated: diff: &v1beta2.KubeadmConfigSpec{
    ClusterConfiguration: {},
    InitConfiguration:    {NodeRegistration: {ImagePullPolicy: "IfNotPresent"}},
    JoinConfiguration:    {NodeRegistration: {Name: "name", ImagePullPolicy: "IfNotPresent"}},
-   Files:                nil,
+   Files:                []v1beta2.File{{Path: "/tmp/foo"}},
    DiskSetup:            {},
    Mounts:               nil,
    ... // 8 identical fields
  }`))
	})
	t.Run("should match on labels and annotations", func(t *testing.T) {
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
					ObjectMeta: clusterv1.ObjectMeta{
						Annotations: map[string]string{
							"test": "annotation",
						},
						Labels: map[string]string{
							"test": "labels",
						},
					},
				},
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    bootstrapv1.InitConfiguration{},
					JoinConfiguration:    bootstrapv1.JoinConfiguration{},
				},
				Version: "v1.30.0",
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: bootstrapv1.JoinConfiguration{},
				},
			},
		}

		t.Run("by returning true if neither labels or annotations match", func(t *testing.T) {
			g := NewWithT(t)
			machineConfigs[m.Name].Annotations = nil
			machineConfigs[m.Name].Labels = nil
			reason, _, _, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(match).To(BeTrue())
			g.Expect(reason).To(BeEmpty())
		})

		t.Run("by returning true if only labels don't match", func(t *testing.T) {
			g := NewWithT(t)
			machineConfigs[m.Name].Annotations = kcp.Spec.MachineTemplate.ObjectMeta.Annotations
			machineConfigs[m.Name].Labels = nil
			reason, _, _, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(match).To(BeTrue())
			g.Expect(reason).To(BeEmpty())
		})

		t.Run("by returning true if only annotations don't match", func(t *testing.T) {
			g := NewWithT(t)
			machineConfigs[m.Name].Annotations = nil
			machineConfigs[m.Name].Labels = kcp.Spec.MachineTemplate.ObjectMeta.Labels
			reason, _, _, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(match).To(BeTrue())
			g.Expect(reason).To(BeEmpty())
		})

		t.Run("by returning true if both labels and annotations match", func(t *testing.T) {
			g := NewWithT(t)
			machineConfigs[m.Name].Labels = kcp.Spec.MachineTemplate.ObjectMeta.Labels
			machineConfigs[m.Name].Annotations = kcp.Spec.MachineTemplate.ObjectMeta.Annotations
			reason, _, _, match, err := matchesKubeadmConfig(machineConfigs, kcp, &clusterv1.Cluster{}, m)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(match).To(BeTrue())
			g.Expect(reason).To(BeEmpty())
		})
	})
}

func TestMatchesInfraMachine(t *testing.T) {
	t.Run("returns true if machine not found", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{}
		machine := &clusterv1.Machine{
			Spec: clusterv1.MachineSpec{
				InfrastructureRef: clusterv1.ContractVersionedObjectReference{
					Kind:     "KubeadmConfig",
					Name:     "test",
					APIGroup: bootstrapv1.GroupVersion.Group,
				},
			},
		}
		reason, _, _, match, err := matchesInfraMachine(t.Context(), nil, map[string]*unstructured.Unstructured{}, kcp, &clusterv1.Cluster{}, machine)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeTrue())
		g.Expect(reason).To(BeEmpty())
	})

	t.Run("matches labels or annotations", func(t *testing.T) {
		kcp := &controlplanev1.KubeadmControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
			},
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
					ObjectMeta: clusterv1.ObjectMeta{
						Annotations: map[string]string{
							"test": "annotation",
						},
						Labels: map[string]string{
							"test": "labels",
						},
					},
					Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
						InfrastructureRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: builder.InfrastructureGroupVersion.Group,
							Kind:     builder.TestInfrastructureMachineTemplateKind,
							Name:     "infra-machine-template1",
						},
					},
				},
			},
		}

		infraMachineTemplate := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": builder.InfrastructureGroupVersion.String(),
				"kind":       builder.TestInfrastructureMachineTemplateKind,
				"metadata": map[string]interface{}{
					"name":      "infra-machine-template1",
					"namespace": "default",
				},
			},
		}

		m := &clusterv1.Machine{
			Spec: clusterv1.MachineSpec{
				InfrastructureRef: clusterv1.ContractVersionedObjectReference{
					APIGroup: builder.InfrastructureGroupVersion.Group,
					Kind:     builder.TestInfrastructureMachineKind,
					Name:     "infra-config1",
				},
			},
		}

		infraConfigs := map[string]*unstructured.Unstructured{
			m.Name: {
				Object: map[string]interface{}{
					"apiVersion": builder.InfrastructureGroupVersion.String(),
					"kind":       builder.TestInfrastructureMachineKind,
					"metadata": map[string]interface{}{
						"name":      "infra-config1",
						"namespace": "default",
					},
				},
			},
		}
		scheme := runtime.NewScheme()
		_ = apiextensionsv1.AddToScheme(scheme)
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(builder.TestInfrastructureMachineTemplateCRD, infraMachineTemplate).Build()

		t.Run("by returning true if annotations don't exist", func(t *testing.T) {
			g := NewWithT(t)
			infraConfigs[m.Name].SetAnnotations(map[string]string{})
			reason, _, _, match, err := matchesInfraMachine(t.Context(), c, infraConfigs, kcp, &clusterv1.Cluster{}, m)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(match).To(BeTrue())
			g.Expect(reason).To(BeEmpty())
		})

		t.Run("by returning false if neither Name nor GroupKind matches", func(t *testing.T) {
			g := NewWithT(t)
			infraConfigs[m.Name].SetAnnotations(map[string]string{
				clusterv1.TemplateClonedFromNameAnnotation:      "different-infra-machine-template1",
				clusterv1.TemplateClonedFromGroupKindAnnotation: "DifferentTestInfrastructureMachineTemplate.infrastructure.cluster.x-k8s.io",
			})
			reason, _, _, match, err := matchesInfraMachine(t.Context(), c, infraConfigs, kcp, &clusterv1.Cluster{}, m)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(match).To(BeFalse())
			g.Expect(reason).To(Equal("Infrastructure template on KCP rotated from DifferentTestInfrastructureMachineTemplate.infrastructure.cluster.x-k8s.io different-infra-machine-template1 to TestInfrastructureMachineTemplate.infrastructure.cluster.x-k8s.io infra-machine-template1"))
		})

		t.Run("by returning false if only GroupKind matches", func(t *testing.T) {
			g := NewWithT(t)
			infraConfigs[m.Name].SetAnnotations(map[string]string{
				clusterv1.TemplateClonedFromNameAnnotation:      "different-infra-machine-template1",
				clusterv1.TemplateClonedFromGroupKindAnnotation: "TestInfrastructureMachineTemplate.infrastructure.cluster.x-k8s.io",
			})
			reason, _, _, match, err := matchesInfraMachine(t.Context(), c, infraConfigs, kcp, &clusterv1.Cluster{}, m)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(match).To(BeFalse())
			g.Expect(reason).To(Equal("Infrastructure template on KCP rotated from TestInfrastructureMachineTemplate.infrastructure.cluster.x-k8s.io different-infra-machine-template1 to TestInfrastructureMachineTemplate.infrastructure.cluster.x-k8s.io infra-machine-template1"))
		})

		t.Run("by returning false if only Name matches", func(t *testing.T) {
			g := NewWithT(t)
			infraConfigs[m.Name].SetAnnotations(map[string]string{
				clusterv1.TemplateClonedFromNameAnnotation:      "infra-machine-template1",
				clusterv1.TemplateClonedFromGroupKindAnnotation: "DifferentTestInfrastructureMachineTemplate.infrastructure.cluster.x-k8s.io",
			})
			reason, _, _, match, err := matchesInfraMachine(t.Context(), c, infraConfigs, kcp, &clusterv1.Cluster{}, m)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(match).To(BeFalse())
			g.Expect(reason).To(Equal("Infrastructure template on KCP rotated from DifferentTestInfrastructureMachineTemplate.infrastructure.cluster.x-k8s.io infra-machine-template1 to TestInfrastructureMachineTemplate.infrastructure.cluster.x-k8s.io infra-machine-template1"))
		})

		t.Run("by returning true if both Name and GroupKind match", func(t *testing.T) {
			g := NewWithT(t)
			infraConfigs[m.Name].SetAnnotations(map[string]string{
				clusterv1.TemplateClonedFromNameAnnotation:      "infra-machine-template1",
				clusterv1.TemplateClonedFromGroupKindAnnotation: "TestInfrastructureMachineTemplate.infrastructure.cluster.x-k8s.io",
			})
			reason, _, _, match, err := matchesInfraMachine(t.Context(), c, infraConfigs, kcp, &clusterv1.Cluster{}, m)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(reason).To(BeEmpty())
			g.Expect(match).To(BeTrue())
		})
	})
}

func TestUpToDate(t *testing.T) {
	reconciliationTime := metav1.Now()

	defaultKcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "kcp",
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Replicas: nil,
			Version:  "v1.30.0",
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: builder.InfrastructureGroupVersion.Group,
						Kind:     builder.TestInfrastructureMachineTemplateKind,
						Name:     "infra-machine-template1",
					},
				},
			},
			KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{
					CertificatesDir: "foo",
				},
			},
			Rollout: controlplanev1.KubeadmControlPlaneRolloutSpec{
				Before: controlplanev1.KubeadmControlPlaneRolloutBeforeSpec{
					CertificatesExpiryDays: 60, // rollout if certificates will expire in less then 60 days.
				},
				After: metav1.Time{Time: reconciliationTime.Add(10 * 24 * time.Hour)}, // rollout 10 days from now.
			},
		},
	}

	infraMachineTemplate1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": builder.InfrastructureGroupVersion.String(),
			"kind":       builder.TestInfrastructureMachineTemplateKind,
			"metadata": map[string]interface{}{
				"name":      "infra-machine-template1",
				"namespace": "default",
			},
		},
	}
	infraMachineTemplate2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": builder.InfrastructureGroupVersion.String(),
			"kind":       builder.TestInfrastructureMachineTemplateKind,
			"metadata": map[string]interface{}{
				"name":      "infra-machine-template2",
				"namespace": "default",
			},
		},
	}

	defaultMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.Time{Time: reconciliationTime.Add(-2 * 24 * time.Hour)}, // two days ago.
		},
		Spec: clusterv1.MachineSpec{
			Version: "v1.30.0",
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: clusterv1.ContractVersionedObjectReference{
					APIGroup: bootstrapv1.GroupVersion.Group,
					Kind:     "KubeadmConfig",
					Name:     "boostrap-config1",
				},
			},
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: clusterv1.GroupVersionInfrastructure.Group,
				Kind:     "TestInfrastructureMachine",
				Name:     "infra-machine1",
			},
		},
		Status: clusterv1.MachineStatus{
			CertificatesExpiryDate: metav1.Time{Time: reconciliationTime.Add(100 * 24 * time.Hour)}, // certificates will expire in 100 days from now.
		},
	}

	defaultInfraConfigs := map[string]*unstructured.Unstructured{
		defaultMachine.Name: {
			Object: map[string]interface{}{
				"kind":       builder.TestInfrastructureMachineKind,
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":      "infra-machine1",
					"namespace": "default",
					"annotations": map[string]interface{}{
						"cluster.x-k8s.io/cloned-from-name":      "infra-machine-template1",
						"cluster.x-k8s.io/cloned-from-groupkind": builder.InfrastructureGroupVersion.WithKind(builder.TestInfrastructureMachineTemplateKind).GroupKind().String(),
					},
				},
			},
		},
	}

	defaultMachineConfigs := map[string]*bootstrapv1.KubeadmConfig{
		defaultMachine.Name: {
			ObjectMeta: metav1.ObjectMeta{
				Name: "boostrap-config1",
			},
			Spec: bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{
					CertificatesDir: "foo",
				},
				InitConfiguration: bootstrapv1.InitConfiguration{}, // first control-plane
			},
		},
	}

	tests := []struct {
		name                           string
		kcp                            *controlplanev1.KubeadmControlPlane
		machine                        *clusterv1.Machine
		infraConfigs                   map[string]*unstructured.Unstructured
		machineConfigs                 map[string]*bootstrapv1.KubeadmConfig
		expectUptoDate                 bool
		expectEligibleForInPlaceUpdate bool
		expectLogMessages              []string
		expectConditionMessages        []string
	}{
		{
			name:                           "machine up-to-date",
			kcp:                            defaultKcp,
			machine:                        defaultMachine,
			infraConfigs:                   defaultInfraConfigs,
			machineConfigs:                 defaultMachineConfigs,
			expectUptoDate:                 true,
			expectEligibleForInPlaceUpdate: false,
			expectLogMessages:              nil,
			expectConditionMessages:        nil,
		},
		{
			name: "certificate are expiring soon",
			kcp: func() *controlplanev1.KubeadmControlPlane {
				kcp := defaultKcp.DeepCopy()
				kcp.Spec.Rollout.Before.CertificatesExpiryDays = 150 // rollout if certificates will expire in less then 150 days.
				return kcp
			}(),
			machine:                        defaultMachine, // certificates will expire in 100 days from now.
			infraConfigs:                   defaultInfraConfigs,
			machineConfigs:                 defaultMachineConfigs,
			expectUptoDate:                 false,
			expectEligibleForInPlaceUpdate: false,
			expectLogMessages:              []string{"certificates will expire soon, rolloutBefore expired"},
			expectConditionMessages:        []string{"Certificates will expire soon"},
		},
		{
			name: "rollout after expired",
			kcp: func() *controlplanev1.KubeadmControlPlane {
				kcp := defaultKcp.DeepCopy()
				kcp.Spec.Rollout.After = metav1.Time{Time: reconciliationTime.Add(-1 * 24 * time.Hour)} // one day ago
				return kcp
			}(),
			machine:                        defaultMachine, // created two days ago
			infraConfigs:                   defaultInfraConfigs,
			machineConfigs:                 defaultMachineConfigs,
			expectUptoDate:                 false,
			expectEligibleForInPlaceUpdate: false,
			expectLogMessages:              []string{"rolloutAfter expired"},
			expectConditionMessages:        []string{"KubeadmControlPlane spec.rolloutAfter expired"},
		},
		{
			name: "kubernetes version does not match",
			kcp: func() *controlplanev1.KubeadmControlPlane {
				kcp := defaultKcp.DeepCopy()
				kcp.Spec.Version = "v1.30.2"
				return kcp
			}(),
			machine: func() *clusterv1.Machine {
				machine := defaultMachine.DeepCopy()
				machine.Spec.Version = "v1.30.0"
				return machine
			}(),
			infraConfigs:                   defaultInfraConfigs,
			machineConfigs:                 defaultMachineConfigs,
			expectUptoDate:                 false,
			expectEligibleForInPlaceUpdate: true,
			expectLogMessages:              []string{"Machine version \"v1.30.0\" is not equal to KCP version \"v1.30.2\""},
			expectConditionMessages:        []string{"Version v1.30.0, v1.30.2 required"},
		},
		{
			name: "kubernetes version does not match + delete annotation",
			kcp: func() *controlplanev1.KubeadmControlPlane {
				kcp := defaultKcp.DeepCopy()
				kcp.Spec.Version = "v1.30.2"
				return kcp
			}(),
			machine: func() *clusterv1.Machine {
				machine := defaultMachine.DeepCopy()
				machine.Spec.Version = "v1.30.0"
				machine.Annotations = map[string]string{
					clusterv1.DeleteMachineAnnotation: "",
				}
				return machine
			}(),
			infraConfigs:                   defaultInfraConfigs,
			machineConfigs:                 defaultMachineConfigs,
			expectUptoDate:                 false,
			expectEligibleForInPlaceUpdate: false, // Not eligible for in-place update because of delete annotation.
			expectLogMessages:              []string{"Machine version \"v1.30.0\" is not equal to KCP version \"v1.30.2\""},
			expectConditionMessages:        []string{"Version v1.30.0, v1.30.2 required"},
		},
		{
			name: "KubeadmConfig is not up-to-date",
			kcp: func() *controlplanev1.KubeadmControlPlane {
				kcp := defaultKcp.DeepCopy()
				kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.CertificatesDir = "bar"
				return kcp
			}(),
			machine:                        defaultMachine, // was created with cluster name "foo"
			infraConfigs:                   defaultInfraConfigs,
			machineConfigs:                 defaultMachineConfigs,
			expectUptoDate:                 false,
			expectEligibleForInPlaceUpdate: true,
			expectLogMessages: []string{`Machine KubeadmConfig is outdated: diff: &v1beta2.KubeadmConfigSpec{
    ClusterConfiguration: v1beta2.ClusterConfiguration{
      ... // 4 identical fields
      Scheduler:       {},
      DNS:             {},
-     CertificatesDir: "foo",
+     CertificatesDir: "bar",
      ImageRepository: "",
      FeatureGates:    nil,
      ... // 2 identical fields
    },
    InitConfiguration: {NodeRegistration: {ImagePullPolicy: "IfNotPresent"}},
    JoinConfiguration: {NodeRegistration: {ImagePullPolicy: "IfNotPresent"}},
    ... // 11 identical fields
  }`},
			expectConditionMessages: []string{"KubeadmConfig is not up-to-date"},
		},
		{
			name: "InfraMachine is not up-to-date",
			kcp: func() *controlplanev1.KubeadmControlPlane {
				kcp := defaultKcp.DeepCopy()
				kcp.Spec.MachineTemplate.Spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference{
					APIGroup: builder.InfrastructureGroupVersion.Group,
					Kind:     builder.TestInfrastructureMachineTemplateKind,
					Name:     "infra-machine-template2",
				} // kcp moving to infra-machine-template2
				return kcp
			}(),
			machine:                        defaultMachine,
			infraConfigs:                   defaultInfraConfigs, // infra config cloned from infra-machine-template1
			machineConfigs:                 defaultMachineConfigs,
			expectUptoDate:                 false,
			expectEligibleForInPlaceUpdate: true,
			expectLogMessages: []string{"Infrastructure template on KCP rotated from " +
				"TestInfrastructureMachineTemplate.infrastructure.cluster.x-k8s.io infra-machine-template1 to " +
				"TestInfrastructureMachineTemplate.infrastructure.cluster.x-k8s.io infra-machine-template2"},
			expectConditionMessages: []string{"TestInfrastructureMachine is not up-to-date"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			scheme := runtime.NewScheme()
			_ = apiextensionsv1.AddToScheme(scheme)
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(builder.TestInfrastructureMachineTemplateCRD, infraMachineTemplate1, infraMachineTemplate2).Build()

			upToDate, res, err := UpToDate(t.Context(), c, &clusterv1.Cluster{}, tt.machine, tt.kcp, &reconciliationTime, tt.infraConfigs, tt.machineConfigs)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(upToDate).To(Equal(tt.expectUptoDate))
			g.Expect(res).ToNot(BeNil())
			g.Expect(res.EligibleForInPlaceUpdate).To(Equal(tt.expectEligibleForInPlaceUpdate))
			g.Expect(res.DesiredMachine).ToNot(BeNil())
			g.Expect(res.DesiredMachine.Spec.Version).To(Equal(tt.kcp.Spec.Version))
			g.Expect(res.CurrentInfraMachine).ToNot(BeNil())
			g.Expect(res.DesiredInfraMachine).ToNot(BeNil())
			g.Expect(res.CurrentKubeadmConfig).ToNot(BeNil())
			g.Expect(res.DesiredKubeadmConfig).ToNot(BeNil())
			if upToDate {
				g.Expect(res.LogMessages).To(BeEmpty())
				g.Expect(res.ConditionMessages).To(BeEmpty())
			} else {
				g.Expect(res.LogMessages).To(BeComparableTo(tt.expectLogMessages))
				g.Expect(res.ConditionMessages).To(Equal(tt.expectConditionMessages))
			}
		})
	}
}

func TestOmittableFieldsClusterConfiguration(t *testing.T) {
	tests := []struct {
		name string
		A    bootstrapv1.ClusterConfiguration
		B    bootstrapv1.ClusterConfiguration
	}{
		{
			name: "Test omittable fields",
			A: bootstrapv1.ClusterConfiguration{
				Etcd: bootstrapv1.Etcd{
					Local: bootstrapv1.LocalEtcd{
						DataDir:        "/var/lib/etcd", // Setting a field to avoid differences because empty object is omitted in one of the cases.
						ExtraArgs:      []bootstrapv1.Arg{},
						ExtraEnvs:      ptr.To([]bootstrapv1.EnvVar{}),
						ServerCertSANs: []string{},
						PeerCertSANs:   []string{},
					},
					External: bootstrapv1.ExternalEtcd{
						Endpoints: []string{},
					},
				},
				APIServer: bootstrapv1.APIServer{
					ExtraArgs:    []bootstrapv1.Arg{},
					ExtraVolumes: []bootstrapv1.HostPathMount{},
					ExtraEnvs:    ptr.To([]bootstrapv1.EnvVar{}),
					CertSANs:     []string{},
				},
				ControllerManager: bootstrapv1.ControllerManager{
					ExtraArgs:    []bootstrapv1.Arg{},
					ExtraVolumes: []bootstrapv1.HostPathMount{},
					ExtraEnvs:    ptr.To([]bootstrapv1.EnvVar{}),
				},
				Scheduler: bootstrapv1.Scheduler{
					ExtraArgs:    []bootstrapv1.Arg{},
					ExtraVolumes: []bootstrapv1.HostPathMount{},
					ExtraEnvs:    ptr.To([]bootstrapv1.EnvVar{}),
				},
				FeatureGates: map[string]bool{},
			},
			B: bootstrapv1.ClusterConfiguration{
				Etcd: bootstrapv1.Etcd{
					Local: bootstrapv1.LocalEtcd{
						DataDir:        "/var/lib/etcd", // Setting a field to avoid differences because empty object is omitted in one of the cases.
						ExtraArgs:      nil,
						ExtraEnvs:      nil,
						ServerCertSANs: nil,
						PeerCertSANs:   nil,
					},
					External: bootstrapv1.ExternalEtcd{
						// The field doesn't have omit empty. It also is required and has MinItems=1, so it will
						// never actually be nil or an empty array so that difference also won't trigger any rollouts.
						Endpoints: []string{},
					},
				},
				APIServer: bootstrapv1.APIServer{
					ExtraArgs:    nil,
					ExtraVolumes: nil,
					ExtraEnvs:    nil,
					CertSANs:     nil,
				},
				ControllerManager: bootstrapv1.ControllerManager{
					ExtraArgs:    nil,
					ExtraVolumes: nil,
					ExtraEnvs:    nil,
				},
				Scheduler: bootstrapv1.Scheduler{
					ExtraArgs:    nil,
					ExtraVolumes: nil,
					ExtraEnvs:    nil,
				},
				FeatureGates: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gotA, err := kubeadmtypes.MarshalClusterConfigurationForVersion(&tt.A, semver.MustParse("1.99.0"), &upstream.AdditionalData{}) // we want to test with latest kubeadm API version.
			g.Expect(err).ToNot(HaveOccurred())

			gotB, err := kubeadmtypes.MarshalClusterConfigurationForVersion(&tt.B, semver.MustParse("1.99.0"), &upstream.AdditionalData{}) // we want to test with latest kubeadm API version.
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(gotA).To(Equal(gotB), cmp.Diff(gotA, gotB))

			specA := &bootstrapv1.KubeadmConfigSpec{ClusterConfiguration: tt.A}
			specB := &bootstrapv1.KubeadmConfigSpec{ClusterConfiguration: tt.B}
			dropOmittableFields(specA)
			dropOmittableFields(specB)
			g.Expect(specA.ClusterConfiguration).To(BeComparableTo(specB.ClusterConfiguration))
		})
	}
}

func TestOmittableFieldsInitConfiguration(t *testing.T) {
	tests := []struct {
		name string
		A    bootstrapv1.InitConfiguration
		B    bootstrapv1.InitConfiguration
	}{
		{
			name: "Test omittable fields",
			A: bootstrapv1.InitConfiguration{
				BootstrapTokens: []bootstrapv1.BootstrapToken{},
				NodeRegistration: bootstrapv1.NodeRegistrationOptions{
					Taints:                ptr.To([]corev1.Taint{}),
					KubeletExtraArgs:      []bootstrapv1.Arg{},
					IgnorePreflightErrors: []string{},
				},
				SkipPhases: []string{},
			},
			B: bootstrapv1.InitConfiguration{
				BootstrapTokens: nil,
				NodeRegistration: bootstrapv1.NodeRegistrationOptions{
					Taints:                ptr.To([]corev1.Taint{}), // Special serialization, i.e. intentionally a pointer to a slice to preserve []
					KubeletExtraArgs:      nil,
					IgnorePreflightErrors: nil,
				},
				SkipPhases: nil,
			},
		},
		{
			name: "Test omittable fields in BootstrapToken",
			A: bootstrapv1.InitConfiguration{
				BootstrapTokens: []bootstrapv1.BootstrapToken{
					{
						Usages: []string{},
						Groups: []string{},
					},
				},
			},
			B: bootstrapv1.InitConfiguration{
				BootstrapTokens: []bootstrapv1.BootstrapToken{
					{
						Usages: nil,
						Groups: nil,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gotA, err := kubeadmtypes.MarshalInitConfigurationForVersion(&tt.A, semver.MustParse("1.99.0")) // we want to test with latest kubeadm API version.
			g.Expect(err).ToNot(HaveOccurred())

			gotB, err := kubeadmtypes.MarshalInitConfigurationForVersion(&tt.B, semver.MustParse("1.99.0")) // we want to test with latest kubeadm API version.
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(gotA).To(Equal(gotB), cmp.Diff(gotA, gotB))

			specA := &bootstrapv1.KubeadmConfigSpec{InitConfiguration: tt.A}
			specB := &bootstrapv1.KubeadmConfigSpec{InitConfiguration: tt.B}
			dropOmittableFields(specA)
			dropOmittableFields(specB)
			g.Expect(specA.InitConfiguration).To(BeComparableTo(specB.InitConfiguration))
		})
	}
}

func TestOmittableFieldsJoinConfiguration(t *testing.T) {
	tests := []struct {
		name string
		A    bootstrapv1.JoinConfiguration
		B    bootstrapv1.JoinConfiguration
	}{
		{
			name: "Test omittable fields",
			A: bootstrapv1.JoinConfiguration{
				NodeRegistration: bootstrapv1.NodeRegistrationOptions{
					Taints:                ptr.To([]corev1.Taint{}),
					KubeletExtraArgs:      []bootstrapv1.Arg{},
					IgnorePreflightErrors: []string{},
				},
				Discovery: bootstrapv1.Discovery{
					BootstrapToken: bootstrapv1.BootstrapTokenDiscovery{
						Token:        "token", // Setting a field to avoid differences because empty object is omitted in one of the cases.
						CACertHashes: []string{},
					},
					File: bootstrapv1.FileDiscovery{
						KubeConfigPath: "/tmp/kubeconfig", // Setting a field to avoid differences because empty object is omitted in one of the cases.
						KubeConfig: bootstrapv1.FileDiscoveryKubeConfig{
							Cluster: bootstrapv1.KubeConfigCluster{
								CertificateAuthorityData: []byte{},
							},
							User: bootstrapv1.KubeConfigUser{
								AuthProvider: bootstrapv1.KubeConfigAuthProvider{
									Config: map[string]string{},
								},
								Exec: bootstrapv1.KubeConfigAuthExec{
									Args: []string{},
									Env:  []bootstrapv1.KubeConfigAuthExecEnv{},
								},
							},
						},
					},
				},
				SkipPhases: []string{},
			},
			B: bootstrapv1.JoinConfiguration{
				NodeRegistration: bootstrapv1.NodeRegistrationOptions{
					Taints:                ptr.To([]corev1.Taint{}), // Special serialization, i.e. intentionally a pointer to a slice to preserve []
					KubeletExtraArgs:      nil,
					IgnorePreflightErrors: nil,
				},
				Discovery: bootstrapv1.Discovery{
					BootstrapToken: bootstrapv1.BootstrapTokenDiscovery{
						Token:        "token", // Setting a field to avoid differences because empty object is omitted in one of the cases.
						CACertHashes: nil,
					},
					File: bootstrapv1.FileDiscovery{
						KubeConfigPath: "/tmp/kubeconfig", // Setting a field to avoid differences because empty object is omitted in one of the cases.
						KubeConfig: bootstrapv1.FileDiscoveryKubeConfig{
							Cluster: bootstrapv1.KubeConfigCluster{
								CertificateAuthorityData: nil,
							},
							User: bootstrapv1.KubeConfigUser{
								AuthProvider: bootstrapv1.KubeConfigAuthProvider{
									Config: nil,
								},
								Exec: bootstrapv1.KubeConfigAuthExec{
									Args: nil,
									Env:  nil,
								},
							},
						},
					},
				},
				SkipPhases: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gotA, err := kubeadmtypes.MarshalJoinConfigurationForVersion(&tt.A, semver.MustParse("1.99.0")) // we want to test with latest kubeadm API version.
			g.Expect(err).ToNot(HaveOccurred())

			gotB, err := kubeadmtypes.MarshalJoinConfigurationForVersion(&tt.B, semver.MustParse("1.99.0")) // we want to test with latest kubeadm API version.
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(gotA).To(Equal(gotB), cmp.Diff(gotA, gotB))

			specA := &bootstrapv1.KubeadmConfigSpec{JoinConfiguration: tt.A}
			specB := &bootstrapv1.KubeadmConfigSpec{JoinConfiguration: tt.B}
			dropOmittableFields(specA)
			dropOmittableFields(specB)
			g.Expect(specA.JoinConfiguration).To(BeComparableTo(specB.JoinConfiguration))
		})
	}
}
