/*
Copyright 2021 The Kubernetes Authors.

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
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/webhooks/util"
)

var (
	invalidNamespaceName = "bar"
	ctx                  = ctrl.SetupSignalHandler()
)

func TestKubeadmControlPlaneDefault(t *testing.T) {
	g := NewWithT(t)

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo",
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.18.3",
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "test/v1alpha1",
					Kind:       "UnknownInfraMachine",
					Name:       "foo",
				},
			},
			RolloutStrategy: &controlplanev1.RolloutStrategy{},
		},
	}
	updateDefaultingValidationKCP := kcp.DeepCopy()
	updateDefaultingValidationKCP.Spec.Version = "v1.18.3"
	updateDefaultingValidationKCP.Spec.MachineTemplate.InfrastructureRef = corev1.ObjectReference{
		APIVersion: "test/v1alpha1",
		Kind:       "UnknownInfraMachine",
		Name:       "foo",
		Namespace:  "foo",
	}
	webhook := &KubeadmControlPlane{}
	t.Run("for KubeadmControlPlane", util.CustomDefaultValidateTest(ctx, updateDefaultingValidationKCP, webhook))
	g.Expect(webhook.Default(ctx, kcp)).To(Succeed())

	g.Expect(kcp.Spec.KubeadmConfigSpec.Format).To(Equal(bootstrapv1.CloudConfig))
	g.Expect(kcp.Spec.MachineTemplate.InfrastructureRef.Namespace).To(Equal(kcp.Namespace))
	g.Expect(kcp.Spec.Version).To(Equal("v1.18.3"))
	g.Expect(kcp.Spec.RolloutStrategy.Type).To(Equal(controlplanev1.RollingUpdateStrategyType))
	g.Expect(kcp.Spec.RolloutStrategy.RollingUpdate.MaxSurge.IntVal).To(Equal(int32(1)))
}

func TestKubeadmControlPlaneValidateCreate(t *testing.T) {
	valid := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "foo",
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "test/v1alpha1",
					Kind:       "UnknownInfraMachine",
					Namespace:  "foo",
					Name:       "infraTemplate",
				},
			},
			KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{},
			},
			Replicas: ptr.To[int32](1),
			Version:  "v1.19.0",
			RolloutStrategy: &controlplanev1.RolloutStrategy{
				Type: controlplanev1.RollingUpdateStrategyType,
				RollingUpdate: &controlplanev1.RollingUpdate{
					MaxSurge: &intstr.IntOrString{
						IntVal: 1,
					},
				},
			},
		},
	}

	invalidMaxSurge := valid.DeepCopy()
	invalidMaxSurge.Spec.RolloutStrategy.RollingUpdate.MaxSurge.IntVal = int32(3)

	stringMaxSurge := valid.DeepCopy()
	val := intstr.FromString("1")
	stringMaxSurge.Spec.RolloutStrategy.RollingUpdate.MaxSurge = &val

	invalidNamespace := valid.DeepCopy()
	invalidNamespace.Spec.MachineTemplate.InfrastructureRef.Namespace = invalidNamespaceName

	missingReplicas := valid.DeepCopy()
	missingReplicas.Spec.Replicas = nil

	zeroReplicas := valid.DeepCopy()
	zeroReplicas.Spec.Replicas = ptr.To[int32](0)

	evenReplicas := valid.DeepCopy()
	evenReplicas.Spec.Replicas = ptr.To[int32](2)

	evenReplicasExternalEtcd := evenReplicas.DeepCopy()
	evenReplicasExternalEtcd.Spec.KubeadmConfigSpec = bootstrapv1.KubeadmConfigSpec{
		ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
			Etcd: bootstrapv1.Etcd{
				External: &bootstrapv1.ExternalEtcd{},
			},
		},
	}

	validVersion := valid.DeepCopy()
	validVersion.Spec.Version = "v1.16.6"

	invalidVersion1 := valid.DeepCopy()
	invalidVersion1.Spec.Version = "vv1.16.6"

	invalidVersion2 := valid.DeepCopy()
	invalidVersion2.Spec.Version = "1.16.6"

	invalidCoreDNSVersion := valid.DeepCopy()
	invalidCoreDNSVersion.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS.ImageTag = "v1.7" // not a valid semantic version

	invalidRolloutBeforeCertificateExpiryDays := valid.DeepCopy()
	invalidRolloutBeforeCertificateExpiryDays.Spec.RolloutBefore = &controlplanev1.RolloutBefore{
		CertificatesExpiryDays: ptr.To[int32](5), // less than minimum
	}

	invalidIgnitionConfiguration := valid.DeepCopy()
	invalidIgnitionConfiguration.Spec.KubeadmConfigSpec.Ignition = &bootstrapv1.IgnitionSpec{}

	validIgnitionConfiguration := valid.DeepCopy()
	validIgnitionConfiguration.Spec.KubeadmConfigSpec.Format = bootstrapv1.Ignition
	validIgnitionConfiguration.Spec.KubeadmConfigSpec.Ignition = &bootstrapv1.IgnitionSpec{}

	invalidMetadata := valid.DeepCopy()
	invalidMetadata.Spec.MachineTemplate.ObjectMeta.Labels = map[string]string{
		"foo":          "$invalid-key",
		"bar":          strings.Repeat("a", 64) + "too-long-value",
		"/invalid-key": "foo",
	}
	invalidMetadata.Spec.MachineTemplate.ObjectMeta.Annotations = map[string]string{
		"/invalid-key": "foo",
	}

	tests := []struct {
		name                  string
		enableIgnitionFeature bool
		expectErr             bool
		kcp                   *controlplanev1.KubeadmControlPlane
	}{
		{
			name:      "should succeed when given a valid config",
			expectErr: false,
			kcp:       valid,
		},
		{
			name:      "should return error when kubeadmControlPlane namespace and infrastructureTemplate  namespace mismatch",
			expectErr: true,
			kcp:       invalidNamespace,
		},
		{
			name:      "should return error when replicas is nil",
			expectErr: true,
			kcp:       missingReplicas,
		},
		{
			name:      "should return error when replicas is zero",
			expectErr: true,
			kcp:       zeroReplicas,
		},
		{
			name:      "should return error when replicas is even",
			expectErr: true,
			kcp:       evenReplicas,
		},
		{
			name:      "should allow even replicas when using external etcd",
			expectErr: false,
			kcp:       evenReplicasExternalEtcd,
		},
		{
			name:      "should succeed when given a valid semantic version with prepended 'v'",
			expectErr: false,
			kcp:       validVersion,
		},
		{
			name:      "should error when given a valid semantic version without 'v'",
			expectErr: true,
			kcp:       invalidVersion2,
		},
		{
			name:      "should return error when given an invalid semantic version",
			expectErr: true,
			kcp:       invalidVersion1,
		},
		{
			name:      "should return error when given an invalid semantic CoreDNS version",
			expectErr: true,
			kcp:       invalidCoreDNSVersion,
		},
		{
			name:      "should return error when maxSurge is not 1",
			expectErr: true,
			kcp:       invalidMaxSurge,
		},
		{
			name:      "should succeed when maxSurge is a string",
			expectErr: false,
			kcp:       stringMaxSurge,
		},
		{
			name:      "should return error when given an invalid rolloutBefore.certificatesExpiryDays value",
			expectErr: true,
			kcp:       invalidRolloutBeforeCertificateExpiryDays,
		},

		{
			name:                  "should return error when Ignition configuration is invalid",
			enableIgnitionFeature: true,
			expectErr:             true,
			kcp:                   invalidIgnitionConfiguration,
		},
		{
			name:                  "should succeed when Ignition configuration is valid",
			enableIgnitionFeature: true,
			expectErr:             false,
			kcp:                   validIgnitionConfiguration,
		},
		{
			name:                  "should return error for invalid metadata",
			enableIgnitionFeature: true,
			expectErr:             true,
			kcp:                   invalidMetadata,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.enableIgnitionFeature {
				// NOTE: KubeadmBootstrapFormatIgnition feature flag is disabled by default.
				// Enabling the feature flag temporarily for this test.
				defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.KubeadmBootstrapFormatIgnition, true)()
			}

			g := NewWithT(t)

			webhook := &KubeadmControlPlane{}

			warnings, err := webhook.ValidateCreate(ctx, tt.kcp)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(warnings).To(BeEmpty())
		})
	}
}

func TestKubeadmControlPlaneValidateUpdate(t *testing.T) {
	before := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "foo",
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "test/v1alpha1",
					Kind:       "UnknownInfraMachine",
					Namespace:  "foo",
					Name:       "infraTemplate",
				},
				NodeDrainTimeout:        &metav1.Duration{Duration: time.Second},
				NodeVolumeDetachTimeout: &metav1.Duration{Duration: time.Second},
				NodeDeletionTimeout:     &metav1.Duration{Duration: time.Second},
			},
			Replicas: ptr.To[int32](1),
			RolloutStrategy: &controlplanev1.RolloutStrategy{
				Type: controlplanev1.RollingUpdateStrategyType,
				RollingUpdate: &controlplanev1.RollingUpdate{
					MaxSurge: &intstr.IntOrString{
						IntVal: 1,
					},
				},
			},
			KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
				InitConfiguration: &bootstrapv1.InitConfiguration{
					LocalAPIEndpoint: bootstrapv1.APIEndpoint{
						AdvertiseAddress: "127.0.0.1",
						BindPort:         int32(443),
					},
					NodeRegistration: bootstrapv1.NodeRegistrationOptions{
						Name: "test",
					},
				},
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
					ClusterName: "test",
					DNS: bootstrapv1.DNS{
						ImageMeta: bootstrapv1.ImageMeta{
							ImageRepository: "registry.k8s.io/coredns",
							ImageTag:        "1.6.5",
						},
					},
				},
				JoinConfiguration: &bootstrapv1.JoinConfiguration{
					Discovery: bootstrapv1.Discovery{
						Timeout: &metav1.Duration{
							Duration: 10 * time.Minute,
						},
					},
					NodeRegistration: bootstrapv1.NodeRegistrationOptions{
						Name: "test",
					},
				},
				PreKubeadmCommands: []string{
					"test", "foo",
				},
				PostKubeadmCommands: []string{
					"test", "foo",
				},
				Files: []bootstrapv1.File{
					{
						Path: "test",
					},
				},
				Users: []bootstrapv1.User{
					{
						Name: "user",
						SSHAuthorizedKeys: []string{
							"ssh-rsa foo",
						},
					},
				},
				NTP: &bootstrapv1.NTP{
					Servers: []string{"test-server-1", "test-server-2"},
					Enabled: ptr.To(true),
				},
			},
			Version: "v1.16.6",
			RolloutBefore: &controlplanev1.RolloutBefore{
				CertificatesExpiryDays: ptr.To[int32](7),
			},
		},
	}

	updateMaxSurgeVal := before.DeepCopy()
	updateMaxSurgeVal.Spec.RolloutStrategy.RollingUpdate.MaxSurge.IntVal = int32(0)
	updateMaxSurgeVal.Spec.Replicas = ptr.To[int32](3)

	wrongReplicaCountForScaleIn := before.DeepCopy()
	wrongReplicaCountForScaleIn.Spec.RolloutStrategy.RollingUpdate.MaxSurge.IntVal = int32(0)

	validUpdateKubeadmConfigInit := before.DeepCopy()
	validUpdateKubeadmConfigInit.Spec.KubeadmConfigSpec.InitConfiguration.NodeRegistration = bootstrapv1.NodeRegistrationOptions{}

	invalidUpdateKubeadmConfigCluster := before.DeepCopy()
	invalidUpdateKubeadmConfigCluster.Spec.KubeadmConfigSpec.ClusterConfiguration = &bootstrapv1.ClusterConfiguration{}

	validUpdateKubeadmConfigJoin := before.DeepCopy()
	validUpdateKubeadmConfigJoin.Spec.KubeadmConfigSpec.JoinConfiguration.NodeRegistration = bootstrapv1.NodeRegistrationOptions{}

	beforeKubeadmConfigFormatSet := before.DeepCopy()
	beforeKubeadmConfigFormatSet.Spec.KubeadmConfigSpec.Format = bootstrapv1.CloudConfig
	invalidUpdateKubeadmConfigFormat := beforeKubeadmConfigFormatSet.DeepCopy()
	invalidUpdateKubeadmConfigFormat.Spec.KubeadmConfigSpec.Format = bootstrapv1.Ignition

	validUpdate := before.DeepCopy()
	validUpdate.Labels = map[string]string{"blue": "green"}
	validUpdate.Spec.KubeadmConfigSpec.PreKubeadmCommands = []string{"ab", "abc"}
	validUpdate.Spec.KubeadmConfigSpec.PostKubeadmCommands = []string{"ab", "abc"}
	validUpdate.Spec.KubeadmConfigSpec.Files = []bootstrapv1.File{
		{
			Path: "ab",
		},
		{
			Path: "abc",
		},
	}
	validUpdate.Spec.Version = "v1.17.1"
	validUpdate.Spec.KubeadmConfigSpec.Users = []bootstrapv1.User{
		{
			Name: "bar",
			SSHAuthorizedKeys: []string{
				"ssh-rsa bar",
				"ssh-rsa foo",
			},
		},
	}
	validUpdate.Spec.MachineTemplate.ObjectMeta.Labels = map[string]string{
		"label": "labelValue",
	}
	validUpdate.Spec.MachineTemplate.ObjectMeta.Annotations = map[string]string{
		"annotation": "labelAnnotation",
	}
	validUpdate.Spec.MachineTemplate.InfrastructureRef.APIVersion = "test/v1alpha2"
	validUpdate.Spec.MachineTemplate.InfrastructureRef.Name = "orange"
	validUpdate.Spec.MachineTemplate.NodeDrainTimeout = &metav1.Duration{Duration: 10 * time.Second}
	validUpdate.Spec.MachineTemplate.NodeVolumeDetachTimeout = &metav1.Duration{Duration: 10 * time.Second}
	validUpdate.Spec.MachineTemplate.NodeDeletionTimeout = &metav1.Duration{Duration: 10 * time.Second}
	validUpdate.Spec.Replicas = ptr.To[int32](5)
	now := metav1.NewTime(time.Now())
	validUpdate.Spec.RolloutAfter = &now
	validUpdate.Spec.RolloutBefore = &controlplanev1.RolloutBefore{
		CertificatesExpiryDays: ptr.To[int32](14),
	}
	validUpdate.Spec.RemediationStrategy = &controlplanev1.RemediationStrategy{
		MaxRetry:         ptr.To[int32](50),
		MinHealthyPeriod: &metav1.Duration{Duration: 10 * time.Hour},
		RetryPeriod:      metav1.Duration{Duration: 10 * time.Minute},
	}
	validUpdate.Spec.KubeadmConfigSpec.Format = bootstrapv1.CloudConfig

	scaleToZero := before.DeepCopy()
	scaleToZero.Spec.Replicas = ptr.To[int32](0)

	scaleToEven := before.DeepCopy()
	scaleToEven.Spec.Replicas = ptr.To[int32](2)

	invalidNamespace := before.DeepCopy()
	invalidNamespace.Spec.MachineTemplate.InfrastructureRef.Namespace = invalidNamespaceName

	missingReplicas := before.DeepCopy()
	missingReplicas.Spec.Replicas = nil

	etcdLocalImageTag := before.DeepCopy()
	etcdLocalImageTag.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local = &bootstrapv1.LocalEtcd{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageTag: "v9.1.1",
		},
	}

	etcdLocalImageBuildTag := before.DeepCopy()
	etcdLocalImageBuildTag.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local = &bootstrapv1.LocalEtcd{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageTag: "v9.1.1_validBuild1",
		},
	}

	etcdLocalImageInvalidTag := before.DeepCopy()
	etcdLocalImageInvalidTag.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local = &bootstrapv1.LocalEtcd{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageTag: "v9.1.1+invalidBuild1",
		},
	}

	unsetEtcd := etcdLocalImageTag.DeepCopy()
	unsetEtcd.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local = nil

	networking := before.DeepCopy()
	networking.Spec.KubeadmConfigSpec.ClusterConfiguration.Networking.DNSDomain = "some dns domain"

	kubernetesVersion := before.DeepCopy()
	kubernetesVersion.Spec.KubeadmConfigSpec.ClusterConfiguration.KubernetesVersion = "some kubernetes version"

	controlPlaneEndpoint := before.DeepCopy()
	controlPlaneEndpoint.Spec.KubeadmConfigSpec.ClusterConfiguration.ControlPlaneEndpoint = "some control plane endpoint"

	apiServer := before.DeepCopy()
	apiServer.Spec.KubeadmConfigSpec.ClusterConfiguration.APIServer = bootstrapv1.APIServer{
		ControlPlaneComponent: bootstrapv1.ControlPlaneComponent{
			ExtraArgs:    map[string]string{"foo": "bar"},
			ExtraVolumes: []bootstrapv1.HostPathMount{{Name: "mount1"}},
		},
		TimeoutForControlPlane: &metav1.Duration{Duration: 5 * time.Minute},
		CertSANs:               []string{"foo", "bar"},
	}

	controllerManager := before.DeepCopy()
	controllerManager.Spec.KubeadmConfigSpec.ClusterConfiguration.ControllerManager = bootstrapv1.ControlPlaneComponent{
		ExtraArgs:    map[string]string{"controller manager field": "controller manager value"},
		ExtraVolumes: []bootstrapv1.HostPathMount{{Name: "mount", HostPath: "/foo", MountPath: "bar", ReadOnly: true, PathType: "File"}},
	}

	scheduler := before.DeepCopy()
	scheduler.Spec.KubeadmConfigSpec.ClusterConfiguration.Scheduler = bootstrapv1.ControlPlaneComponent{
		ExtraArgs:    map[string]string{"scheduler field": "scheduler value"},
		ExtraVolumes: []bootstrapv1.HostPathMount{{Name: "mount", HostPath: "/foo", MountPath: "bar", ReadOnly: true, PathType: "File"}},
	}

	dns := before.DeepCopy()
	dns.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS = bootstrapv1.DNS{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageRepository: "gcr.io/capi-test",
			ImageTag:        "v1.6.6_foobar.1",
		},
	}

	dnsBuildTag := before.DeepCopy()
	dnsBuildTag.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS = bootstrapv1.DNS{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageRepository: "gcr.io/capi-test",
			ImageTag:        "1.6.7",
		},
	}

	dnsInvalidTag := before.DeepCopy()
	dnsInvalidTag.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS = bootstrapv1.DNS{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageRepository: "gcr.io/capi-test",
			ImageTag:        "v0.20.0+invalidBuild1",
		},
	}

	dnsInvalidCoreDNSToVersion := dns.DeepCopy()
	dnsInvalidCoreDNSToVersion.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS = bootstrapv1.DNS{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageRepository: "gcr.io/capi-test",
			ImageTag:        "1.6.5",
		},
	}

	validCoreDNSCustomToVersion := dns.DeepCopy()
	validCoreDNSCustomToVersion.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS = bootstrapv1.DNS{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageRepository: "gcr.io/capi-test",
			ImageTag:        "v1.6.6_foobar.2",
		},
	}
	validUnsupportedCoreDNSVersion := dns.DeepCopy()
	validUnsupportedCoreDNSVersion.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS = bootstrapv1.DNS{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageRepository: "gcr.io/capi-test",
			ImageTag:        "v99.99.99",
		},
	}

	unsetCoreDNSToVersion := dns.DeepCopy()
	unsetCoreDNSToVersion.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS = bootstrapv1.DNS{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageRepository: "",
			ImageTag:        "",
		},
	}

	certificatesDir := before.DeepCopy()
	certificatesDir.Spec.KubeadmConfigSpec.ClusterConfiguration.CertificatesDir = "a new certificates directory"

	imageRepository := before.DeepCopy()
	imageRepository.Spec.KubeadmConfigSpec.ClusterConfiguration.ImageRepository = "a new image repository"

	featureGates := before.DeepCopy()
	featureGates.Spec.KubeadmConfigSpec.ClusterConfiguration.FeatureGates = map[string]bool{"a feature gate": true}

	externalEtcd := before.DeepCopy()
	externalEtcd.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.External = &bootstrapv1.ExternalEtcd{
		KeyFile: "some key file",
	}

	localDataDir := before.DeepCopy()
	localDataDir.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local = &bootstrapv1.LocalEtcd{
		DataDir: "some local data dir",
	}

	localPeerCertSANs := before.DeepCopy()
	localPeerCertSANs.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local = &bootstrapv1.LocalEtcd{
		PeerCertSANs: []string{"a cert"},
	}

	localServerCertSANs := before.DeepCopy()
	localServerCertSANs.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local = &bootstrapv1.LocalEtcd{
		ServerCertSANs: []string{"a cert"},
	}

	localExtraArgs := before.DeepCopy()
	localExtraArgs.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local = &bootstrapv1.LocalEtcd{
		ExtraArgs: map[string]string{"an arg": "a value"},
	}

	beforeExternalEtcdCluster := before.DeepCopy()
	beforeExternalEtcdCluster.Spec.KubeadmConfigSpec.ClusterConfiguration = &bootstrapv1.ClusterConfiguration{
		Etcd: bootstrapv1.Etcd{
			External: &bootstrapv1.ExternalEtcd{
				Endpoints: []string{"127.0.0.1"},
			},
		},
	}
	scaleToEvenExternalEtcdCluster := beforeExternalEtcdCluster.DeepCopy()
	scaleToEvenExternalEtcdCluster.Spec.Replicas = ptr.To[int32](2)

	beforeInvalidEtcdCluster := before.DeepCopy()
	beforeInvalidEtcdCluster.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd = bootstrapv1.Etcd{
		Local: &bootstrapv1.LocalEtcd{
			ImageMeta: bootstrapv1.ImageMeta{
				ImageRepository: "image-repository",
				ImageTag:        "latest",
			},
		},
	}

	afterInvalidEtcdCluster := beforeInvalidEtcdCluster.DeepCopy()
	afterInvalidEtcdCluster.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd = bootstrapv1.Etcd{
		External: &bootstrapv1.ExternalEtcd{
			Endpoints: []string{"127.0.0.1"},
		},
	}

	withoutClusterConfiguration := before.DeepCopy()
	withoutClusterConfiguration.Spec.KubeadmConfigSpec.ClusterConfiguration = nil

	afterEtcdLocalDirAddition := before.DeepCopy()
	afterEtcdLocalDirAddition.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local = &bootstrapv1.LocalEtcd{
		DataDir: "/data",
	}

	updateNTPServers := before.DeepCopy()
	updateNTPServers.Spec.KubeadmConfigSpec.NTP.Servers = []string{"new-server"}

	disableNTPServers := before.DeepCopy()
	disableNTPServers.Spec.KubeadmConfigSpec.NTP.Enabled = ptr.To(false)

	invalidRolloutBeforeCertificateExpiryDays := before.DeepCopy()
	invalidRolloutBeforeCertificateExpiryDays.Spec.RolloutBefore = &controlplanev1.RolloutBefore{
		CertificatesExpiryDays: ptr.To[int32](5), // less than minimum
	}

	unsetRolloutBefore := before.DeepCopy()
	unsetRolloutBefore.Spec.RolloutBefore = nil

	invalidIgnitionConfiguration := before.DeepCopy()
	invalidIgnitionConfiguration.Spec.KubeadmConfigSpec.Ignition = &bootstrapv1.IgnitionSpec{}

	validIgnitionConfigurationBefore := before.DeepCopy()
	validIgnitionConfigurationBefore.Spec.KubeadmConfigSpec.Format = bootstrapv1.Ignition
	validIgnitionConfigurationBefore.Spec.KubeadmConfigSpec.Ignition = &bootstrapv1.IgnitionSpec{
		ContainerLinuxConfig: &bootstrapv1.ContainerLinuxConfig{},
	}

	validIgnitionConfigurationAfter := validIgnitionConfigurationBefore.DeepCopy()
	validIgnitionConfigurationAfter.Spec.KubeadmConfigSpec.Ignition.ContainerLinuxConfig.AdditionalConfig = "foo: bar"

	updateInitConfigurationPatches := before.DeepCopy()
	updateInitConfigurationPatches.Spec.KubeadmConfigSpec.InitConfiguration.Patches = &bootstrapv1.Patches{
		Directory: "/tmp/patches",
	}

	updateJoinConfigurationPatches := before.DeepCopy()
	updateJoinConfigurationPatches.Spec.KubeadmConfigSpec.InitConfiguration.Patches = &bootstrapv1.Patches{
		Directory: "/tmp/patches",
	}

	updateInitConfigurationSkipPhases := before.DeepCopy()
	updateInitConfigurationSkipPhases.Spec.KubeadmConfigSpec.InitConfiguration.SkipPhases = []string{"addon/kube-proxy"}

	updateJoinConfigurationSkipPhases := before.DeepCopy()
	updateJoinConfigurationSkipPhases.Spec.KubeadmConfigSpec.JoinConfiguration.SkipPhases = []string{"addon/kube-proxy"}

	updateDiskSetup := before.DeepCopy()
	updateDiskSetup.Spec.KubeadmConfigSpec.DiskSetup = &bootstrapv1.DiskSetup{
		Filesystems: []bootstrapv1.Filesystem{
			{
				Device:     "/dev/sda",
				Filesystem: "ext4",
			},
		},
	}

	switchFromCloudInitToIgnition := before.DeepCopy()
	switchFromCloudInitToIgnition.Spec.KubeadmConfigSpec.Format = bootstrapv1.Ignition
	switchFromCloudInitToIgnition.Spec.KubeadmConfigSpec.Mounts = []bootstrapv1.MountPoints{
		{"/var/lib/testdir", "/var/lib/etcd/data"},
	}

	invalidMetadata := before.DeepCopy()
	invalidMetadata.Spec.MachineTemplate.ObjectMeta.Labels = map[string]string{
		"foo":          "$invalid-key",
		"bar":          strings.Repeat("a", 64) + "too-long-value",
		"/invalid-key": "foo",
	}
	invalidMetadata.Spec.MachineTemplate.ObjectMeta.Annotations = map[string]string{
		"/invalid-key": "foo",
	}

	beforeUseExperimentalRetryJoin := before.DeepCopy()
	beforeUseExperimentalRetryJoin.Spec.KubeadmConfigSpec.UseExperimentalRetryJoin = true //nolint:staticcheck
	updateUseExperimentalRetryJoin := before.DeepCopy()
	updateUseExperimentalRetryJoin.Spec.KubeadmConfigSpec.UseExperimentalRetryJoin = false //nolint:staticcheck

	tests := []struct {
		name                  string
		enableIgnitionFeature bool
		expectErr             bool
		before                *controlplanev1.KubeadmControlPlane
		kcp                   *controlplanev1.KubeadmControlPlane
	}{
		{
			name:      "should succeed when given a valid config",
			expectErr: false,
			before:    before,
			kcp:       validUpdate,
		},
		{
			name:      "should not return an error when trying to mutate the kubeadmconfigspec initconfiguration noderegistration",
			expectErr: false,
			before:    before,
			kcp:       validUpdateKubeadmConfigInit,
		},
		{
			name:      "should return error when trying to mutate the kubeadmconfigspec clusterconfiguration",
			expectErr: true,
			before:    before,
			kcp:       invalidUpdateKubeadmConfigCluster,
		},
		{
			name:      "should not return an error when trying to mutate the kubeadmconfigspec joinconfiguration noderegistration",
			expectErr: false,
			before:    before,
			kcp:       validUpdateKubeadmConfigJoin,
		},
		{
			name:      "should return error when trying to mutate the kubeadmconfigspec format from cloud-config to ignition",
			expectErr: true,
			before:    beforeKubeadmConfigFormatSet,
			kcp:       invalidUpdateKubeadmConfigFormat,
		},
		{
			name:      "should return error when trying to scale to zero",
			expectErr: true,
			before:    before,
			kcp:       scaleToZero,
		},
		{
			name:      "should return error when trying to scale to an even number",
			expectErr: true,
			before:    before,
			kcp:       scaleToEven,
		},
		{
			name:      "should return error when trying to reference cross namespace",
			expectErr: true,
			before:    before,
			kcp:       invalidNamespace,
		},
		{
			name:      "should return error when trying to scale to nil",
			expectErr: true,
			before:    before,
			kcp:       missingReplicas,
		},
		{
			name:      "should succeed when trying to scale to an even number with external etcd defined in ClusterConfiguration",
			expectErr: false,
			before:    beforeExternalEtcdCluster,
			kcp:       scaleToEvenExternalEtcdCluster,
		},
		{
			name:      "should succeed when making a change to the local etcd image tag",
			expectErr: false,
			before:    before,
			kcp:       etcdLocalImageTag,
		},
		{
			name:      "should succeed when making a change to the local etcd image tag",
			expectErr: false,
			before:    before,
			kcp:       etcdLocalImageBuildTag,
		},
		{
			name:      "should fail when using an invalid etcd image tag",
			expectErr: true,
			before:    before,
			kcp:       etcdLocalImageInvalidTag,
		},
		{
			name:      "should fail when making a change to the cluster config's networking struct",
			expectErr: true,
			before:    before,
			kcp:       networking,
		},
		{
			name:      "should fail when making a change to the cluster config's kubernetes version",
			expectErr: true,
			before:    before,
			kcp:       kubernetesVersion,
		},
		{
			name:      "should fail when making a change to the cluster config's controlPlaneEndpoint",
			expectErr: true,
			before:    before,
			kcp:       controlPlaneEndpoint,
		},
		{
			name:      "should allow changes to the cluster config's apiServer",
			expectErr: false,
			before:    before,
			kcp:       apiServer,
		},
		{
			name:      "should allow changes to the cluster config's controllerManager",
			expectErr: false,
			before:    before,
			kcp:       controllerManager,
		},
		{
			name:      "should allow changes to the cluster config's scheduler",
			expectErr: false,
			before:    before,
			kcp:       scheduler,
		},
		{
			name:      "should succeed when making a change to the cluster config's dns",
			expectErr: false,
			before:    before,
			kcp:       dns,
		},
		{
			name:      "should succeed when changing to a valid custom CoreDNS version",
			expectErr: false,
			before:    dns,
			kcp:       validCoreDNSCustomToVersion,
		},
		{
			name:      "should succeed when CoreDNS ImageTag is unset",
			expectErr: false,
			before:    dns,
			kcp:       unsetCoreDNSToVersion,
		},
		{
			name:      "should succeed when using an valid DNS build",
			expectErr: false,
			before:    before,
			kcp:       dnsBuildTag,
		},
		{
			name:   "should succeed when using the same CoreDNS version",
			before: dns,
			kcp:    dns.DeepCopy(),
		},
		{
			name:   "should succeed when using the same CoreDNS version - not supported",
			before: validUnsupportedCoreDNSVersion,
			kcp:    validUnsupportedCoreDNSVersion,
		},
		{
			name:      "should fail when using an invalid DNS build",
			expectErr: true,
			before:    before,
			kcp:       dnsInvalidTag,
		},
		{
			name:      "should fail when using an invalid CoreDNS version",
			expectErr: true,
			before:    dns,
			kcp:       dnsInvalidCoreDNSToVersion,
		},

		{
			name:      "should fail when making a change to the cluster config's certificatesDir",
			expectErr: true,
			before:    before,
			kcp:       certificatesDir,
		},
		{
			name:      "should fail when making a change to the cluster config's imageRepository",
			expectErr: false,
			before:    before,
			kcp:       imageRepository,
		},
		{
			name:      "should succeed when making a change to the cluster config's featureGates",
			expectErr: false,
			before:    before,
			kcp:       featureGates,
		},
		{
			name:      "should succeed when making a change to the cluster config's local etcd's configuration localDataDir field",
			expectErr: false,
			before:    before,
			kcp:       localDataDir,
		},
		{
			name:      "should succeed when making a change to the cluster config's local etcd's configuration localPeerCertSANs field",
			expectErr: false,
			before:    before,
			kcp:       localPeerCertSANs,
		},
		{
			name:      "should succeed when making a change to the cluster config's local etcd's configuration localServerCertSANs field",
			expectErr: false,
			before:    before,
			kcp:       localServerCertSANs,
		},
		{
			name:      "should succeed when making a change to the cluster config's local etcd's configuration localExtraArgs field",
			expectErr: false,
			before:    before,
			kcp:       localExtraArgs,
		},
		{
			name:      "should succeed when making a change to the cluster config's external etcd's configuration",
			expectErr: false,
			before:    before,
			kcp:       externalEtcd,
		},
		{
			name:      "should fail when attempting to unset the etcd local object",
			expectErr: true,
			before:    etcdLocalImageTag,
			kcp:       unsetEtcd,
		},
		{
			name:      "should fail if both local and external etcd are set",
			expectErr: true,
			before:    beforeInvalidEtcdCluster,
			kcp:       afterInvalidEtcdCluster,
		},
		{
			name:      "should pass if ClusterConfiguration is nil",
			expectErr: false,
			before:    withoutClusterConfiguration,
			kcp:       withoutClusterConfiguration,
		},
		{
			name:      "should fail if etcd local dir is changed from missing ClusterConfiguration",
			expectErr: true,
			before:    withoutClusterConfiguration,
			kcp:       afterEtcdLocalDirAddition,
		},
		{
			name:      "should not return an error when maxSurge value is updated to 0",
			expectErr: false,
			before:    before,
			kcp:       updateMaxSurgeVal,
		},
		{
			name:      "should return an error when maxSurge value is updated to 0, but replica count is < 3",
			expectErr: true,
			before:    before,
			kcp:       wrongReplicaCountForScaleIn,
		},
		{
			name:      "should pass if NTP servers are updated",
			expectErr: false,
			before:    before,
			kcp:       updateNTPServers,
		},
		{
			name:      "should pass if NTP servers is disabled during update",
			expectErr: false,
			before:    before,
			kcp:       disableNTPServers,
		},
		{
			name:      "should allow changes to initConfiguration.patches",
			expectErr: false,
			before:    before,
			kcp:       updateInitConfigurationPatches,
		},
		{
			name:      "should allow changes to joinConfiguration.patches",
			expectErr: false,
			before:    before,
			kcp:       updateJoinConfigurationPatches,
		},
		{
			name:      "should allow changes to initConfiguration.skipPhases",
			expectErr: false,
			before:    before,
			kcp:       updateInitConfigurationSkipPhases,
		},
		{
			name:      "should allow changes to joinConfiguration.skipPhases",
			expectErr: false,
			before:    before,
			kcp:       updateJoinConfigurationSkipPhases,
		},
		{
			name:      "should allow changes to diskSetup",
			expectErr: false,
			before:    before,
			kcp:       updateDiskSetup,
		},
		{
			name:      "should return error when rolloutBefore.certificatesExpiryDays is invalid",
			expectErr: true,
			before:    before,
			kcp:       invalidRolloutBeforeCertificateExpiryDays,
		},
		{
			name:      "should allow unsetting rolloutBefore",
			expectErr: false,
			before:    before,
			kcp:       unsetRolloutBefore,
		},
		{
			name:                  "should return error when Ignition configuration is invalid",
			enableIgnitionFeature: true,
			expectErr:             true,
			before:                invalidIgnitionConfiguration,
			kcp:                   invalidIgnitionConfiguration,
		},
		{
			name:                  "should succeed when Ignition configuration is modified",
			enableIgnitionFeature: true,
			expectErr:             false,
			before:                validIgnitionConfigurationBefore,
			kcp:                   validIgnitionConfigurationAfter,
		},
		{
			name:                  "should succeed when CloudInit was used before",
			enableIgnitionFeature: true,
			expectErr:             false,
			before:                before,
			kcp:                   switchFromCloudInitToIgnition,
		},
		{
			name:                  "should return error for invalid metadata",
			enableIgnitionFeature: true,
			expectErr:             true,
			before:                before,
			kcp:                   invalidMetadata,
		},
		{
			name:      "should allow changes to useExperimentalRetryJoin",
			expectErr: false,
			before:    beforeUseExperimentalRetryJoin,
			kcp:       updateUseExperimentalRetryJoin,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.enableIgnitionFeature {
				// NOTE: KubeadmBootstrapFormatIgnition feature flag is disabled by default.
				// Enabling the feature flag temporarily for this test.
				defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.KubeadmBootstrapFormatIgnition, true)()
			}

			g := NewWithT(t)

			webhook := &KubeadmControlPlane{}

			warnings, err := webhook.ValidateUpdate(ctx, tt.before.DeepCopy(), tt.kcp)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).To(Succeed())
			}
			g.Expect(warnings).To(BeEmpty())
		})
	}
}

func TestValidateVersion(t *testing.T) {
	tests := []struct {
		name                 string
		clusterConfiguration *bootstrapv1.ClusterConfiguration
		oldVersion           string
		newVersion           string
		expectErr            bool
	}{
		// Basic validation of old and new version.
		{
			name:       "error when old version is empty",
			oldVersion: "",
			newVersion: "v1.16.6",
			expectErr:  true,
		},
		{
			name:       "error when old version is invalid",
			oldVersion: "invalid-version",
			newVersion: "v1.18.1",
			expectErr:  true,
		},
		{
			name:       "error when new version is empty",
			oldVersion: "v1.16.6",
			newVersion: "",
			expectErr:  true,
		},
		{
			name:       "error when new version is invalid",
			oldVersion: "v1.18.1",
			newVersion: "invalid-version",
			expectErr:  true,
		},
		// Validation that we block upgrade to v1.19.0.
		// Note: Upgrading to v1.19.0 is not supported, because of issues in v1.19.0,
		// see: https://github.com/kubernetes-sigs/cluster-api/issues/3564
		{
			name:       "error when upgrading to v1.19.0",
			oldVersion: "v1.18.8",
			newVersion: "v1.19.0",
			expectErr:  true,
		},
		{
			name:       "pass when both versions are v1.19.0",
			oldVersion: "v1.19.0",
			newVersion: "v1.19.0",
			expectErr:  false,
		},
		// Validation for skip-level upgrades.
		{
			name:       "error when upgrading two minor versions",
			oldVersion: "v1.18.8",
			newVersion: "v1.20.0-alpha.0.734_ba502ee555924a",
			expectErr:  true,
		},
		{
			name:       "pass when upgrading one minor version",
			oldVersion: "v1.20.1",
			newVersion: "v1.21.18",
			expectErr:  false,
		},
		// Validation for usage of the old registry.
		// Notes:
		// * kubeadm versions < v1.22 are always using the old registry.
		// * kubeadm versions >= v1.25.0 are always using the new registry.
		// * kubeadm versions in between are using the new registry
		//   starting with certain patch versions.
		// This test validates that we don't block upgrades for < v1.22.0 and >= v1.25.0
		// and block upgrades to kubeadm versions in between with the old registry.
		{
			name: "pass when imageRepository is set",
			clusterConfiguration: &bootstrapv1.ClusterConfiguration{
				ImageRepository: "k8s.gcr.io",
			},
			oldVersion: "v1.21.1",
			newVersion: "v1.22.16",
			expectErr:  false,
		},
		{
			name:       "pass when version didn't change",
			oldVersion: "v1.22.16",
			newVersion: "v1.22.16",
			expectErr:  false,
		},
		{
			name:       "pass when new version is < v1.22.0",
			oldVersion: "v1.20.10",
			newVersion: "v1.21.5",
			expectErr:  false,
		},
		{
			name:       "error when new version is using old registry (v1.22.0 <= version <= v1.22.16)",
			oldVersion: "v1.21.1",
			newVersion: "v1.22.16", // last patch release using old registry
			expectErr:  true,
		},
		{
			name:       "pass when new version is using new registry (>= v1.22.17)",
			oldVersion: "v1.21.1",
			newVersion: "v1.22.17", // first patch release using new registry
			expectErr:  false,
		},
		{
			name:       "error when new version is using old registry (v1.23.0 <= version <= v1.23.14)",
			oldVersion: "v1.22.17",
			newVersion: "v1.23.14", // last patch release using old registry
			expectErr:  true,
		},
		{
			name:       "pass when new version is using new registry (>= v1.23.15)",
			oldVersion: "v1.22.17",
			newVersion: "v1.23.15", // first patch release using new registry
			expectErr:  false,
		},
		{
			name:       "error when new version is using old registry (v1.24.0 <= version <= v1.24.8)",
			oldVersion: "v1.23.1",
			newVersion: "v1.24.8", // last patch release using old registry
			expectErr:  true,
		},
		{
			name:       "pass when new version is using new registry (>= v1.24.9)",
			oldVersion: "v1.23.1",
			newVersion: "v1.24.9", // first patch release using new registry
			expectErr:  false,
		},
		{
			name:       "pass when new version is using new registry (>= v1.25.0)",
			oldVersion: "v1.24.8",
			newVersion: "v1.25.0", // uses new registry
			expectErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			kcpNew := controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: tt.clusterConfiguration,
					},
					Version: tt.newVersion,
				},
			}

			kcpOld := controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: tt.clusterConfiguration,
					},
					Version: tt.oldVersion,
				},
			}

			webhook := &KubeadmControlPlane{}

			allErrs := webhook.validateVersion(&kcpOld, &kcpNew)
			if tt.expectErr {
				g.Expect(allErrs).ToNot(BeEmpty())
			} else {
				g.Expect(allErrs).To(BeEmpty())
			}
		})
	}
}
func TestKubeadmControlPlaneValidateUpdateAfterDefaulting(t *testing.T) {
	g := NewWithT(t)

	before := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "foo",
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.19.0",
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "test/v1alpha1",
					Kind:       "UnknownInfraMachine",
					Namespace:  "foo",
					Name:       "infraTemplate",
				},
			},
		},
	}

	afterDefault := before.DeepCopy()
	webhook := &KubeadmControlPlane{}
	g.Expect(webhook.Default(ctx, afterDefault)).To(Succeed())

	tests := []struct {
		name      string
		expectErr bool
		before    *controlplanev1.KubeadmControlPlane
		kcp       *controlplanev1.KubeadmControlPlane
	}{
		{
			name:      "update should succeed after defaulting",
			expectErr: false,
			before:    before,
			kcp:       afterDefault,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			webhook := &KubeadmControlPlane{}

			warnings, err := webhook.ValidateUpdate(ctx, tt.before.DeepCopy(), tt.kcp)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).To(Succeed())
				g.Expect(tt.kcp.Spec.MachineTemplate.InfrastructureRef.Namespace).To(Equal(tt.before.Namespace))
				g.Expect(tt.kcp.Spec.Version).To(Equal("v1.19.0"))
				g.Expect(tt.kcp.Spec.RolloutStrategy.Type).To(Equal(controlplanev1.RollingUpdateStrategyType))
				g.Expect(tt.kcp.Spec.RolloutStrategy.RollingUpdate.MaxSurge.IntVal).To(Equal(int32(1)))
				g.Expect(tt.kcp.Spec.Replicas).To(Equal(ptr.To[int32](1)))
			}
			g.Expect(warnings).To(BeEmpty())
		})
	}
}

func TestPathsMatch(t *testing.T) {
	tests := []struct {
		name          string
		allowed, path []string
		match         bool
	}{
		{
			name:    "a simple match case",
			allowed: []string{"a", "b", "c"},
			path:    []string{"a", "b", "c"},
			match:   true,
		},
		{
			name:    "a case can't match",
			allowed: []string{"a", "b", "c"},
			path:    []string{"a"},
			match:   false,
		},
		{
			name:    "an empty path for whatever reason",
			allowed: []string{"a"},
			path:    []string{""},
			match:   false,
		},
		{
			name:    "empty allowed matches nothing",
			allowed: []string{},
			path:    []string{"a"},
			match:   false,
		},
		{
			name:    "wildcard match",
			allowed: []string{"a", "b", "c", "d", "*"},
			path:    []string{"a", "b", "c", "d", "e", "f", "g"},
			match:   true,
		},
		{
			name:    "long path",
			allowed: []string{"a"},
			path:    []string{"a", "b", "c", "d", "e", "f", "g"},
			match:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(pathsMatch(tt.allowed, tt.path)).To(Equal(tt.match))
		})
	}
}

func TestAllowed(t *testing.T) {
	tests := []struct {
		name      string
		allowList [][]string
		path      []string
		match     bool
	}{
		{
			name: "matches the first and none of the others",
			allowList: [][]string{
				{"a", "b", "c"},
				{"b", "d", "x"},
			},
			path:  []string{"a", "b", "c"},
			match: true,
		},
		{
			name: "matches none in the allow list",
			allowList: [][]string{
				{"a", "b", "c"},
				{"b", "c", "d"},
				{"e", "*"},
			},
			path:  []string{"a"},
			match: false,
		},
		{
			name: "an empty path matches nothing",
			allowList: [][]string{
				{"a", "b", "c"},
				{"*"},
				{"b", "c"},
			},
			path:  []string{},
			match: false,
		},
		{
			name:      "empty allowList matches nothing",
			allowList: [][]string{},
			path:      []string{"a"},
			match:     false,
		},
		{
			name: "length test check",
			allowList: [][]string{
				{"a", "b", "c", "d", "e", "f"},
				{"a", "b", "c", "d", "e", "f", "g", "h"},
			},
			path:  []string{"a", "b", "c", "d", "e", "f", "g"},
			match: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(allowed(tt.allowList, tt.path)).To(Equal(tt.match))
		})
	}
}

func TestPaths(t *testing.T) {
	tests := []struct {
		name     string
		path     []string
		diff     map[string]interface{}
		expected [][]string
	}{
		{
			name: "basic check",
			diff: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": 4,
					"version":  "1.17.3",
					"kubeadmConfigSpec": map[string]interface{}{
						"clusterConfiguration": map[string]interface{}{
							"version": "v2.0.1",
						},
						"initConfiguration": map[string]interface{}{
							"bootstrapToken": []string{"abcd", "defg"},
						},
						"joinConfiguration": nil,
					},
				},
			},
			expected: [][]string{
				{"spec", "replicas"},
				{"spec", "version"},
				{"spec", "kubeadmConfigSpec", "joinConfiguration"},
				{"spec", "kubeadmConfigSpec", "clusterConfiguration", "version"},
				{"spec", "kubeadmConfigSpec", "initConfiguration", "bootstrapToken"},
			},
		},
		{
			name:     "empty input makes for empty output",
			path:     []string{"a"},
			diff:     map[string]interface{}{},
			expected: [][]string{},
		},
		{
			name: "long recursive check with two keys",
			diff: map[string]interface{}{
				"spec": map[string]interface{}{
					"kubeadmConfigSpec": map[string]interface{}{
						"clusterConfiguration": map[string]interface{}{
							"version": "v2.0.1",
							"abc":     "d",
						},
					},
				},
			},
			expected: [][]string{
				{"spec", "kubeadmConfigSpec", "clusterConfiguration", "version"},
				{"spec", "kubeadmConfigSpec", "clusterConfiguration", "abc"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(paths(tt.path, tt.diff)).To(ConsistOf(tt.expected))
		})
	}
}
