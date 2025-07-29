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
	"fmt"
	"testing"
	"time"

	"github.com/blang/semver/v4"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	kubeadmtypes "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstream"
)

func TestClusterConfigurationAnnotation(t *testing.T) {
	t.Run("ClusterConfigurationToMachineAnnotationValue", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
						APIServer: bootstrapv1.APIServer{
							ExtraArgs: []bootstrapv1.Arg{
								{
									Name:  "foo",
									Value: "bar",
								},
							},
						},
						CertificatesDir: "foo",
					},
				},
			},
		}

		annotations, err := ClusterConfigurationToMachineAnnotationValue(kcp.Spec.KubeadmConfigSpec.ClusterConfiguration)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(annotations).To(Equal("{\"marshalVersion\":\"v1beta2\",\"apiServer\":{\"extraArgs\":[{\"name\":\"foo\",\"value\":\"bar\"}]},\"certificatesDir\":\"foo\"}"))
	})
	t.Run("ClusterConfigurationFromMachineIsOutdated", func(t *testing.T) {
		g := NewWithT(t)

		// Without annotation.
		annotation := ""
		g.Expect(ClusterConfigurationAnnotationFromMachineIsOutdated(annotation)).To(BeTrue())

		// v1beta1 annotation (without marshalVersion)
		annotation = "{\"etcd\":{},\"apiServer\":{\"extraArgs\":{\"foo\":\"bar\"}},\"controllerManager\":{},\"scheduler\":{},\"dns\":{}}"
		g.Expect(ClusterConfigurationAnnotationFromMachineIsOutdated(annotation)).To(BeTrue())

		// up to date annotation (marshalVersion equal to current version)
		annotation = fmt.Sprintf("{\"marshalVersion\":%q,\"etcd\":{},\"apiServer\":{\"extraArgs\":[{\"name\":\"foo\",\"value\":\"bar\"}]},\"controllerManager\":{},\"scheduler\":{},\"dns\":{}}", bootstrapv1.GroupVersion.Version)
		g.Expect(ClusterConfigurationAnnotationFromMachineIsOutdated(annotation)).To(BeFalse())

		// marshalVersion not equal to the current version (this should not happen because marshalVersion has been introduced with the v1beta2 API)
		annotation = "{\"marshalVersion\":\"foo\",\"etcd\":{},\"apiServer\":{\"extraArgs\":[{\"name\":\"foo\",\"value\":\"bar\"}]},\"controllerManager\":{},\"scheduler\":{},\"dns\":{}}"
		g.Expect(ClusterConfigurationAnnotationFromMachineIsOutdated(annotation)).To(BeTrue())
	})
	t.Run("ClusterConfigurationFromMachine", func(t *testing.T) {
		g := NewWithT(t)
		m1 := &clusterv1.Machine{}

		// Without annotation.
		clusterConfiguration, err := ClusterConfigurationFromMachine(m1)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(clusterConfiguration).To(BeNil())

		// v1beta1 annotation (without marshalVersion)
		m1.SetAnnotations(map[string]string{controlplanev1.KubeadmClusterConfigurationAnnotation: "{\"etcd\":{},\"apiServer\":{\"extraArgs\":{\"foo\":\"bar\"}},\"controllerManager\":{},\"scheduler\":{},\"dns\":{},\"certificatesDir\":\"foo\"}"})
		clusterConfiguration, err = ClusterConfigurationFromMachine(m1)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(clusterConfiguration).To(Equal(&bootstrapv1.ClusterConfiguration{
			APIServer: bootstrapv1.APIServer{
				ExtraArgs: []bootstrapv1.Arg{ // Extra args converted from old format to new format.
					{
						Name:  "foo",
						Value: "bar",
					},
				},
			},
			CertificatesDir: "foo",
		}))

		// up to date annotation (marshalVersion equal to current version)
		m1.SetAnnotations(map[string]string{controlplanev1.KubeadmClusterConfigurationAnnotation: fmt.Sprintf("{\"marshalVersion\":%q,\"etcd\":{},\"apiServer\":{\"extraArgs\":[{\"name\":\"foo\",\"value\":\"bar\"}]},\"controllerManager\":{},\"scheduler\":{},\"dns\":{},\"certificatesDir\":\"foo\"}", bootstrapv1.GroupVersion.Version)})
		clusterConfiguration, err = ClusterConfigurationFromMachine(m1)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(clusterConfiguration).To(Equal(&bootstrapv1.ClusterConfiguration{
			APIServer: bootstrapv1.APIServer{
				ExtraArgs: []bootstrapv1.Arg{
					{
						Name:  "foo",
						Value: "bar",
					},
				},
			},
			CertificatesDir: "foo",
		}))
	})
}

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
						CertificatesDir: "foo",
					},
				},
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					controlplanev1.KubeadmClusterConfigurationAnnotation: "{\n  \"certificatesDir\": \"foo\"\n}",
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
						CertificatesDir: "foo",
					},
				},
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					controlplanev1.KubeadmClusterConfigurationAnnotation: "{\n  \"certificatesDir\": \"bar\"\n}",
				},
			},
		}
		match, diff, err := matchClusterConfiguration(kcp, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeFalse())
		g.Expect(diff).To(BeComparableTo(`&v1beta2.ClusterConfiguration{
    ... // 4 identical fields
    Scheduler:       {},
    DNS:             {},
-   CertificatesDir: "bar",
+   CertificatesDir: "foo",
    ImageRepository: "",
    FeatureGates:    nil,
    ... // 2 identical fields
  }`))
	})
	t.Run("Return true if only omittable fields are changed", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
						FeatureGates: map[string]bool{},
					},
				},
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					controlplanev1.KubeadmClusterConfigurationAnnotation: "{}",
				},
			},
		}
		match, diff, err := matchClusterConfiguration(kcp, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeTrue())
		g.Expect(diff).To(BeEmpty())
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
							ExtraArgs: []bootstrapv1.Arg{
								{
									Name:  "foo",
									Value: "bar",
								},
							},
						},
						ControllerManager: bootstrapv1.ControllerManager{
							ExtraArgs: []bootstrapv1.Arg{
								{
									Name:  "foo",
									Value: "bar",
								},
							},
						},
						Scheduler: bootstrapv1.Scheduler{
							ExtraArgs: []bootstrapv1.Arg{
								{
									Name:  "foo",
									Value: "bar",
								},
							},
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
		annotationsCheckPoint := "{\"marshalVersion\":\"v1beta2\",\"apiServer\":{\"extraArgs\":[{\"name\":\"foo\",\"value\":\"bar\"}]},\"controllerManager\":{\"extraArgs\":[{\"name\":\"foo\",\"value\":\"bar\"}]},\"scheduler\":{\"extraArgs\":[{\"name\":\"foo\",\"value\":\"bar\"}]},\"dns\":{\"imageRepository\":\"gcr.io/capi-test\",\"imageTag\":\"v1.10.1\"}}"

		// compute how a serialized ClusterConfiguration looks like now
		annotations, err := ClusterConfigurationToMachineAnnotationValue(kcp.Spec.KubeadmConfigSpec.ClusterConfiguration)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(annotations).To(Equal(annotationsCheckPoint))

		// check the match function detects if a Machine with the annotation string above matches the object it originates from (round trip).
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					controlplanev1.KubeadmClusterConfigurationAnnotation: annotationsCheckPoint,
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
	t.Run("drops omittable fields", func(t *testing.T) {
		g := NewWithT(t)
		kcpConfig := &bootstrapv1.KubeadmConfigSpec{
			JoinConfiguration: &bootstrapv1.JoinConfiguration{
				NodeRegistration: bootstrapv1.NodeRegistrationOptions{
					KubeletExtraArgs: []bootstrapv1.Arg{},
				},
			},
		}
		machineConfig := &bootstrapv1.KubeadmConfig{
			Spec: bootstrapv1.KubeadmConfigSpec{
				JoinConfiguration: &bootstrapv1.JoinConfiguration{
					NodeRegistration: bootstrapv1.NodeRegistrationOptions{
						KubeletExtraArgs: []bootstrapv1.Arg{},
					},
				},
			},
		}
		cleanupConfigFields(kcpConfig, machineConfig)
		g.Expect(kcpConfig.JoinConfiguration.NodeRegistration.KubeletExtraArgs).To(BeNil())
		g.Expect(machineConfig.Spec.JoinConfiguration.NodeRegistration.KubeletExtraArgs).To(BeNil())
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
				Kind:       "Machine",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
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
					InitConfiguration:    nil,
					JoinConfiguration:    &bootstrapv1.JoinConfiguration{},
				},
			},
		}
		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Machine",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
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
				Kind:       "Machine",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
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
				Kind:       "Machine",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
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
		g.Expect(diff).To(BeComparableTo(`&v1beta2.KubeadmConfigSpec{
    ClusterConfiguration: nil,
    InitConfiguration: &v1beta2.InitConfiguration{
      BootstrapTokens: nil,
      NodeRegistration: v1beta2.NodeRegistrationOptions{
-       Name:      "",
+       Name:      "A new name",
        CRISocket: "",
        Taints:    nil,
        ... // 4 identical fields
      },
      LocalAPIEndpoint: {},
      SkipPhases:       nil,
      ... // 2 identical fields
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
				Kind:       "Machine",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
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
				Kind:       "Machine",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
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
		g.Expect(diff).To(BeComparableTo(`&v1beta2.KubeadmConfigSpec{
    ClusterConfiguration: nil,
    InitConfiguration:    nil,
    JoinConfiguration: &v1beta2.JoinConfiguration{
      NodeRegistration: v1beta2.NodeRegistrationOptions{
-       Name:      "",
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
    DiskSetup: nil,
    ... // 9 identical fields
  }`))
	})
	t.Run("returns true if returns true if only omittable configurations are not equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    &bootstrapv1.InitConfiguration{},
					JoinConfiguration:    &bootstrapv1.JoinConfiguration{},
					Files:                []bootstrapv1.File{}, // This is a change, but it is an omittable field and the diff between nil and empty array is not relevant.
				},
			},
		}
		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Machine",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
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
	t.Run("returns false if some other configurations are not equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    &bootstrapv1.InitConfiguration{},
					JoinConfiguration:    &bootstrapv1.JoinConfiguration{},
					Files:                []bootstrapv1.File{{Path: "/tmp/foo"}}, // This is a change
				},
			},
		}
		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Machine",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
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
		g.Expect(diff).To(BeComparableTo(`&v1beta2.KubeadmConfigSpec{
    ClusterConfiguration: nil,
    InitConfiguration:    &{NodeRegistration: {ImagePullPolicy: "IfNotPresent"}},
    JoinConfiguration:    nil,
-   Files:                nil,
+   Files:                []v1beta2.File{{Path: "/tmp/foo"}},
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
						CertificatesDir: "foo",
					},
				},
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					controlplanev1.KubeadmClusterConfigurationAnnotation: "{\n  \"certificatesDir\": \"foo\"\n}",
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
						CertificatesDir: "foo",
					},
				},
			},
		}
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					controlplanev1.KubeadmClusterConfigurationAnnotation: "{\n  \"certificatesDir\": \"bar\"\n}",
				},
			},
		}
		machineConfigs := map[string]*bootstrapv1.KubeadmConfig{
			m.Name: {},
		}
		reason, match, err := matchesKubeadmBootstrapConfig(machineConfigs, kcp, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeFalse())
		g.Expect(reason).To(BeComparableTo(`Machine KubeadmConfig ClusterConfiguration is outdated: diff: &v1beta2.ClusterConfiguration{
    ... // 4 identical fields
    Scheduler:       {},
    DNS:             {},
-   CertificatesDir: "bar",
+   CertificatesDir: "foo",
    ImageRepository: "",
    FeatureGates:    nil,
    ... // 2 identical fields
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
				Kind:       "Machine",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
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
				Kind:       "Machine",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
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
		g.Expect(reason).To(BeComparableTo(`Machine KubeadmConfig InitConfiguration or JoinConfiguration are outdated: diff: &v1beta2.KubeadmConfigSpec{
    ClusterConfiguration: nil,
    InitConfiguration: &v1beta2.InitConfiguration{
      BootstrapTokens: nil,
      NodeRegistration: v1beta2.NodeRegistrationOptions{
-       Name:      "",
+       Name:      "foo",
        CRISocket: "",
        Taints:    nil,
        ... // 4 identical fields
      },
      LocalAPIEndpoint: {},
      SkipPhases:       nil,
      ... // 2 identical fields
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
				Kind:       "Machine",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
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
				Kind:       "Machine",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
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
		g.Expect(reason).To(BeComparableTo(`Machine KubeadmConfig InitConfiguration or JoinConfiguration are outdated: diff: &v1beta2.KubeadmConfigSpec{
    ClusterConfiguration: nil,
    InitConfiguration:    nil,
    JoinConfiguration: &v1beta2.JoinConfiguration{
      NodeRegistration: v1beta2.NodeRegistrationOptions{
-       Name:      "",
+       Name:      "foo",
        CRISocket: "",
        Taints:    nil,
        ... // 4 identical fields
      },
      CACertPath: "",
      Discovery:  {},
      ... // 4 identical fields
    },
    Files:     nil,
    DiskSetup: nil,
    ... // 9 identical fields
  }`))
	})
	t.Run("returns true if only omittable configurations are not equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    &bootstrapv1.InitConfiguration{},
					JoinConfiguration:    &bootstrapv1.JoinConfiguration{},
					Files:                []bootstrapv1.File{}, // This is a change, but it is an omittable field and the diff between nil and empty array is not relevant.
				},
			},
		}
		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Machine",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
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
	t.Run("returns false if some other configurations are not equal", func(t *testing.T) {
		g := NewWithT(t)
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
					ClusterConfiguration: &bootstrapv1.ClusterConfiguration{},
					InitConfiguration:    &bootstrapv1.InitConfiguration{},
					JoinConfiguration:    &bootstrapv1.JoinConfiguration{},
					Files:                []bootstrapv1.File{{Path: "/tmp/foo"}}, // This is a change
				},
			},
		}
		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Machine",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
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
		g.Expect(reason).To(BeComparableTo(`Machine KubeadmConfig InitConfiguration or JoinConfiguration are outdated: diff: &v1beta2.KubeadmConfigSpec{
    ClusterConfiguration: nil,
    InitConfiguration:    &{NodeRegistration: {ImagePullPolicy: "IfNotPresent"}},
    JoinConfiguration:    nil,
-   Files:                nil,
+   Files:                []v1beta2.File{{Path: "/tmp/foo"}},
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
				Kind:       "Machine",
				APIVersion: clusterv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &clusterv1.ContractVersionedObjectReference{
						Kind:     "KubeadmConfig",
						Name:     "test",
						APIGroup: bootstrapv1.GroupVersion.Group,
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
		g.Expect(reason).To(Equal("Machine cannot be compared with KCP.spec.machineTemplate.spec.infrastructureRef: Machine is nil"))
	})

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
					Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
						InfrastructureRef: clusterv1.ContractVersionedObjectReference{
							Kind:     "GenericMachineTemplate",
							Name:     "infra-foo",
							APIGroup: "generic.io",
						},
					},
				},
			},
		}
		m := &clusterv1.Machine{
			Spec: clusterv1.MachineSpec{
				InfrastructureRef: clusterv1.ContractVersionedObjectReference{
					Kind:     "GenericMachine",
					Name:     "infra-foo",
					APIGroup: "generic.io",
				},
			},
		}

		infraConfigs := map[string]*unstructured.Unstructured{
			m.Name: {
				Object: map[string]interface{}{
					"kind":       "InfrastructureMachine",
					"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
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
				Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "GenericMachineTemplate",
						Name:     "infra-foo",
						APIGroup: "generic.io",
					},
				},
			},
		},
	}
	machine := &clusterv1.Machine{
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: clusterv1.GroupVersionInfrastructure.Group,
				Kind:     "InfrastructureMachine",
				Name:     "infra-config1",
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
						"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
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
				Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: clusterv1.GroupVersionInfrastructure.Group,
						Kind:     "AWSMachineTemplate",
						Name:     "template1",
					},
				},
			},
			KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
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
	defaultMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.Time{Time: reconciliationTime.Add(-2 * 24 * time.Hour)}, // two days ago.
			Annotations: map[string]string{
				controlplanev1.KubeadmClusterConfigurationAnnotation: "{\n  \"certificatesDir\": \"foo\"\n}",
			},
		},
		Spec: clusterv1.MachineSpec{
			Version:           "v1.31.0",
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{APIGroup: clusterv1.GroupVersionInfrastructure.Group, Kind: "AWSMachine", Name: "infra-machine1"},
		},
		Status: clusterv1.MachineStatus{
			CertificatesExpiryDate: metav1.Time{Time: reconciliationTime.Add(100 * 24 * time.Hour)}, // certificates will expire in 100 days from now.
		},
	}

	defaultInfraConfigs := map[string]*unstructured.Unstructured{
		defaultMachine.Name: {
			Object: map[string]interface{}{
				"kind":       "AWSMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
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
				kcp.Spec.Rollout.Before.CertificatesExpiryDays = 150 // rollout if certificates will expire in less then 150 days.
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
				kcp.Spec.Rollout.After = metav1.Time{Time: reconciliationTime.Add(-1 * 24 * time.Hour)} // one day ago
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
				kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.CertificatesDir = "bar"
				return kcp
			}(),
			machine:                 defaultMachine, // was created with cluster name "foo"
			infraConfigs:            defaultInfraConfigs,
			machineConfigs:          defaultMachineConfigs,
			expectUptoDate:          false,
			expectLogMessages:       []string{"Machine KubeadmConfig ClusterConfiguration is outdated: diff: &v1beta2.ClusterConfiguration{\n    ... // 4 identical fields\n    Scheduler:       {},\n    DNS:             {},\n-   CertificatesDir: \"foo\",\n+   CertificatesDir: \"bar\",\n    ImageRepository: \"\",\n    FeatureGates:    nil,\n    ... // 2 identical fields\n  }"},
			expectConditionMessages: []string{"KubeadmConfig is not up-to-date"},
		},
		{
			name: "AWSMachine is not up-to-date",
			kcp: func() *controlplanev1.KubeadmControlPlane {
				kcp := defaultKcp.DeepCopy()
				kcp.Spec.MachineTemplate.Spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference{APIGroup: clusterv1.GroupVersionInfrastructure.Group, Kind: "AWSMachineTemplate", Name: "template2"} // kcp moving to template 2
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

func TestOmittableFieldsClusterConfiguration(t *testing.T) {
	tests := []struct {
		name string
		A    *bootstrapv1.ClusterConfiguration
		B    *bootstrapv1.ClusterConfiguration
	}{
		{
			name: "Test omittable fields",
			A: &bootstrapv1.ClusterConfiguration{
				Etcd: bootstrapv1.Etcd{
					Local: &bootstrapv1.LocalEtcd{
						ExtraArgs:      []bootstrapv1.Arg{},
						ExtraEnvs:      []bootstrapv1.EnvVar{},
						ServerCertSANs: []string{},
						PeerCertSANs:   []string{},
					},
					External: &bootstrapv1.ExternalEtcd{
						Endpoints: []string{},
					},
				},
				APIServer: bootstrapv1.APIServer{
					ExtraArgs:    []bootstrapv1.Arg{},
					ExtraVolumes: []bootstrapv1.HostPathMount{},
					ExtraEnvs:    []bootstrapv1.EnvVar{},
					CertSANs:     []string{},
				},
				ControllerManager: bootstrapv1.ControllerManager{
					ExtraArgs:    []bootstrapv1.Arg{},
					ExtraVolumes: []bootstrapv1.HostPathMount{},
					ExtraEnvs:    []bootstrapv1.EnvVar{},
				},
				Scheduler: bootstrapv1.Scheduler{
					ExtraArgs:    []bootstrapv1.Arg{},
					ExtraVolumes: []bootstrapv1.HostPathMount{},
					ExtraEnvs:    []bootstrapv1.EnvVar{},
				},
				FeatureGates: map[string]bool{},
			},
			B: &bootstrapv1.ClusterConfiguration{
				Etcd: bootstrapv1.Etcd{
					Local: &bootstrapv1.LocalEtcd{
						ExtraArgs:      nil,
						ExtraEnvs:      nil,
						ServerCertSANs: nil,
						PeerCertSANs:   nil,
					},
					External: &bootstrapv1.ExternalEtcd{
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

			gotA, err := kubeadmtypes.MarshalClusterConfigurationForVersion(tt.A, semver.MustParse("1.99.0"), &upstream.AdditionalData{}) // we want to test with latest kubeadm API version.
			g.Expect(err).ToNot(HaveOccurred())

			gotB, err := kubeadmtypes.MarshalClusterConfigurationForVersion(tt.B, semver.MustParse("1.99.0"), &upstream.AdditionalData{}) // we want to test with latest kubeadm API version.
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(gotA).To(Equal(gotB), cmp.Diff(gotA, gotB))

			dropOmittableFields(&bootstrapv1.KubeadmConfigSpec{ClusterConfiguration: tt.A})
			dropOmittableFields(&bootstrapv1.KubeadmConfigSpec{ClusterConfiguration: tt.B})
			g.Expect(tt.A).To(Equal(tt.B))
		})
	}
}

func TestOmittableFieldsInitConfiguration(t *testing.T) {
	tests := []struct {
		name string
		A    *bootstrapv1.InitConfiguration
		B    *bootstrapv1.InitConfiguration
	}{
		{
			name: "Test omittable fields",
			A: &bootstrapv1.InitConfiguration{
				BootstrapTokens: []bootstrapv1.BootstrapToken{},
				NodeRegistration: bootstrapv1.NodeRegistrationOptions{
					Taints:                ptr.To([]corev1.Taint{}),
					KubeletExtraArgs:      []bootstrapv1.Arg{},
					IgnorePreflightErrors: []string{},
				},
				SkipPhases: []string{},
			},
			B: &bootstrapv1.InitConfiguration{
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
			A: &bootstrapv1.InitConfiguration{
				BootstrapTokens: []bootstrapv1.BootstrapToken{
					{
						Usages: []string{},
						Groups: []string{},
					},
				},
			},
			B: &bootstrapv1.InitConfiguration{
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

			gotA, err := kubeadmtypes.MarshalInitConfigurationForVersion(tt.A, semver.MustParse("1.99.0")) // we want to test with latest kubeadm API version.
			g.Expect(err).ToNot(HaveOccurred())

			gotB, err := kubeadmtypes.MarshalInitConfigurationForVersion(tt.B, semver.MustParse("1.99.0")) // we want to test with latest kubeadm API version.
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(gotA).To(Equal(gotB), cmp.Diff(gotA, gotB))

			dropOmittableFields(&bootstrapv1.KubeadmConfigSpec{InitConfiguration: tt.A})
			dropOmittableFields(&bootstrapv1.KubeadmConfigSpec{InitConfiguration: tt.B})
			g.Expect(tt.A).To(Equal(tt.B))
		})
	}
}

func TestOmittableFieldsJoinConfiguration(t *testing.T) {
	tests := []struct {
		name string
		A    *bootstrapv1.JoinConfiguration
		B    *bootstrapv1.JoinConfiguration
	}{
		{
			name: "Test omittable fields",
			A: &bootstrapv1.JoinConfiguration{
				NodeRegistration: bootstrapv1.NodeRegistrationOptions{
					Taints:                ptr.To([]corev1.Taint{}),
					KubeletExtraArgs:      []bootstrapv1.Arg{},
					IgnorePreflightErrors: []string{},
				},
				Discovery: bootstrapv1.Discovery{
					BootstrapToken: &bootstrapv1.BootstrapTokenDiscovery{
						CACertHashes: []string{},
					},
					File: &bootstrapv1.FileDiscovery{
						KubeConfig: &bootstrapv1.FileDiscoveryKubeConfig{
							Cluster: &bootstrapv1.KubeConfigCluster{
								CertificateAuthorityData: []byte{},
							},
							User: bootstrapv1.KubeConfigUser{
								AuthProvider: &bootstrapv1.KubeConfigAuthProvider{
									Config: map[string]string{},
								},
								Exec: &bootstrapv1.KubeConfigAuthExec{
									Args: []string{},
									Env:  []bootstrapv1.KubeConfigAuthExecEnv{},
								},
							},
						},
					},
				},
				SkipPhases: []string{},
			},
			B: &bootstrapv1.JoinConfiguration{
				NodeRegistration: bootstrapv1.NodeRegistrationOptions{
					Taints:                ptr.To([]corev1.Taint{}), // Special serialization, i.e. intentionally a pointer to a slice to preserve []
					KubeletExtraArgs:      nil,
					IgnorePreflightErrors: nil,
				},
				Discovery: bootstrapv1.Discovery{
					BootstrapToken: &bootstrapv1.BootstrapTokenDiscovery{
						CACertHashes: nil,
					},
					File: &bootstrapv1.FileDiscovery{
						KubeConfig: &bootstrapv1.FileDiscoveryKubeConfig{
							Cluster: &bootstrapv1.KubeConfigCluster{
								CertificateAuthorityData: nil,
							},
							User: bootstrapv1.KubeConfigUser{
								AuthProvider: &bootstrapv1.KubeConfigAuthProvider{
									Config: nil,
								},
								Exec: &bootstrapv1.KubeConfigAuthExec{
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
		{
			name: "Test omittable struct",
			A: &bootstrapv1.JoinConfiguration{
				Discovery: bootstrapv1.Discovery{
					File: &bootstrapv1.FileDiscovery{
						KubeConfig: &bootstrapv1.FileDiscoveryKubeConfig{
							Cluster: &bootstrapv1.KubeConfigCluster{},
							User: bootstrapv1.KubeConfigUser{
								AuthProvider: &bootstrapv1.KubeConfigAuthProvider{},
								Exec:         &bootstrapv1.KubeConfigAuthExec{},
							},
						},
					},
				},
			},
			B: &bootstrapv1.JoinConfiguration{
				Discovery: bootstrapv1.Discovery{
					File: &bootstrapv1.FileDiscovery{
						KubeConfigPath: "",
						KubeConfig: &bootstrapv1.FileDiscoveryKubeConfig{
							Cluster: nil,
							User: bootstrapv1.KubeConfigUser{
								AuthProvider: nil,
								Exec:         nil,
							},
						},
					},
				},
			},
		},
		{
			name: "Test omittable struct",
			A: &bootstrapv1.JoinConfiguration{
				Discovery: bootstrapv1.Discovery{
					File: &bootstrapv1.FileDiscovery{
						KubeConfig: &bootstrapv1.FileDiscoveryKubeConfig{},
					},
				},
			},
			B: &bootstrapv1.JoinConfiguration{
				Discovery: bootstrapv1.Discovery{
					File: &bootstrapv1.FileDiscovery{
						KubeConfig: nil,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gotA, err := kubeadmtypes.MarshalJoinConfigurationForVersion(tt.A, semver.MustParse("1.99.0")) // we want to test with latest kubeadm API version.
			g.Expect(err).ToNot(HaveOccurred())

			gotB, err := kubeadmtypes.MarshalJoinConfigurationForVersion(tt.B, semver.MustParse("1.99.0")) // we want to test with latest kubeadm API version.
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(gotA).To(Equal(gotB), cmp.Diff(gotA, gotB))

			dropOmittableFields(&bootstrapv1.KubeadmConfigSpec{JoinConfiguration: tt.A})
			dropOmittableFields(&bootstrapv1.KubeadmConfigSpec{JoinConfiguration: tt.B})
			g.Expect(tt.A).To(Equal(tt.B))
		})
	}
}
