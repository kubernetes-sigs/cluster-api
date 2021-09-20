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

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
)

func TestMatchClusterConfiguration(t *testing.T) {
	t.Run("machine without the ClusterConfiguration annotation should match (not enough information to make a decision)", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{}
		m := &clusterv1.Machine{}
		g.Expect(matchClusterConfiguration(kcp, m)).To(BeTrue())
	})
	t.Run("machine without an invalid ClusterConfiguration annotation should not match (only solution is to rollout)", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					controlplanev1.KubeadmClusterConfigurationAnnotation: "$|^^_",
				},
			},
		}
		g.Expect(matchClusterConfiguration(kcp, m)).To(BeFalse())
	})
	t.Run("Return true if cluster configuration matches", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
						ClusterName: "foo",
					},
				},
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					controlplanev1.KubeadmClusterConfigurationAnnotation: "{\n  \"clusterName\": \"foo\"\n}",
				},
			},
		}
		g.Expect(matchClusterConfiguration(kcp, m)).To(BeTrue())
	})
	t.Run("Return false if cluster configuration does not match", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
						ClusterName: "foo",
					},
				},
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					controlplanev1.KubeadmClusterConfigurationAnnotation: "{\n  \"clusterName\": \"bar\"\n}",
				},
			},
		}
		g.Expect(matchClusterConfiguration(kcp, m)).To(BeFalse())
	})
	t.Run("Return true if cluster configuration is nil (special case)", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{},
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					controlplanev1.KubeadmClusterConfigurationAnnotation: "null",
				},
			},
		}
		g.Expect(matchClusterConfiguration(kcp, m)).To(BeTrue())
	})
}

func TestGetAdjustedKcpConfig(t *testing.T) {
	t.Run("if the machine is the first control plane, kcp config should get InitConfiguration", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					InitConfiguration: &bootstrapv1.InitConfiguration{},
					JoinConfiguration: &bootstrapv1.JoinConfiguration{},
				},
			},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			Spec: bootstrapv1.KubeadmConfigSpec{
				InitConfiguration: &bootstrapv1.InitConfiguration{}, // first control-plane
			},
		}
		kcpConfig := getAdjustedKcpConfig(kcp, machineConfig)
		g.Expect(kcpConfig.InitConfiguration).ToNot(BeNil())
		g.Expect(kcpConfig.JoinConfiguration).To(BeNil())
	})
	t.Run("if the machine is a joining control plane, kcp config should get JoinConfiguration", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					InitConfiguration: &bootstrapv1.InitConfiguration{},
					JoinConfiguration: &bootstrapv1.JoinConfiguration{},
				},
			},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			Spec: bootstrapv1.KubeadmConfigSpec{
				JoinConfiguration: &bootstrapv1.JoinConfiguration{}, // joining control-plane
			},
		}
		kcpConfig := getAdjustedKcpConfig(kcp, machineConfig)
		g.Expect(kcpConfig.InitConfiguration).To(BeNil())
		g.Expect(kcpConfig.JoinConfiguration).ToNot(BeNil())
	})
}

func TestCleanupConfigFields(t *testing.T) {
	t.Run("ClusterConfiguration gets removed from KcpConfig and MachineConfig", func(t *testing.T) {
		g := NewWithT(t)
		kcpConfig := &bootstrapv1.KubeadmConfigSpec{
			ClusterConfiguration: &bootstrapv1.ClusterConfiguration{},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			Spec: bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{},
			},
		}
		cleanupConfigFields(kcpConfig, machineConfig)
		g.Expect(kcpConfig.ClusterConfiguration).To(BeNil())
		g.Expect(machineConfig.Spec.ClusterConfiguration).To(BeNil())
	})
	t.Run("JoinConfiguration gets removed from MachineConfig if it was not derived by KCPConfig", func(t *testing.T) {
		g := NewWithT(t)
		kcpConfig := &bootstrapv1.KubeadmConfigSpec{
			JoinConfiguration: nil, // KCP not providing a JoinConfiguration
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			Spec: bootstrapv1.KubeadmConfigSpec{
				JoinConfiguration: &bootstrapv1.JoinConfiguration{}, // Machine gets a default JoinConfiguration from CABPK
			},
		}
		cleanupConfigFields(kcpConfig, machineConfig)
		g.Expect(kcpConfig.JoinConfiguration).To(BeNil())
		g.Expect(machineConfig.Spec.JoinConfiguration).To(BeNil())
	})
	t.Run("JoinConfiguration.Discovery gets removed because it is not relevant for compare", func(t *testing.T) {
		g := NewWithT(t)
		kcpConfig := &bootstrapv1.KubeadmConfigSpec{
			JoinConfiguration: &bootstrapv1.JoinConfiguration{
				Discovery: bootstrapv1.Discovery{TLSBootstrapToken: "aaa"},
			},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			Spec: bootstrapv1.KubeadmConfigSpec{
				JoinConfiguration: &bootstrapv1.JoinConfiguration{
					Discovery: bootstrapv1.Discovery{TLSBootstrapToken: "aaa"},
				},
			},
		}
		cleanupConfigFields(kcpConfig, machineConfig)
		g.Expect(kcpConfig.JoinConfiguration.Discovery).To(Equal(bootstrapv1.Discovery{}))
		g.Expect(machineConfig.Spec.JoinConfiguration.Discovery).To(Equal(bootstrapv1.Discovery{}))
	})
	t.Run("JoinConfiguration.ControlPlane gets removed from MachineConfig if it was not derived by KCPConfig", func(t *testing.T) {
		g := NewWithT(t)
		kcpConfig := &bootstrapv1.KubeadmConfigSpec{
			JoinConfiguration: &bootstrapv1.JoinConfiguration{
				ControlPlane: nil, // Control plane configuration missing in KCP
			},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			Spec: bootstrapv1.KubeadmConfigSpec{
				JoinConfiguration: &bootstrapv1.JoinConfiguration{
					ControlPlane: &bootstrapv1.JoinControlPlane{}, // Machine gets a default JoinConfiguration.ControlPlane from CABPK
				},
			},
		}
		cleanupConfigFields(kcpConfig, machineConfig)
		g.Expect(kcpConfig.JoinConfiguration).ToNot(BeNil())
		g.Expect(machineConfig.Spec.JoinConfiguration.ControlPlane).To(BeNil())
	})
	t.Run("JoinConfiguration.NodeRegistrationOptions gets removed from MachineConfig if it was not derived by KCPConfig", func(t *testing.T) {
		g := NewWithT(t)
		kcpConfig := &bootstrapv1.KubeadmConfigSpec{
			JoinConfiguration: &bootstrapv1.JoinConfiguration{
				NodeRegistration: bootstrapv1.NodeRegistrationOptions{}, // NodeRegistrationOptions configuration missing in KCP
			},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			Spec: bootstrapv1.KubeadmConfigSpec{
				JoinConfiguration: &bootstrapv1.JoinConfiguration{
					NodeRegistration: bootstrapv1.NodeRegistrationOptions{Name: "test"}, // Machine gets a some JoinConfiguration.NodeRegistrationOptions
				},
			},
		}
		cleanupConfigFields(kcpConfig, machineConfig)
		g.Expect(kcpConfig.JoinConfiguration).ToNot(BeNil())
		g.Expect(machineConfig.Spec.JoinConfiguration.NodeRegistration).To(Equal(bootstrapv1.NodeRegistrationOptions{}))
	})
	t.Run("InitConfiguration.TypeMeta gets removed from MachineConfig", func(t *testing.T) {
		g := NewWithT(t)
		kcpConfig := &bootstrapv1.KubeadmConfigSpec{
			InitConfiguration: &bootstrapv1.InitConfiguration{},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			Spec: bootstrapv1.KubeadmConfigSpec{
				InitConfiguration: &bootstrapv1.InitConfiguration{
					TypeMeta: metav1.TypeMeta{
						Kind:       "JoinConfiguration",
						APIVersion: bootstrapv1.GroupVersion.String(),
					},
				},
			},
		}
		cleanupConfigFields(kcpConfig, machineConfig)
		g.Expect(kcpConfig.InitConfiguration).ToNot(BeNil())
		g.Expect(machineConfig.Spec.InitConfiguration.TypeMeta).To(Equal(metav1.TypeMeta{}))
	})
	t.Run("JoinConfiguration.TypeMeta gets removed from MachineConfig", func(t *testing.T) {
		g := NewWithT(t)
		kcpConfig := &bootstrapv1.KubeadmConfigSpec{
			JoinConfiguration: &bootstrapv1.JoinConfiguration{},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			Spec: bootstrapv1.KubeadmConfigSpec{
				JoinConfiguration: &bootstrapv1.JoinConfiguration{
					TypeMeta: metav1.TypeMeta{
						Kind:       "JoinConfiguration",
						APIVersion: bootstrapv1.GroupVersion.String(),
					},
				},
			},
		}
		cleanupConfigFields(kcpConfig, machineConfig)
		g.Expect(kcpConfig.JoinConfiguration).ToNot(BeNil())
		g.Expect(machineConfig.Spec.JoinConfiguration.TypeMeta).To(Equal(metav1.TypeMeta{}))
	})
}

func TestMatchInitOrJoinConfiguration(t *testing.T) {
	t.Run("returns true if the machine does not have a bootstrap config", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{}
		g.Expect(matchInitOrJoinConfiguration(nil, kcp)).To(BeTrue())
	})
	t.Run("returns true if the there are problems reading the bootstrap config", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{}
		g.Expect(matchInitOrJoinConfiguration(nil, kcp)).To(BeTrue())
	})
	t.Run("returns true if InitConfiguration is equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    &bootstrapv1.InitConfiguration{},
					JoinConfiguration:    &bootstrapv1.JoinConfiguration{},
				},
			},
		}
		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "KubeadmConfig",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{
						Kind:       "KubeadmConfig",
						Namespace:  "default",
						Name:       "test",
						APIVersion: bootstrapv1.GroupVersion.String(),
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				TypeMeta: metav1.TypeMeta{
					Kind:       "KubeadmConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					InitConfiguration: &bootstrapv1.InitConfiguration{},
				},
			},
		}
		g.Expect(matchInitOrJoinConfiguration(machineConfigs[m.Name], kcp)).To(BeTrue())
	})
	t.Run("returns false if InitConfiguration is NOT equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{},
					InitConfiguration: &bootstrapv1.InitConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "A new name", // This is a change
						},
					},
					JoinConfiguration: &bootstrapv1.JoinConfiguration{},
				},
			},
		}
		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "KubeadmConfig",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{
						Kind:       "KubeadmConfig",
						Namespace:  "default",
						Name:       "test",
						APIVersion: bootstrapv1.GroupVersion.String(),
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				TypeMeta: metav1.TypeMeta{
					Kind:       "KubeadmConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					InitConfiguration: &bootstrapv1.InitConfiguration{},
				},
			},
		}
		g.Expect(matchInitOrJoinConfiguration(machineConfigs[m.Name], kcp)).To(BeFalse())
	})
	t.Run("returns true if JoinConfiguration is equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    &bootstrapv1.InitConfiguration{},
					JoinConfiguration:    &bootstrapv1.JoinConfiguration{},
				},
			},
		}
		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "KubeadmConfig",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{
						Kind:       "KubeadmConfig",
						Namespace:  "default",
						Name:       "test",
						APIVersion: bootstrapv1.GroupVersion.String(),
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				TypeMeta: metav1.TypeMeta{
					Kind:       "KubeadmConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: &bootstrapv1.JoinConfiguration{},
				},
			},
		}
		g.Expect(matchInitOrJoinConfiguration(machineConfigs[m.Name], kcp)).To(BeTrue())
	})
	t.Run("returns false if JoinConfiguration is NOT equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    &bootstrapv1.InitConfiguration{},
					JoinConfiguration: &bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "A new name", // This is a change
						},
					},
				},
			},
		}
		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "KubeadmConfig",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{
						Kind:       "KubeadmConfig",
						Namespace:  "default",
						Name:       "test",
						APIVersion: bootstrapv1.GroupVersion.String(),
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				TypeMeta: metav1.TypeMeta{
					Kind:       "KubeadmConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: &bootstrapv1.JoinConfiguration{},
				},
			},
		}
		g.Expect(matchInitOrJoinConfiguration(machineConfigs[m.Name], kcp)).To(BeFalse())
	})
	t.Run("returns false if some other configurations are not equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    &bootstrapv1.InitConfiguration{},
					JoinConfiguration:    &bootstrapv1.JoinConfiguration{},
					Files:                []bootstrapv1.File{}, // This is a change
				},
			},
		}
		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "KubeadmConfig",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{
						Kind:       "KubeadmConfig",
						Namespace:  "default",
						Name:       "test",
						APIVersion: bootstrapv1.GroupVersion.String(),
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				TypeMeta: metav1.TypeMeta{
					Kind:       "KubeadmConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					InitConfiguration: &bootstrapv1.InitConfiguration{},
				},
			},
		}
		g.Expect(matchInitOrJoinConfiguration(machineConfigs[m.Name], kcp)).To(BeFalse())
	})
}

func TestMatchesKubeadmBootstrapConfig(t *testing.T) {
	t.Run("returns true if ClusterConfiguration is equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
						ClusterName: "foo",
					},
				},
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					controlplanev1.KubeadmClusterConfigurationAnnotation: "{\n  \"clusterName\": \"foo\"\n}",
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {},
		}
		f := MatchesKubeadmBootstrapConfig(machineConfigs, kcp)
		g.Expect(f(m)).To(BeTrue())
	})
	t.Run("returns false if ClusterConfiguration is NOT equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
						ClusterName: "foo",
					},
				},
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					controlplanev1.KubeadmClusterConfigurationAnnotation: "{\n  \"clusterName\": \"bar\"\n}",
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {},
		}
		f := MatchesKubeadmBootstrapConfig(machineConfigs, kcp)
		g.Expect(f(m)).To(BeFalse())
	})
	t.Run("returns true if InitConfiguration is equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    &bootstrapv1.InitConfiguration{},
					JoinConfiguration:    &bootstrapv1.JoinConfiguration{},
				},
			},
		}
		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "KubeadmConfig",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{
						Kind:       "KubeadmConfig",
						Namespace:  "default",
						Name:       "test",
						APIVersion: bootstrapv1.GroupVersion.String(),
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				TypeMeta: metav1.TypeMeta{
					Kind:       "KubeadmConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					InitConfiguration: &bootstrapv1.InitConfiguration{},
				},
			},
		}
		f := MatchesKubeadmBootstrapConfig(machineConfigs, kcp)
		g.Expect(f(m)).To(BeTrue())
	})
	t.Run("returns false if InitConfiguration is NOT equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{},
					InitConfiguration: &bootstrapv1.InitConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "foo", // This is a change
						},
					},
					JoinConfiguration: &bootstrapv1.JoinConfiguration{},
				},
			},
		}
		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "KubeadmConfig",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{
						Kind:       "KubeadmConfig",
						Namespace:  "default",
						Name:       "test",
						APIVersion: bootstrapv1.GroupVersion.String(),
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				TypeMeta: metav1.TypeMeta{
					Kind:       "KubeadmConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					InitConfiguration: &bootstrapv1.InitConfiguration{},
				},
			},
		}
		f := MatchesKubeadmBootstrapConfig(machineConfigs, kcp)
		g.Expect(f(m)).To(BeFalse())
	})
	t.Run("returns true if JoinConfiguration is equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    &bootstrapv1.InitConfiguration{},
					JoinConfiguration:    &bootstrapv1.JoinConfiguration{},
				},
			},
		}
		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "KubeadmConfig",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{
						Kind:       "KubeadmConfig",
						Namespace:  "default",
						Name:       "test",
						APIVersion: bootstrapv1.GroupVersion.String(),
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				TypeMeta: metav1.TypeMeta{
					Kind:       "KubeadmConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: &bootstrapv1.JoinConfiguration{},
				},
			},
		}
		f := MatchesKubeadmBootstrapConfig(machineConfigs, kcp)
		g.Expect(f(m)).To(BeTrue())
	})
	t.Run("returns false if JoinConfiguration is NOT equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    &bootstrapv1.InitConfiguration{},
					JoinConfiguration: &bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							Name: "foo", // This is a change
						},
					},
				},
			},
		}
		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "KubeadmConfig",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{
						Kind:       "KubeadmConfig",
						Namespace:  "default",
						Name:       "test",
						APIVersion: bootstrapv1.GroupVersion.String(),
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				TypeMeta: metav1.TypeMeta{
					Kind:       "KubeadmConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: &bootstrapv1.JoinConfiguration{},
				},
			},
		}
		f := MatchesKubeadmBootstrapConfig(machineConfigs, kcp)
		g.Expect(f(m)).To(BeFalse())
	})
	t.Run("returns false if some other configurations are not equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    &bootstrapv1.InitConfiguration{},
					JoinConfiguration:    &bootstrapv1.JoinConfiguration{},
					Files:                []bootstrapv1.File{}, // This is a change
				},
			},
		}
		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "KubeadmConfig",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{
						Kind:       "KubeadmConfig",
						Namespace:  "default",
						Name:       "test",
						APIVersion: bootstrapv1.GroupVersion.String(),
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				TypeMeta: metav1.TypeMeta{
					Kind:       "KubeadmConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					InitConfiguration: &bootstrapv1.InitConfiguration{},
				},
			},
		}
		f := MatchesKubeadmBootstrapConfig(machineConfigs, kcp)
		g.Expect(f(m)).To(BeFalse())
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
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    &bootstrapv1.InitConfiguration{},
					JoinConfiguration:    &bootstrapv1.JoinConfiguration{},
				},
			},
		}
		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "KubeadmConfig",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{
						Kind:       "KubeadmConfig",
						Namespace:  "default",
						Name:       "test",
						APIVersion: bootstrapv1.GroupVersion.String(),
					},
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {
				TypeMeta: metav1.TypeMeta{
					Kind:       "KubeadmConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: &bootstrapv1.JoinConfiguration{},
				},
			},
		}

		t.Run("by returning false if neither labels or annotations match", func(t *testing.T) {
			g := NewWithT(t)
			machineConfigs[m.Name].Annotations = nil
			machineConfigs[m.Name].Labels = nil
			f := MatchesKubeadmBootstrapConfig(machineConfigs, kcp)
			g.Expect(f(m)).To(BeFalse())
		})

		t.Run("by returning false if only labels don't match", func(t *testing.T) {
			g := NewWithT(t)
			machineConfigs[m.Name].Annotations = kcp.Spec.MachineTemplate.ObjectMeta.Annotations
			machineConfigs[m.Name].Labels = nil
			f := MatchesKubeadmBootstrapConfig(machineConfigs, kcp)
			g.Expect(f(m)).To(BeFalse())
		})

		t.Run("by returning false if only annotations don't match", func(t *testing.T) {
			g := NewWithT(t)
			machineConfigs[m.Name].Annotations = nil
			machineConfigs[m.Name].Labels = kcp.Spec.MachineTemplate.ObjectMeta.Labels
			f := MatchesKubeadmBootstrapConfig(machineConfigs, kcp)
			g.Expect(f(m)).To(BeFalse())
		})

		t.Run("by returning true if both labels and annotations match", func(t *testing.T) {
			g := NewWithT(t)
			machineConfigs[m.Name].Labels = kcp.Spec.MachineTemplate.ObjectMeta.Labels
			machineConfigs[m.Name].Annotations = kcp.Spec.MachineTemplate.ObjectMeta.Annotations
			f := MatchesKubeadmBootstrapConfig(machineConfigs, kcp)
			g.Expect(f(m)).To(BeTrue())
		})
	})
}

func TestMatchesTemplateClonedFrom(t *testing.T) {
	t.Run("nil machine returns false", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(
			MatchesTemplateClonedFrom(nil, nil)(nil),
		).To(BeFalse())
	})

	t.Run("returns true if machine not found", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{}
		machine := &clusterv1.Machine{
			Spec: clusterv1.MachineSpec{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "KubeadmConfig",
					Namespace:  "default",
					Name:       "test",
					APIVersion: bootstrapv1.GroupVersion.String(),
				},
			},
		}
		g.Expect(
			MatchesTemplateClonedFrom(map[string]*unstructured.Unstructured{}, kcp)(machine),
		).To(BeTrue())
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
					InfrastructureRef: corev1.ObjectReference{
						Kind:       "GenericMachineTemplate",
						Namespace:  "default",
						Name:       "infra-foo",
						APIVersion: "generic.io/v1",
					},
				},
			},
		}
		m := &clusterv1.Machine{
			Spec: clusterv1.MachineSpec{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "GenericMachine",
					Namespace:  "default",
					Name:       "infra-foo",
					APIVersion: "generic.io/v1",
				},
			},
		}

		infraConfigs := map[string]*unstructured.Unstructured{
			m.Name: {
				Object: map[string]interface{}{
					"kind":       "InfrastructureMachine",
					"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
					"metadata": map[string]interface{}{
						"name":      "infra-config1",
						"namespace": "default",
					},
				},
			},
		}

		t.Run("by returning false if neither labels or annotations match", func(t *testing.T) {
			g := NewWithT(t)
			infraConfigs[m.Name].SetAnnotations(map[string]string{
				clusterv1.TemplateClonedFromNameAnnotation:      "infra-foo",
				clusterv1.TemplateClonedFromGroupKindAnnotation: "GenericMachineTemplate.generic.io",
			})
			infraConfigs[m.Name].SetLabels(nil)
			f := MatchesTemplateClonedFrom(infraConfigs, kcp)
			g.Expect(f(m)).To(BeFalse())
		})

		t.Run("by returning false if only labels don't match", func(t *testing.T) {
			g := NewWithT(t)
			infraConfigs[m.Name].SetAnnotations(map[string]string{
				clusterv1.TemplateClonedFromNameAnnotation:      "infra-foo",
				clusterv1.TemplateClonedFromGroupKindAnnotation: "GenericMachineTemplate.generic.io",
				"test": "annotation",
			})
			infraConfigs[m.Name].SetLabels(nil)
			f := MatchesTemplateClonedFrom(infraConfigs, kcp)
			g.Expect(f(m)).To(BeFalse())
		})

		t.Run("by returning false if only annotations don't match", func(t *testing.T) {
			g := NewWithT(t)
			infraConfigs[m.Name].SetAnnotations(map[string]string{
				clusterv1.TemplateClonedFromNameAnnotation:      "infra-foo",
				clusterv1.TemplateClonedFromGroupKindAnnotation: "GenericMachineTemplate.generic.io",
			})
			infraConfigs[m.Name].SetLabels(kcp.Spec.MachineTemplate.ObjectMeta.Labels)
			f := MatchesTemplateClonedFrom(infraConfigs, kcp)
			g.Expect(f(m)).To(BeFalse())
		})

		t.Run("by returning true if both labels and annotations match", func(t *testing.T) {
			g := NewWithT(t)
			infraConfigs[m.Name].SetAnnotations(map[string]string{
				clusterv1.TemplateClonedFromNameAnnotation:      "infra-foo",
				clusterv1.TemplateClonedFromGroupKindAnnotation: "GenericMachineTemplate.generic.io",
				"test": "annotation",
			})
			infraConfigs[m.Name].SetLabels(kcp.Spec.MachineTemplate.ObjectMeta.Labels)
			f := MatchesTemplateClonedFrom(infraConfigs, kcp)
			g.Expect(f(m)).To(BeTrue())
		})
	})
}

func TestMatchesTemplateClonedFrom_WithClonedFromAnnotations(t *testing.T) {
	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "GenericMachineTemplate",
					Namespace:  "default",
					Name:       "infra-foo",
					APIVersion: "generic.io/v1",
				},
			},
		},
	}
	machine := &clusterv1.Machine{
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachine",
				Name:       "infra-config1",
				Namespace:  "default",
			},
		},
	}
	tests := []struct {
		name        string
		annotations map[string]interface{}
		expectMatch bool
	}{
		{
			name:        "returns true if annotations don't exist",
			annotations: map[string]interface{}{},
			expectMatch: true,
		},
		{
			name: "returns false if annotations don't match anything",
			annotations: map[string]interface{}{
				clusterv1.TemplateClonedFromNameAnnotation:      "barfoo1",
				clusterv1.TemplateClonedFromGroupKindAnnotation: "barfoo2",
			},
			expectMatch: false,
		},
		{
			name: "returns false if TemplateClonedFromNameAnnotation matches but TemplateClonedFromGroupKindAnnotation doesn't",
			annotations: map[string]interface{}{
				clusterv1.TemplateClonedFromNameAnnotation:      "infra-foo",
				clusterv1.TemplateClonedFromGroupKindAnnotation: "barfoo2",
			},
			expectMatch: false,
		},
		{
			name: "returns true if both annotations match",
			annotations: map[string]interface{}{
				clusterv1.TemplateClonedFromNameAnnotation:      "infra-foo",
				clusterv1.TemplateClonedFromGroupKindAnnotation: "GenericMachineTemplate.generic.io",
			},
			expectMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			infraConfigs := map[string]*unstructured.Unstructured{
				machine.Name: {
					Object: map[string]interface{}{
						"kind":       "InfrastructureMachine",
						"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
						"metadata": map[string]interface{}{
							"name":        "infra-config1",
							"namespace":   "default",
							"annotations": tt.annotations,
						},
					},
				},
			}
			g.Expect(
				MatchesTemplateClonedFrom(infraConfigs, kcp)(machine),
			).To(Equal(tt.expectMatch))
		})
	}
}
