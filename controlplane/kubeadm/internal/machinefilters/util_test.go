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

package machinefilters

import (
	"testing"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	kubeadmv1beta1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
)

func TestMatchClusterConfiguration(t *testing.T) {
	t.Run("machine without the ClusterConfiguration annotation should match (not enough information to make a decision)", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{}
		m := &clusterv1.Machine{}
		g.Expect(matchClusterConfiguration(kcp, m)).To(gomega.BeTrue())
	})
	t.Run("machine without an invalid ClusterConfiguration annotation should not match (only solution is to rollout)", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					controlplanev1.KubeadmClusterConfigurationAnnotation: "$|^^_",
				},
			},
		}
		g.Expect(matchClusterConfiguration(kcp, m)).To(gomega.BeFalse())
	})
	t.Run("Return true if cluster configuration matches", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{
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
		g.Expect(matchClusterConfiguration(kcp, m)).To(gomega.BeTrue())
	})
	t.Run("Return false if cluster configuration does not match", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{
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
		g.Expect(matchClusterConfiguration(kcp, m)).To(gomega.BeFalse())
	})
	t.Run("Return true if cluster configuration is nil (special case)", func(t *testing.T) {
		g := gomega.NewWithT(t)
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
		g.Expect(matchClusterConfiguration(kcp, m)).To(gomega.BeTrue())
	})
}

func TestGetAdjustedKcpConfig(t *testing.T) {
	t.Run("if the machine is the first control plane, kcp config should get InitConfiguration", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					InitConfiguration: &kubeadmv1beta1.InitConfiguration{},
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{},
				},
			},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			Spec: bootstrapv1.KubeadmConfigSpec{
				InitConfiguration: &kubeadmv1beta1.InitConfiguration{}, // first control-plane
			},
		}
		kcpConfig := getAdjustedKcpConfig(kcp, machineConfig)
		g.Expect(kcpConfig.InitConfiguration).ToNot(gomega.BeNil())
		g.Expect(kcpConfig.JoinConfiguration).To(gomega.BeNil())
	})
	t.Run("if the machine is a joining control plane, kcp config should get JoinConfiguration", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					InitConfiguration: &kubeadmv1beta1.InitConfiguration{},
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{},
				},
			},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			Spec: bootstrapv1.KubeadmConfigSpec{
				JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{}, // joining control-plane
			},
		}
		kcpConfig := getAdjustedKcpConfig(kcp, machineConfig)
		g.Expect(kcpConfig.InitConfiguration).To(gomega.BeNil())
		g.Expect(kcpConfig.JoinConfiguration).ToNot(gomega.BeNil())
	})
}

func TestCleanupConfigFields(t *testing.T) {
	t.Run("ClusterConfiguration gets removed from KcpConfig and MachineConfig", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcpConfig := &bootstrapv1.KubeadmConfigSpec{
			ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			Spec: bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{},
			},
		}
		cleanupConfigFields(kcpConfig, machineConfig)
		g.Expect(kcpConfig.ClusterConfiguration).To(gomega.BeNil())
		g.Expect(machineConfig.Spec.ClusterConfiguration).To(gomega.BeNil())
	})
	t.Run("JoinConfiguration gets removed from MachineConfig if it was not derived by KCPConfig", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcpConfig := &bootstrapv1.KubeadmConfigSpec{
			JoinConfiguration: nil, // KCP not providing a JoinConfiguration
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			Spec: bootstrapv1.KubeadmConfigSpec{
				JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{}, // Machine gets a default JoinConfiguration from CABPK
			},
		}
		cleanupConfigFields(kcpConfig, machineConfig)
		g.Expect(kcpConfig.JoinConfiguration).To(gomega.BeNil())
		g.Expect(machineConfig.Spec.JoinConfiguration).To(gomega.BeNil())
	})
	t.Run("JoinConfiguration.Discovery gets removed because it is not relevant for compare", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcpConfig := &bootstrapv1.KubeadmConfigSpec{
			JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{
				Discovery: kubeadmv1beta1.Discovery{TLSBootstrapToken: "aaa"},
			},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			Spec: bootstrapv1.KubeadmConfigSpec{
				JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{
					Discovery: kubeadmv1beta1.Discovery{TLSBootstrapToken: "aaa"},
				},
			},
		}
		cleanupConfigFields(kcpConfig, machineConfig)
		g.Expect(kcpConfig.JoinConfiguration.Discovery).To(gomega.Equal(kubeadmv1beta1.Discovery{}))
		g.Expect(machineConfig.Spec.JoinConfiguration.Discovery).To(gomega.Equal(kubeadmv1beta1.Discovery{}))
	})
	t.Run("JoinConfiguration.ControlPlane gets removed from MachineConfig if it was not derived by KCPConfig", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcpConfig := &bootstrapv1.KubeadmConfigSpec{
			JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{
				ControlPlane: nil, // Control plane configuration missing in KCP
			},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			Spec: bootstrapv1.KubeadmConfigSpec{
				JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{
					ControlPlane: &kubeadmv1beta1.JoinControlPlane{}, // Machine gets a default JoinConfiguration.ControlPlane from CABPK
				},
			},
		}
		cleanupConfigFields(kcpConfig, machineConfig)
		g.Expect(kcpConfig.JoinConfiguration).ToNot(gomega.BeNil())
		g.Expect(machineConfig.Spec.JoinConfiguration.ControlPlane).To(gomega.BeNil())
	})
	t.Run("JoinConfiguration.NodeRegistrationOptions gets removed from MachineConfig if it was not derived by KCPConfig", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcpConfig := &bootstrapv1.KubeadmConfigSpec{
			JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{
				NodeRegistration: kubeadmv1beta1.NodeRegistrationOptions{}, // NodeRegistrationOptions configuration missing in KCP
			},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			Spec: bootstrapv1.KubeadmConfigSpec{
				JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{
					NodeRegistration: kubeadmv1beta1.NodeRegistrationOptions{Name: "test"}, // Machine gets a some JoinConfiguration.NodeRegistrationOptions
				},
			},
		}
		cleanupConfigFields(kcpConfig, machineConfig)
		g.Expect(kcpConfig.JoinConfiguration).ToNot(gomega.BeNil())
		g.Expect(machineConfig.Spec.JoinConfiguration.NodeRegistration).To(gomega.Equal(kubeadmv1beta1.NodeRegistrationOptions{}))
	})
	t.Run("InitConfiguration.TypeMeta gets removed from MachineConfig", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcpConfig := &bootstrapv1.KubeadmConfigSpec{
			InitConfiguration: &kubeadmv1beta1.InitConfiguration{},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			Spec: bootstrapv1.KubeadmConfigSpec{
				InitConfiguration: &kubeadmv1beta1.InitConfiguration{
					TypeMeta: metav1.TypeMeta{
						Kind:       "JoinConfiguration",
						APIVersion: kubeadmv1beta1.GroupVersion.String(),
					},
				},
			},
		}
		cleanupConfigFields(kcpConfig, machineConfig)
		g.Expect(kcpConfig.InitConfiguration).ToNot(gomega.BeNil())
		g.Expect(machineConfig.Spec.InitConfiguration.TypeMeta).To(gomega.Equal(metav1.TypeMeta{}))
	})
	t.Run("JoinConfiguration.TypeMeta gets removed from MachineConfig", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcpConfig := &bootstrapv1.KubeadmConfigSpec{
			JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			Spec: bootstrapv1.KubeadmConfigSpec{
				JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{
					TypeMeta: metav1.TypeMeta{
						Kind:       "JoinConfiguration",
						APIVersion: kubeadmv1beta1.GroupVersion.String(),
					},
				},
			},
		}
		cleanupConfigFields(kcpConfig, machineConfig)
		g.Expect(kcpConfig.JoinConfiguration).ToNot(gomega.BeNil())
		g.Expect(machineConfig.Spec.JoinConfiguration.TypeMeta).To(gomega.Equal(metav1.TypeMeta{}))
	})
}

func TestMatchInitOrJoinConfiguration(t *testing.T) {
	t.Run("returns true if the machine does not have a bootstrap config", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{}
		m := &clusterv1.Machine{}
		g.Expect(matchInitOrJoinConfiguration(nil, kcp, m)).To(gomega.BeTrue())
	})
	t.Run("returns true if the there are problems reading the bootstrap config", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{}
		m := &clusterv1.Machine{
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{},
				},
			},
		}
		g.Expect(matchInitOrJoinConfiguration(nil, kcp, m)).To(gomega.BeTrue())
	})
	t.Run("returns true if InitConfiguration is equal", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{},
					InitConfiguration:    &kubeadmv1beta1.InitConfiguration{},
					JoinConfiguration:    &kubeadmv1beta1.JoinConfiguration{},
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
					InitConfiguration: &kubeadmv1beta1.InitConfiguration{},
				},
			},
		}
		g.Expect(matchInitOrJoinConfiguration(machineConfigs, kcp, m)).To(gomega.BeTrue())
	})
	t.Run("returns false if InitConfiguration is NOT equal", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{},
					InitConfiguration: &kubeadmv1beta1.InitConfiguration{
						NodeRegistration: kubeadmv1beta1.NodeRegistrationOptions{
							Name: "A new name", // This is a change
						},
					},
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{},
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
					InitConfiguration: &kubeadmv1beta1.InitConfiguration{},
				},
			},
		}
		g.Expect(matchInitOrJoinConfiguration(machineConfigs, kcp, m)).To(gomega.BeFalse())
	})
	t.Run("returns true if JoinConfiguration is equal", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{},
					InitConfiguration:    &kubeadmv1beta1.InitConfiguration{},
					JoinConfiguration:    &kubeadmv1beta1.JoinConfiguration{},
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
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{},
				},
			},
		}
		g.Expect(matchInitOrJoinConfiguration(machineConfigs, kcp, m)).To(gomega.BeTrue())
	})
	t.Run("returns false if JoinConfiguration is NOT equal", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{},
					InitConfiguration:    &kubeadmv1beta1.InitConfiguration{},
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{
						NodeRegistration: kubeadmv1beta1.NodeRegistrationOptions{
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
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{},
				},
			},
		}
		g.Expect(matchInitOrJoinConfiguration(machineConfigs, kcp, m)).To(gomega.BeFalse())
	})
	t.Run("returns false if some other configurations are not equal", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{},
					InitConfiguration:    &kubeadmv1beta1.InitConfiguration{},
					JoinConfiguration:    &kubeadmv1beta1.JoinConfiguration{},
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
					InitConfiguration: &kubeadmv1beta1.InitConfiguration{},
				},
			},
		}
		g.Expect(matchInitOrJoinConfiguration(machineConfigs, kcp, m)).To(gomega.BeFalse())
	})
}

func TestMatchesKubeadmBootstrapConfig(t *testing.T) {
	t.Run("returns true if ClusterConfiguration is equal", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{
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
		g.Expect(f(m)).To(gomega.BeTrue())
	})
	t.Run("returns false if ClusterConfiguration is NOT equal", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{
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
		g.Expect(f(m)).To(gomega.BeFalse())
	})
	t.Run("returns true if InitConfiguration is equal", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{},
					InitConfiguration:    &kubeadmv1beta1.InitConfiguration{},
					JoinConfiguration:    &kubeadmv1beta1.JoinConfiguration{},
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
					InitConfiguration: &kubeadmv1beta1.InitConfiguration{},
				},
			},
		}
		f := MatchesKubeadmBootstrapConfig(machineConfigs, kcp)
		g.Expect(f(m)).To(gomega.BeTrue())
	})
	t.Run("returns false if InitConfiguration is NOT equal", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{},
					InitConfiguration: &kubeadmv1beta1.InitConfiguration{
						NodeRegistration: kubeadmv1beta1.NodeRegistrationOptions{
							Name: "foo", // This is a change
						},
					},
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{},
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
					InitConfiguration: &kubeadmv1beta1.InitConfiguration{},
				},
			},
		}
		f := MatchesKubeadmBootstrapConfig(machineConfigs, kcp)
		g.Expect(f(m)).To(gomega.BeFalse())
	})
	t.Run("returns true if JoinConfiguration is equal", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{},
					InitConfiguration:    &kubeadmv1beta1.InitConfiguration{},
					JoinConfiguration:    &kubeadmv1beta1.JoinConfiguration{},
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
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{},
				},
			},
		}
		f := MatchesKubeadmBootstrapConfig(machineConfigs, kcp)
		g.Expect(f(m)).To(gomega.BeTrue())
	})
	t.Run("returns false if JoinConfiguration is NOT equal", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{},
					InitConfiguration:    &kubeadmv1beta1.InitConfiguration{},
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{
						NodeRegistration: kubeadmv1beta1.NodeRegistrationOptions{
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
					JoinConfiguration: &kubeadmv1beta1.JoinConfiguration{},
				},
			},
		}
		f := MatchesKubeadmBootstrapConfig(machineConfigs, kcp)
		g.Expect(f(m)).To(gomega.BeFalse())
	})
	t.Run("returns false if some other configurations are not equal", func(t *testing.T) {
		g := gomega.NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{},
					InitConfiguration:    &kubeadmv1beta1.InitConfiguration{},
					JoinConfiguration:    &kubeadmv1beta1.JoinConfiguration{},
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
					InitConfiguration: &kubeadmv1beta1.InitConfiguration{},
				},
			},
		}
		f := MatchesKubeadmBootstrapConfig(machineConfigs, kcp)
		g.Expect(f(m)).To(gomega.BeFalse())
	})
}
