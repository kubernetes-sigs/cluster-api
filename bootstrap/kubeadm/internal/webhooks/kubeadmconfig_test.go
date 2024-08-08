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
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/webhooks/util"
)

var ctx = ctrl.SetupSignalHandler()

func TestKubeadmConfigDefault(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)

	g := NewWithT(t)

	kubeadmConfig := &bootstrapv1.KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo",
		},
		Spec: bootstrapv1.KubeadmConfigSpec{},
	}
	updateDefaultingKubeadmConfig := kubeadmConfig.DeepCopy()
	updateDefaultingKubeadmConfig.Spec.Verbosity = ptr.To[int32](4)
	webhook := &KubeadmConfig{}
	t.Run("for KubeadmConfig", util.CustomDefaultValidateTest(ctx, updateDefaultingKubeadmConfig, webhook))

	g.Expect(webhook.Default(ctx, kubeadmConfig)).To(Succeed())

	g.Expect(kubeadmConfig.Spec.Format).To(Equal(bootstrapv1.CloudConfig))

	ignitionKubeadmConfig := &bootstrapv1.KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo",
		},
		Spec: bootstrapv1.KubeadmConfigSpec{
			Format: bootstrapv1.Ignition,
		},
	}
	g.Expect(webhook.Default(ctx, ignitionKubeadmConfig)).To(Succeed())
	g.Expect(ignitionKubeadmConfig.Spec.Format).To(Equal(bootstrapv1.Ignition))
}

func TestKubeadmConfigValidate(t *testing.T) {
	cases := map[string]struct {
		in                    *bootstrapv1.KubeadmConfig
		enableIgnitionFeature bool
		expectErr             bool
	}{
		"valid content": {
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Files: []bootstrapv1.File{
						{
							Content: "foo",
						},
					},
				},
			},
		},
		"valid contentFrom": {
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Files: []bootstrapv1.File{
						{
							ContentFrom: &bootstrapv1.FileSource{
								Secret: bootstrapv1.SecretFileSource{
									Name: "foo",
									Key:  "bar",
								},
							},
						},
					},
				},
			},
		},
		"invalid content and contentFrom": {
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Files: []bootstrapv1.File{
						{
							ContentFrom: &bootstrapv1.FileSource{},
							Content:     "foo",
						},
					},
				},
			},
			expectErr: true,
		},
		"invalid contentFrom without name": {
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Files: []bootstrapv1.File{
						{
							ContentFrom: &bootstrapv1.FileSource{
								Secret: bootstrapv1.SecretFileSource{
									Key: "bar",
								},
							},
							Content: "foo",
						},
					},
				},
			},
			expectErr: true,
		},
		"invalid contentFrom without key": {
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Files: []bootstrapv1.File{
						{
							ContentFrom: &bootstrapv1.FileSource{
								Secret: bootstrapv1.SecretFileSource{
									Name: "foo",
								},
							},
							Content: "foo",
						},
					},
				},
			},
			expectErr: true,
		},
		"invalid with duplicate file path": {
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Files: []bootstrapv1.File{
						{
							Content: "foo",
						},
						{
							Content: "bar",
						},
					},
				},
			},
			expectErr: true,
		},
		"valid passwd": {
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Users: []bootstrapv1.User{
						{
							Passwd: ptr.To("foo"),
						},
					},
				},
			},
		},
		"valid passwdFrom": {
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Users: []bootstrapv1.User{
						{
							PasswdFrom: &bootstrapv1.PasswdSource{
								Secret: bootstrapv1.SecretPasswdSource{
									Name: "foo",
									Key:  "bar",
								},
							},
						},
					},
				},
			},
		},
		"invalid passwd and passwdFrom": {
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Users: []bootstrapv1.User{
						{
							PasswdFrom: &bootstrapv1.PasswdSource{},
							Passwd:     ptr.To("foo"),
						},
					},
				},
			},
			expectErr: true,
		},
		"invalid passwdFrom without name": {
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Users: []bootstrapv1.User{
						{
							PasswdFrom: &bootstrapv1.PasswdSource{
								Secret: bootstrapv1.SecretPasswdSource{
									Key: "bar",
								},
							},
							Passwd: ptr.To("foo"),
						},
					},
				},
			},
			expectErr: true,
		},
		"invalid passwdFrom without key": {
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Users: []bootstrapv1.User{
						{
							PasswdFrom: &bootstrapv1.PasswdSource{
								Secret: bootstrapv1.SecretPasswdSource{
									Name: "foo",
								},
							},
							Passwd: ptr.To("foo"),
						},
					},
				},
			},
			expectErr: true,
		},
		"Ignition field is set, format is not Ignition": {
			enableIgnitionFeature: true,
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Ignition: &bootstrapv1.IgnitionSpec{},
				},
			},
			expectErr: true,
		},
		"Ignition field is not set, format is Ignition": {
			enableIgnitionFeature: true,
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Format: bootstrapv1.Ignition,
				},
			},
		},
		"format is Ignition, user is inactive": {
			enableIgnitionFeature: true,
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Format: bootstrapv1.Ignition,
					Users: []bootstrapv1.User{
						{
							Inactive: ptr.To(true),
						},
					},
				},
			},
			expectErr: true,
		},
		"format is Ignition, non-GPT partition configured": {
			enableIgnitionFeature: true,
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Format: bootstrapv1.Ignition,
					DiskSetup: &bootstrapv1.DiskSetup{
						Partitions: []bootstrapv1.Partition{
							{
								TableType: ptr.To("MS-DOS"),
							},
						},
					},
				},
			},
			expectErr: true,
		},
		"format is Ignition, experimental retry join is set": {
			enableIgnitionFeature: true,
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Format:                   bootstrapv1.Ignition,
					UseExperimentalRetryJoin: true,
				},
			},
			expectErr: true,
		},
		"feature gate disabled, format is Ignition": {
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Format: bootstrapv1.Ignition,
				},
			},
			expectErr: true,
		},
		"feature gate disabled, Ignition field is set": {
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Format: bootstrapv1.Ignition,
					Ignition: &bootstrapv1.IgnitionSpec{
						ContainerLinuxConfig: &bootstrapv1.ContainerLinuxConfig{},
					},
				},
			},
			expectErr: true,
		},
		"replaceFS specified with Ignition": {
			enableIgnitionFeature: true,
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Format: bootstrapv1.Ignition,
					DiskSetup: &bootstrapv1.DiskSetup{
						Filesystems: []bootstrapv1.Filesystem{
							{
								ReplaceFS: ptr.To("ntfs"),
							},
						},
					},
				},
			},
			expectErr: true,
		},
		"filesystem partition specified with Ignition": {
			enableIgnitionFeature: true,
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Format: bootstrapv1.Ignition,
					DiskSetup: &bootstrapv1.DiskSetup{
						Filesystems: []bootstrapv1.Filesystem{
							{
								Partition: ptr.To("1"),
							},
						},
					},
				},
			},
			expectErr: true,
		},
		"file encoding gzip specified with Ignition": {
			enableIgnitionFeature: true,
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Format: bootstrapv1.Ignition,
					Files: []bootstrapv1.File{
						{
							Encoding: bootstrapv1.Gzip,
						},
					},
				},
			},
			expectErr: true,
		},
		"file encoding gzip+base64 specified with Ignition": {
			enableIgnitionFeature: true,
			in: &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: bootstrapv1.KubeadmConfigSpec{
					Format: bootstrapv1.Ignition,
					Files: []bootstrapv1.File{
						{
							Encoding: bootstrapv1.GzipBase64,
						},
					},
				},
			},
			expectErr: true,
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			if tt.enableIgnitionFeature {
				// NOTE: KubeadmBootstrapFormatIgnition feature flag is disabled by default.
				// Enabling the feature flag temporarily for this test.
				utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.KubeadmBootstrapFormatIgnition, true)
			}
			g := NewWithT(t)

			webhook := &KubeadmConfig{}

			if tt.expectErr {
				warnings, err := webhook.ValidateCreate(ctx, tt.in)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, nil, tt.in)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			} else {
				warnings, err := webhook.ValidateCreate(ctx, tt.in)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, nil, tt.in)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			}
		})
	}
}
