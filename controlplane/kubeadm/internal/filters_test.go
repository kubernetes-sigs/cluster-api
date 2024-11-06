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
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
)

func TestMatchClusterConfiguration(t *testing.T) {
	t.Run("machine without the ClusterConfiguration annotation should match (not enough information to make a decision)", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{}
		m := &clusterv1.Machine{}
		match, diff, err := matchClusterConfiguration(kcp, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeTrue())
		g.Expect(diff).To(BeEmpty())
	})
	t.Run("machine with an invalid ClusterConfiguration annotation should not match (only solution is to rollout)", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					controlplanev1.KubeadmClusterConfigurationAnnotation: "$|^^_",
				},
			},
		}
		match, diff, err := matchClusterConfiguration(kcp, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeFalse())
		g.Expect(diff).To(BeEmpty())
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
		match, diff, err := matchClusterConfiguration(kcp, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeTrue())
		g.Expect(diff).To(BeEmpty())
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
		match, diff, err := matchClusterConfiguration(kcp, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeFalse())
		g.Expect(diff).To(BeComparableTo(`&v1beta1.ClusterConfiguration{
    ... // 10 identical fields
    ImageRepository: "",
    FeatureGates:    nil,
-   ClusterName:     "bar",
+   ClusterName:     "foo",
  }`))
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
		match, diff, err := matchClusterConfiguration(kcp, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeTrue())
		g.Expect(diff).To(BeEmpty())
	})
	t.Run("Return true although the DNS fields are different", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
						DNS: bootstrapv1.DNS{
							ImageMeta: bootstrapv1.ImageMeta{
								ImageTag:        "v1.10.1",
								ImageRepository: "gcr.io/capi-test",
							},
						},
					},
				},
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					controlplanev1.KubeadmClusterConfigurationAnnotation: "{\"dns\":{\"imageRepository\":\"gcr.io/capi-test\",\"imageTag\":\"v1.9.3\"}}",
				},
			},
		}
		match, diff, err := matchClusterConfiguration(kcp, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeTrue())
		g.Expect(diff).To(BeEmpty())
	})
	t.Run("Check we are not introducing unexpected rollouts when changing the API", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
						APIServer: bootstrapv1.APIServer{
							ControlPlaneComponent: bootstrapv1.ControlPlaneComponent{
								ExtraArgs: map[string]string{"foo": "bar"},
							},
						},
						ControllerManager: bootstrapv1.ControlPlaneComponent{
							ExtraArgs: map[string]string{"foo": "bar"},
						},
						Scheduler: bootstrapv1.ControlPlaneComponent{
							ExtraArgs: map[string]string{"foo": "bar"},
						},
						DNS: bootstrapv1.DNS{
							ImageMeta: bootstrapv1.ImageMeta{
								ImageTag:        "v1.10.1",
								ImageRepository: "gcr.io/capi-test",
							},
						},
					},
				},
			},
		}

		// This is a point in time snapshot of how a serialized ClusterConfiguration looks like;
		// we are hardcoding this in the test so we can detect if a change in the API impacts serialization.
		// NOTE: changes in the json representation do not always trigger a rollout in KCP, but they are an heads up that should be investigated.
		clusterConfigCheckPoint := []byte("{\"etcd\":{},\"networking\":{},\"apiServer\":{\"extraArgs\":{\"foo\":\"bar\"}},\"controllerManager\":{\"extraArgs\":{\"foo\":\"bar\"}},\"scheduler\":{\"extraArgs\":{\"foo\":\"bar\"}},\"dns\":{\"imageRepository\":\"gcr.io/capi-test\",\"imageTag\":\"v1.10.1\"}}")

		// compute how a serialized ClusterConfiguration looks like now
		clusterConfig, err := json.Marshal(kcp.Spec.KubeadmConfigSpec.ClusterConfiguration)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(clusterConfig).To(Equal(clusterConfigCheckPoint))

		// check the match function detects if a Machine with the annotation string above matches the object it originates from (round trip).
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					controlplanev1.KubeadmClusterConfigurationAnnotation: string(clusterConfig),
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
		g.Expect(kcpConfig.JoinConfiguration.Discovery).To(BeComparableTo(bootstrapv1.Discovery{}))
		g.Expect(machineConfig.Spec.JoinConfiguration.Discovery).To(BeComparableTo(bootstrapv1.Discovery{}))
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
		g.Expect(machineConfig.Spec.JoinConfiguration.NodeRegistration).To(BeComparableTo(bootstrapv1.NodeRegistrationOptions{}))
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
		g.Expect(machineConfig.Spec.InitConfiguration.TypeMeta).To(BeComparableTo(metav1.TypeMeta{}))
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
		g.Expect(machineConfig.Spec.JoinConfiguration.TypeMeta).To(BeComparableTo(metav1.TypeMeta{}))
	})
}

func TestMatchInitOrJoinConfiguration(t *testing.T) {
	t.Run("returns true if the machine does not have a bootstrap config", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{}
		match, diff, err := matchInitOrJoinConfiguration(nil, kcp)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeTrue())
		g.Expect(diff).To(BeEmpty())
	})
	t.Run("returns true if one format is empty and the other one cloud-config", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					Format: bootstrapv1.CloudConfig,
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
					Format: "",
				},
			},
		}
		match, diff, err := matchInitOrJoinConfiguration(machineConfigs[m.Name], kcp)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeTrue())
		g.Expect(diff).To(BeEmpty())
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
		match, diff, err := matchInitOrJoinConfiguration(machineConfigs[m.Name], kcp)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeTrue())
		g.Expect(diff).To(BeEmpty())
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
		match, diff, err := matchInitOrJoinConfiguration(machineConfigs[m.Name], kcp)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeFalse())
		g.Expect(diff).To(BeComparableTo(`&v1beta1.KubeadmConfigSpec{
    ClusterConfiguration: nil,
    InitConfiguration: &v1beta1.InitConfiguration{
      TypeMeta:        {},
      BootstrapTokens: nil,
      NodeRegistration: v1beta1.NodeRegistrationOptions{
-       Name:      "",
+       Name:      "A new name",
        CRISocket: "",
        Taints:    nil,
        ... // 4 identical fields
      },
      LocalAPIEndpoint: {},
      SkipPhases:       nil,
      Patches:          nil,
    },
    JoinConfiguration: nil,
    Files:             nil,
    ... // 10 identical fields
  }`))
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
		match, diff, err := matchInitOrJoinConfiguration(machineConfigs[m.Name], kcp)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeTrue())
		g.Expect(diff).To(BeEmpty())
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
		match, diff, err := matchInitOrJoinConfiguration(machineConfigs[m.Name], kcp)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeFalse())
		g.Expect(diff).To(BeComparableTo(`&v1beta1.KubeadmConfigSpec{
    ClusterConfiguration: nil,
    InitConfiguration:    nil,
    JoinConfiguration: &v1beta1.JoinConfiguration{
      TypeMeta: {},
      NodeRegistration: v1beta1.NodeRegistrationOptions{
-       Name:      "",
+       Name:      "A new name",
        CRISocket: "",
        Taints:    nil,
        ... // 4 identical fields
      },
      CACertPath: "",
      Discovery:  {},
      ... // 3 identical fields
    },
    Files:     nil,
    DiskSetup: nil,
    ... // 9 identical fields
  }`))
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
		match, diff, err := matchInitOrJoinConfiguration(machineConfigs[m.Name], kcp)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeFalse())
		g.Expect(diff).To(BeComparableTo(`&v1beta1.KubeadmConfigSpec{
    ClusterConfiguration: nil,
    InitConfiguration:    &{NodeRegistration: {ImagePullPolicy: "IfNotPresent"}},
    JoinConfiguration:    nil,
-   Files:                nil,
+   Files:                []v1beta1.File{},
    DiskSetup:            nil,
    Mounts:               nil,
    ... // 8 identical fields
  }`))
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
		reason, match, err := matchesKubeadmBootstrapConfig(machineConfigs, kcp, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeTrue())
		g.Expect(reason).To(BeEmpty())
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
		reason, match, err := matchesKubeadmBootstrapConfig(machineConfigs, kcp, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeFalse())
		g.Expect(reason).To(BeComparableTo(`Machine KubeadmConfig ClusterConfiguration is outdated: diff: &v1beta1.ClusterConfiguration{
    ... // 10 identical fields
    ImageRepository: "",
    FeatureGates:    nil,
-   ClusterName:     "bar",
+   ClusterName:     "foo",
  }`))
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
		reason, match, err := matchesKubeadmBootstrapConfig(machineConfigs, kcp, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeTrue())
		g.Expect(reason).To(BeEmpty())
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
		reason, match, err := matchesKubeadmBootstrapConfig(machineConfigs, kcp, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeFalse())
		g.Expect(reason).To(BeComparableTo(`Machine KubeadmConfig InitConfiguration or JoinConfiguration are outdated: diff: &v1beta1.KubeadmConfigSpec{
    ClusterConfiguration: nil,
    InitConfiguration: &v1beta1.InitConfiguration{
      TypeMeta:        {},
      BootstrapTokens: nil,
      NodeRegistration: v1beta1.NodeRegistrationOptions{
-       Name:      "",
+       Name:      "foo",
        CRISocket: "",
        Taints:    nil,
        ... // 4 identical fields
      },
      LocalAPIEndpoint: {},
      SkipPhases:       nil,
      Patches:          nil,
    },
    JoinConfiguration: nil,
    Files:             nil,
    ... // 10 identical fields
  }`))
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
		reason, match, err := matchesKubeadmBootstrapConfig(machineConfigs, kcp, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeTrue())
		g.Expect(reason).To(BeEmpty())
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
		reason, match, err := matchesKubeadmBootstrapConfig(machineConfigs, kcp, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeFalse())
		g.Expect(reason).To(BeComparableTo(`Machine KubeadmConfig InitConfiguration or JoinConfiguration are outdated: diff: &v1beta1.KubeadmConfigSpec{
    ClusterConfiguration: nil,
    InitConfiguration:    nil,
    JoinConfiguration: &v1beta1.JoinConfiguration{
      TypeMeta: {},
      NodeRegistration: v1beta1.NodeRegistrationOptions{
-       Name:      "",
+       Name:      "foo",
        CRISocket: "",
        Taints:    nil,
        ... // 4 identical fields
      },
      CACertPath: "",
      Discovery:  {},
      ... // 3 identical fields
    },
    Files:     nil,
    DiskSetup: nil,
    ... // 9 identical fields
  }`))
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
		reason, match, err := matchesKubeadmBootstrapConfig(machineConfigs, kcp, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeFalse())
		g.Expect(reason).To(BeComparableTo(`Machine KubeadmConfig InitConfiguration or JoinConfiguration are outdated: diff: &v1beta1.KubeadmConfigSpec{
    ClusterConfiguration: nil,
    InitConfiguration:    &{NodeRegistration: {ImagePullPolicy: "IfNotPresent"}},
    JoinConfiguration:    nil,
-   Files:                nil,
+   Files:                []v1beta1.File{},
    DiskSetup:            nil,
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

		t.Run("by returning true if neither labels or annotations match", func(t *testing.T) {
			g := NewWithT(t)
			machineConfigs[m.Name].Annotations = nil
			machineConfigs[m.Name].Labels = nil
			reason, match, err := matchesKubeadmBootstrapConfig(machineConfigs, kcp, m)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(match).To(BeTrue())
			g.Expect(reason).To(BeEmpty())
		})

		t.Run("by returning true if only labels don't match", func(t *testing.T) {
			g := NewWithT(t)
			machineConfigs[m.Name].Annotations = kcp.Spec.MachineTemplate.ObjectMeta.Annotations
			machineConfigs[m.Name].Labels = nil
			reason, match, err := matchesKubeadmBootstrapConfig(machineConfigs, kcp, m)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(match).To(BeTrue())
			g.Expect(reason).To(BeEmpty())
		})

		t.Run("by returning true if only annotations don't match", func(t *testing.T) {
			g := NewWithT(t)
			machineConfigs[m.Name].Annotations = nil
			machineConfigs[m.Name].Labels = kcp.Spec.MachineTemplate.ObjectMeta.Labels
			reason, match, err := matchesKubeadmBootstrapConfig(machineConfigs, kcp, m)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(match).To(BeTrue())
			g.Expect(reason).To(BeEmpty())
		})

		t.Run("by returning true if both labels and annotations match", func(t *testing.T) {
			g := NewWithT(t)
			machineConfigs[m.Name].Labels = kcp.Spec.MachineTemplate.ObjectMeta.Labels
			machineConfigs[m.Name].Annotations = kcp.Spec.MachineTemplate.ObjectMeta.Annotations
			reason, match, err := matchesKubeadmBootstrapConfig(machineConfigs, kcp, m)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(match).To(BeTrue())
			g.Expect(reason).To(BeEmpty())
		})
	})
}

func TestMatchesTemplateClonedFrom(t *testing.T) {
	t.Run("nil machine returns false", func(t *testing.T) {
		g := NewWithT(t)
		reason, match := matchesTemplateClonedFrom(nil, nil, nil)
		g.Expect(match).To(BeFalse())
		g.Expect(reason).To(Equal("Machine cannot be compared with KCP.spec.machineTemplate.infrastructureRef: Machine is nil"))
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
		reason, match := matchesTemplateClonedFrom(map[string]*unstructured.Unstructured{}, kcp, machine)
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

		t.Run("by returning true if neither labels or annotations match", func(t *testing.T) {
			g := NewWithT(t)
			infraConfigs[m.Name].SetAnnotations(map[string]string{
				clusterv1.TemplateClonedFromNameAnnotation:      "infra-foo",
				clusterv1.TemplateClonedFromGroupKindAnnotation: "GenericMachineTemplate.generic.io",
			})
			infraConfigs[m.Name].SetLabels(nil)
			reason, match := matchesTemplateClonedFrom(infraConfigs, kcp, m)
			g.Expect(match).To(BeTrue())
			g.Expect(reason).To(BeEmpty())
		})

		t.Run("by returning true if only labels don't match", func(t *testing.T) {
			g := NewWithT(t)
			infraConfigs[m.Name].SetAnnotations(map[string]string{
				clusterv1.TemplateClonedFromNameAnnotation:      "infra-foo",
				clusterv1.TemplateClonedFromGroupKindAnnotation: "GenericMachineTemplate.generic.io",
				"test": "annotation",
			})
			infraConfigs[m.Name].SetLabels(nil)
			reason, match := matchesTemplateClonedFrom(infraConfigs, kcp, m)
			g.Expect(match).To(BeTrue())
			g.Expect(reason).To(BeEmpty())
		})

		t.Run("by returning true if only annotations don't match", func(t *testing.T) {
			g := NewWithT(t)
			infraConfigs[m.Name].SetAnnotations(map[string]string{
				clusterv1.TemplateClonedFromNameAnnotation:      "infra-foo",
				clusterv1.TemplateClonedFromGroupKindAnnotation: "GenericMachineTemplate.generic.io",
			})
			infraConfigs[m.Name].SetLabels(kcp.Spec.MachineTemplate.ObjectMeta.Labels)
			reason, match := matchesTemplateClonedFrom(infraConfigs, kcp, m)
			g.Expect(match).To(BeTrue())
			g.Expect(reason).To(BeEmpty())
		})

		t.Run("by returning true if both labels and annotations match", func(t *testing.T) {
			g := NewWithT(t)
			infraConfigs[m.Name].SetAnnotations(map[string]string{
				clusterv1.TemplateClonedFromNameAnnotation:      "infra-foo",
				clusterv1.TemplateClonedFromGroupKindAnnotation: "GenericMachineTemplate.generic.io",
				"test": "annotation",
			})
			infraConfigs[m.Name].SetLabels(kcp.Spec.MachineTemplate.ObjectMeta.Labels)
			reason, match := matchesTemplateClonedFrom(infraConfigs, kcp, m)
			g.Expect(match).To(BeTrue())
			g.Expect(reason).To(BeEmpty())
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
		name         string
		annotations  map[string]interface{}
		expectMatch  bool
		expectReason string
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
			expectMatch:  false,
			expectReason: "Infrastructure template on KCP rotated from barfoo2 barfoo1 to GenericMachineTemplate.generic.io infra-foo",
		},
		{
			name: "returns false if TemplateClonedFromNameAnnotation matches but TemplateClonedFromGroupKindAnnotation doesn't",
			annotations: map[string]interface{}{
				clusterv1.TemplateClonedFromNameAnnotation:      "infra-foo",
				clusterv1.TemplateClonedFromGroupKindAnnotation: "barfoo2",
			},
			expectMatch:  false,
			expectReason: "Infrastructure template on KCP rotated from barfoo2 infra-foo to GenericMachineTemplate.generic.io infra-foo",
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
			reason, match := matchesTemplateClonedFrom(infraConfigs, kcp, machine)
			g.Expect(match).To(Equal(tt.expectMatch))
			g.Expect(reason).To(Equal(tt.expectReason))
		})
	}
}

func TestUpToDate(t *testing.T) {
	reconciliationTime := metav1.Now()

	defaultKcp := &controlplanev1.KubeadmControlPlane{
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Replicas: nil,
			Version:  "v1.31.0",
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1", Kind: "AWSMachineTemplate", Name: "template1"},
			},
			KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
					ClusterName: "foo",
				},
			},
			RolloutBefore: &controlplanev1.RolloutBefore{
				CertificatesExpiryDays: ptr.To(int32(60)), // rollout if certificates will expire in less then 60 days.
			},
			RolloutAfter: ptr.To(metav1.Time{Time: reconciliationTime.Add(10 * 24 * time.Hour)}), // rollout 10 days from now.
		},
	}
	defaultMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.Time{Time: reconciliationTime.Add(-2 * 24 * time.Hour)}, // two days ago.
			Annotations: map[string]string{
				controlplanev1.KubeadmClusterConfigurationAnnotation: "{\n  \"clusterName\": \"foo\"\n}",
			},
		},
		Spec: clusterv1.MachineSpec{
			Version:           ptr.To("v1.31.0"),
			InfrastructureRef: corev1.ObjectReference{APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1", Kind: "AWSMachine", Name: "infra-machine1"},
		},
		Status: clusterv1.MachineStatus{
			CertificatesExpiryDate: &metav1.Time{Time: reconciliationTime.Add(100 * 24 * time.Hour)}, // certificates will expire in 100 days from now.
		},
	}

	defaultInfraConfigs := map[string]*unstructured.Unstructured{
		defaultMachine.Name: {
			Object: map[string]interface{}{
				"kind":       "AWSMachine",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": "default",
					"annotations": map[string]interface{}{
						"cluster.x-k8s.io/cloned-from-name":      "template1",
						"cluster.x-k8s.io/cloned-from-groupkind": "AWSMachineTemplate.infrastructure.cluster.x-k8s.io",
					},
				},
			},
		},
	}

	defaultMachineConfigs := map[string]*bootstrapv1.KubeadmConfig{
		defaultMachine.Name: {
			Spec: bootstrapv1.KubeadmConfigSpec{
				InitConfiguration: &bootstrapv1.InitConfiguration{}, // first control-plane
			},
		},
	}

	tests := []struct {
		name                    string
		kcp                     *controlplanev1.KubeadmControlPlane
		machine                 *clusterv1.Machine
		infraConfigs            map[string]*unstructured.Unstructured
		machineConfigs          map[string]*bootstrapv1.KubeadmConfig
		expectUptoDate          bool
		expectLogMessages       []string
		expectConditionMessages []string
	}{
		{
			name:                    "machine up-to-date",
			kcp:                     defaultKcp,
			machine:                 defaultMachine,
			infraConfigs:            defaultInfraConfigs,
			machineConfigs:          defaultMachineConfigs,
			expectUptoDate:          true,
			expectLogMessages:       nil,
			expectConditionMessages: nil,
		},
		{
			name: "certificate are expiring soon",
			kcp: func() *controlplanev1.KubeadmControlPlane {
				kcp := defaultKcp.DeepCopy()
				kcp.Spec.RolloutBefore = &controlplanev1.RolloutBefore{
					CertificatesExpiryDays: ptr.To(int32(150)), // rollout if certificates will expire in less then 150 days.
				}
				return kcp
			}(),
			machine:                 defaultMachine, // certificates will expire in 100 days from now.
			infraConfigs:            defaultInfraConfigs,
			machineConfigs:          defaultMachineConfigs,
			expectUptoDate:          false,
			expectLogMessages:       []string{"certificates will expire soon, rolloutBefore expired"},
			expectConditionMessages: []string{"Certificates will expire soon"},
		},
		{
			name: "rollout after expired",
			kcp: func() *controlplanev1.KubeadmControlPlane {
				kcp := defaultKcp.DeepCopy()
				kcp.Spec.RolloutAfter = ptr.To(metav1.Time{Time: reconciliationTime.Add(-1 * 24 * time.Hour)}) // one day ago
				return kcp
			}(),
			machine:                 defaultMachine, // created two days ago
			infraConfigs:            defaultInfraConfigs,
			machineConfigs:          defaultMachineConfigs,
			expectUptoDate:          false,
			expectLogMessages:       []string{"rolloutAfter expired"},
			expectConditionMessages: []string{"KubeadmControlPlane spec.rolloutAfter expired"},
		},
		{
			name: "kubernetes version does not match",
			kcp: func() *controlplanev1.KubeadmControlPlane {
				kcp := defaultKcp.DeepCopy()
				kcp.Spec.Version = "v1.31.2"
				return kcp
			}(),
			machine:                 defaultMachine, // defaultMachine has "v1.31.0"
			infraConfigs:            defaultInfraConfigs,
			machineConfigs:          defaultMachineConfigs,
			expectUptoDate:          false,
			expectLogMessages:       []string{"Machine version \"v1.31.0\" is not equal to KCP version \"v1.31.2\""},
			expectConditionMessages: []string{"Version v1.31.0, v1.31.2 required"},
		},
		{
			name: "KubeadmConfig is not up-to-date",
			kcp: func() *controlplanev1.KubeadmControlPlane {
				kcp := defaultKcp.DeepCopy()
				kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.ClusterName = "bar"
				return kcp
			}(),
			machine:                 defaultMachine, // was created with cluster name "foo"
			infraConfigs:            defaultInfraConfigs,
			machineConfigs:          defaultMachineConfigs,
			expectUptoDate:          false,
			expectLogMessages:       []string{"Machine KubeadmConfig ClusterConfiguration is outdated: diff: &v1beta1.ClusterConfiguration{\n    ... // 10 identical fields\n    ImageRepository: \"\",\n    FeatureGates:    nil,\n-   ClusterName:     \"foo\",\n+   ClusterName:     \"bar\",\n  }"},
			expectConditionMessages: []string{"KubeadmConfig is not up-to-date"},
		},
		{
			name: "AWSMachine is not up-to-date",
			kcp: func() *controlplanev1.KubeadmControlPlane {
				kcp := defaultKcp.DeepCopy()
				kcp.Spec.MachineTemplate.InfrastructureRef = corev1.ObjectReference{APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1", Kind: "AWSMachineTemplate", Name: "template2"} // kcp moving to template 2
				return kcp
			}(),
			machine:                 defaultMachine,
			infraConfigs:            defaultInfraConfigs, // infra config cloned from template1
			machineConfigs:          defaultMachineConfigs,
			expectUptoDate:          false,
			expectLogMessages:       []string{"Infrastructure template on KCP rotated from AWSMachineTemplate.infrastructure.cluster.x-k8s.io template1 to AWSMachineTemplate.infrastructure.cluster.x-k8s.io template2"},
			expectConditionMessages: []string{"AWSMachine is not up-to-date"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			upToDate, logMessages, conditionMessages, err := UpToDate(tt.machine, tt.kcp, &reconciliationTime, tt.infraConfigs, tt.machineConfigs)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(upToDate).To(Equal(tt.expectUptoDate))
			g.Expect(logMessages).To(Equal(tt.expectLogMessages))
			g.Expect(conditionMessages).To(Equal(tt.expectConditionMessages))
		})
	}
}
