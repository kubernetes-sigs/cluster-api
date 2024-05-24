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

	"github.com/blang/semver/v4"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"
)

func TestUpdateCoreDNS(t *testing.T) {
	validKCP := &controlplanev1.KubeadmControlPlane{
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
					DNS: bootstrapv1.DNS{
						ImageMeta: bootstrapv1.ImageMeta{
							ImageRepository: "",
							ImageTag:        "",
						},
					},
					ImageRepository: "",
				},
			},
		},
	}
	// This is used to force an error to be returned so we can assert the
	// following pre-checks that need to happen before we retrieve the
	// CoreDNSInfo.
	badCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      coreDNSKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			"BadCoreFileKey": "",
		},
	}

	depl := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      coreDNSKey,
			Namespace: metav1.NamespaceSystem,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:   coreDNSKey,
					Labels: map[string]string{"app": coreDNSKey},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  coreDNSKey,
						Image: "k8s.gcr.io/some-folder/coredns:1.6.2",
					}},
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": coreDNSKey},
			},
		},
	}

	deplWithImage := func(image string) *appsv1.Deployment {
		d := depl.DeepCopy()
		d.Spec.Template.Spec.Containers[0].Image = image
		return d
	}

	expectedCorefile := "coredns-core-file"
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      coreDNSKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			"Corefile": expectedCorefile,
		},
	}
	updatedCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      coreDNSKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			"Corefile":        "updated-core-file",
			"Corefile-backup": expectedCorefile,
		},
	}
	kubeadmCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeadmConfigKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			"ClusterConfiguration": utilyaml.Raw(`
				apiServer:
				apiVersion: kubeadm.k8s.io/v1beta2
				dns:
				  type: CoreDNS
				imageRepository: k8s.gcr.io
				kind: ClusterConfiguration
				`),
		},
	}
	kubeadmCM181 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeadmConfigKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			"ClusterConfiguration": utilyaml.Raw(`
				apiServer:
				apiVersion: kubeadm.k8s.io/v1beta2
				dns:
				  type: CoreDNS
					imageTag: v1.8.1
				imageRepository: k8s.gcr.io
				kind: ClusterConfiguration
				`),
		},
	}

	oldCR := &rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRole",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: coreDNSClusterRoleName,
		},
	}

	semver1191 := semver.MustParse("1.19.1")
	semver1221 := semver.MustParse("1.22.1")
	semver1230 := semver.MustParse("1.23.0")

	tests := []struct {
		name          string
		kcp           *controlplanev1.KubeadmControlPlane
		migrator      coreDNSMigrator
		semver        semver.Version
		objs          []client.Object
		expectErr     bool
		expectUpdates bool
		expectImage   string
		expectRules   []rbacv1.PolicyRule
	}{
		{
			name: "returns early without error if skip core dns annotation is present",
			kcp: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						controlplanev1.SkipCoreDNSAnnotation: "",
					},
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							DNS: bootstrapv1.DNS{},
						},
					},
				},
			},
			semver:    semver1191,
			objs:      []client.Object{badCM},
			expectErr: false,
		},
		{
			name: "returns early without error if KCP ClusterConfiguration is nil",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{},
				},
			},
			semver:    semver1191,
			objs:      []client.Object{badCM},
			expectErr: false,
		},
		{
			name:      "returns early without error if CoreDNS info is not found",
			kcp:       validKCP,
			semver:    semver1191,
			expectErr: false,
		},
		{
			name:      "returns error if there was a problem retrieving CoreDNS info",
			kcp:       validKCP,
			semver:    semver1191,
			objs:      []client.Object{badCM},
			expectErr: true,
		},
		{
			name:      "returns early without error if CoreDNS fromImage == ToImage",
			kcp:       validKCP,
			semver:    semver1191,
			objs:      []client.Object{depl, cm},
			expectErr: false,
		},
		{
			name: "returns error if validation of CoreDNS image tag fails",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							DNS: bootstrapv1.DNS{
								ImageMeta: bootstrapv1.ImageMeta{
									// image is older than what's already
									// installed.
									ImageRepository: "k8s.gcr.io/some-folder/coredns",
									ImageTag:        "1.1.2",
								},
							},
						},
					},
				},
			},
			semver:    semver1191,
			objs:      []client.Object{depl, cm},
			expectErr: true,
		},
		{
			name: "returns error if unable to update CoreDNS image info in kubeadm config map",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							DNS: bootstrapv1.DNS{
								ImageMeta: bootstrapv1.ImageMeta{
									// provide an newer image to update to
									ImageRepository: "k8s.gcr.io/some-folder/coredns",
									ImageTag:        "1.7.2",
								},
							},
						},
					},
				},
			},
			semver: semver1191,
			// no kubeadmConfigMap available so it will trigger an error
			objs:      []client.Object{depl, cm},
			expectErr: true,
		},
		{
			name: "returns error if unable to update CoreDNS corefile",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							DNS: bootstrapv1.DNS{
								ImageMeta: bootstrapv1.ImageMeta{
									// provide an newer image to update to
									ImageRepository: "k8s.gcr.io/some-folder/coredns",
									ImageTag:        "1.7.2",
								},
							},
						},
					},
				},
			},
			migrator: &fakeMigrator{
				migrateErr: errors.New("failed to migrate"),
			},
			semver:    semver1191,
			objs:      []client.Object{depl, cm, kubeadmCM},
			expectErr: true,
		},
		{
			name: "updates everything successfully",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							DNS: bootstrapv1.DNS{
								ImageMeta: bootstrapv1.ImageMeta{
									// provide an newer image to update to
									ImageRepository: "k8s.gcr.io/some-repo",
									ImageTag:        "1.7.2",
								},
							},
						},
					},
				},
			},
			migrator: &fakeMigrator{
				migratedCorefile: "updated-core-file",
			},
			semver:        semver1191,
			objs:          []client.Object{depl, cm, kubeadmCM},
			expectErr:     false,
			expectUpdates: true,
			expectImage:   "k8s.gcr.io/some-repo/coredns:1.7.2",
		},
		{
			name: "updates everything successfully to v1.8.0 with a custom repo should not change the image name",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							DNS: bootstrapv1.DNS{
								ImageMeta: bootstrapv1.ImageMeta{
									// provide an newer image to update to
									ImageRepository: "k8s.gcr.io/some-repo",
									ImageTag:        "1.8.0",
								},
							},
						},
					},
				},
			},
			migrator: &fakeMigrator{
				migratedCorefile: "updated-core-file",
			},
			semver:        semver1191,
			objs:          []client.Object{deplWithImage("k8s.gcr.io/some-repo/coredns:1.7.0"), cm, kubeadmCM},
			expectErr:     false,
			expectUpdates: true,
			expectImage:   "k8s.gcr.io/some-repo/coredns:1.8.0",
		},
		{
			name: "kubeadm defaults, upgrade from Kubernetes v1.18.x to v1.19.y (from k8s.gcr.io/coredns:1.6.7 to k8s.gcr.io/coredns:1.7.0)",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							DNS: bootstrapv1.DNS{
								ImageMeta: bootstrapv1.ImageMeta{
									ImageRepository: "k8s.gcr.io",
									ImageTag:        "1.7.0",
								},
							},
						},
					},
				},
			},
			migrator: &fakeMigrator{
				migratedCorefile: "updated-core-file",
			},
			semver:        semver1191,
			objs:          []client.Object{deplWithImage("k8s.gcr.io/coredns:1.6.7"), cm, kubeadmCM},
			expectErr:     false,
			expectUpdates: true,
			expectImage:   "k8s.gcr.io/coredns:1.7.0",
		},
		{
			name: "kubeadm defaults, upgrade from Kubernetes v1.19.x to v1.20.y (stay on k8s.gcr.io/coredns:1.7.0)",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							DNS: bootstrapv1.DNS{
								ImageMeta: bootstrapv1.ImageMeta{
									ImageRepository: "k8s.gcr.io",
									ImageTag:        "1.7.0",
								},
							},
						},
					},
				},
			},
			migrator: &fakeMigrator{
				migratedCorefile: "updated-core-file",
			},
			semver:        semver1191,
			objs:          []client.Object{deplWithImage("k8s.gcr.io/coredns:1.7.0"), cm, kubeadmCM},
			expectErr:     false,
			expectUpdates: false,
		},
		{
			name: "kubeadm defaults, upgrade from Kubernetes v1.20.x to v1.21.y (from k8s.gcr.io/coredns:1.7.0 to k8s.gcr.io/coredns/coredns:v1.8.0)",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							DNS: bootstrapv1.DNS{
								ImageMeta: bootstrapv1.ImageMeta{
									ImageRepository: "k8s.gcr.io",
									ImageTag:        "v1.8.0", // NOTE: ImageTags requires the v prefix
								},
							},
						},
					},
				},
			},
			migrator: &fakeMigrator{
				migratedCorefile: "updated-core-file",
			},
			semver:        semver1191,
			objs:          []client.Object{deplWithImage("k8s.gcr.io/coredns:1.7.0"), cm, kubeadmCM},
			expectErr:     false,
			expectUpdates: true,
			expectImage:   "k8s.gcr.io/coredns/coredns:v1.8.0", // NOTE: ImageName has coredns/coredns
		},
		{
			name: "kubeadm defaults, upgrade from Kubernetes v1.21.x to v1.22.y (stay on k8s.gcr.io/coredns/coredns:v1.8.0)",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							DNS: bootstrapv1.DNS{
								ImageMeta: bootstrapv1.ImageMeta{
									ImageRepository: "k8s.gcr.io",
									ImageTag:        "v1.8.0", // NOTE: ImageTags requires the v prefix
								},
							},
						},
					},
				},
			},
			semver: semver1191,
			migrator: &fakeMigrator{
				migratedCorefile: "updated-core-file",
			},
			objs:          []client.Object{deplWithImage("k8s.gcr.io/coredns/coredns:v1.8.0"), cm, kubeadmCM},
			expectErr:     false,
			expectUpdates: false,
			expectRules:   oldCR.Rules,
		},
		{
			name: "upgrade from Kubernetes v1.21.x to v1.22.y and update cluster role",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							DNS: bootstrapv1.DNS{
								ImageMeta: bootstrapv1.ImageMeta{
									ImageRepository: "k8s.gcr.io",
									ImageTag:        "v1.8.1", // NOTE: ImageTags requires the v prefix
								},
							},
						},
					},
				},
			},
			migrator: &fakeMigrator{
				migratedCorefile: "updated-core-file",
			},
			semver:        semver1221,
			objs:          []client.Object{deplWithImage("k8s.gcr.io/coredns/coredns:v1.8.1"), updatedCM, kubeadmCM181, oldCR},
			expectErr:     false,
			expectUpdates: true,
			expectImage:   "k8s.gcr.io/coredns/coredns:v1.8.1", // NOTE: ImageName has coredns/coredns
			expectRules:   coreDNS181PolicyRules,
		},
		{
			name: "returns early without error if kubernetes version is >= v1.23",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							DNS: bootstrapv1.DNS{
								ImageMeta: bootstrapv1.ImageMeta{
									ImageRepository: "k8s.gcr.io",
									ImageTag:        "v1.8.1", // NOTE: ImageTags requires the v prefix
								},
							},
						},
					},
				},
			},
			semver:      semver1230,
			objs:        []client.Object{deplWithImage("k8s.gcr.io/coredns/coredns:v1.8.1"), updatedCM, kubeadmCM181},
			expectErr:   false,
			expectRules: oldCR.Rules,
		},
	}

	// We are using testEnv as a workload cluster, and given that each test case assumes well known objects with specific
	// Namespace/Name (e.g. The CoderDNS ConfigMap & Deployment, the kubeadm ConfigMap), it is not possible to run the use cases in parallel.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			for _, o := range tt.objs {
				// NB. deep copy test object so changes applied during a test does not affect other tests.
				o := o.DeepCopyObject().(client.Object)
				g.Expect(env.CreateAndWait(ctx, o)).To(Succeed())
			}

			// Register cleanup function
			t.Cleanup(func() {
				_ = env.CleanupAndWait(ctx, tt.objs...)
			})

			w := &Workload{
				Client:          env.GetClient(),
				CoreDNSMigrator: tt.migrator,
			}
			err := w.UpdateCoreDNS(ctx, tt.kcp, tt.semver)

			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			// Assert that CoreDNS updates have been made
			if tt.expectUpdates {
				// assert kubeadmConfigMap
				g.Eventually(func(g Gomega) error {
					var expectedKubeadmConfigMap corev1.ConfigMap
					g.Expect(env.Get(ctx, client.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem}, &expectedKubeadmConfigMap)).To(Succeed())
					g.Expect(expectedKubeadmConfigMap.Data).To(HaveKeyWithValue("ClusterConfiguration", ContainSubstring(tt.kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS.ImageTag)))
					g.Expect(expectedKubeadmConfigMap.Data).To(HaveKeyWithValue("ClusterConfiguration", ContainSubstring(tt.kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS.ImageRepository)))
					return nil
				}, "5s").Should(Succeed())

				// assert CoreDNS corefile
				var expectedConfigMap corev1.ConfigMap
				g.Eventually(func() error {
					if err := env.Get(ctx, client.ObjectKey{Name: coreDNSKey, Namespace: metav1.NamespaceSystem}, &expectedConfigMap); err != nil {
						return errors.Wrap(err, "failed to get the coredns ConfigMap")
					}
					if len(expectedConfigMap.Data) != 2 {
						return errors.Errorf("the coredns ConfigMap has %d data items, expected 2", len(expectedConfigMap.Data))
					}
					if val, ok := expectedConfigMap.Data["Corefile"]; !ok || val != "updated-core-file" {
						return errors.New("the coredns ConfigMap does not have the Corefile entry or this it has an unexpected value")
					}
					if val, ok := expectedConfigMap.Data["Corefile-backup"]; !ok || val != expectedCorefile {
						return errors.New("the coredns ConfigMap does not have the Corefile-backup entry or this it has an unexpected value")
					}
					return nil
				}, "5s").Should(BeNil())

				// assert CoreDNS deployment
				var actualDeployment appsv1.Deployment
				g.Eventually(func() string {
					g.Expect(env.Get(ctx, client.ObjectKey{Name: coreDNSKey, Namespace: metav1.NamespaceSystem}, &actualDeployment)).To(Succeed())
					return actualDeployment.Spec.Template.Spec.Containers[0].Image
				}, "5s").Should(Equal(tt.expectImage))

				// assert CoreDNS ClusterRole
				if tt.expectRules != nil {
					var actualClusterRole rbacv1.ClusterRole
					g.Eventually(func() []rbacv1.PolicyRule {
						g.Expect(env.Get(ctx, client.ObjectKey{Name: coreDNSClusterRoleName}, &actualClusterRole)).To(Succeed())
						return actualClusterRole.Rules
					}, "5s").Should(BeComparableTo(tt.expectRules))
				}
			}
		})
	}
}

func TestValidateCoreDNSImageTag(t *testing.T) {
	tests := []struct {
		name            string
		fromVer         string
		toVer           string
		expectErrSubStr string
	}{
		{
			name:            "fromVer is higher than toVer",
			fromVer:         "1.6.2",
			toVer:           "1.1.3",
			expectErrSubStr: "must be greater than",
		},
		{
			name:            "fromVer is not a valid coredns version",
			fromVer:         "0.204.123",
			toVer:           "1.6.3",
			expectErrSubStr: "not a compatible coredns version",
		},
		{
			name:            "toVer is not a valid semver",
			fromVer:         "1.5.1",
			toVer:           "foobar",
			expectErrSubStr: "failed to parse CoreDNS target version",
		},
		{
			name:            "fromVer is not a valid semver",
			fromVer:         "foobar",
			toVer:           "1.6.1",
			expectErrSubStr: "failed to parse CoreDNS current version",
		},
		{
			name:    "fromVer is equal to toVer, but different patch versions",
			fromVer: "1.6.5_foobar.1",
			toVer:   "1.6.5_foobar.2",
		},
		{
			name:    "fromVer is equal to toVer",
			fromVer: "1.6.5_foobar.1",
			toVer:   "1.6.5_foobar.1",
		},
		{
			name:    "fromVer is lower but has meta",
			fromVer: "1.6.5-foobar.1",
			toVer:   "1.7.5",
		},
		{
			name:    "fromVer is lower and has meta and leading v",
			fromVer: "v1.6.5-foobar.1",
			toVer:   "1.7.5",
		},
		{
			name:    "fromVer is lower, toVer has meta and leading v",
			fromVer: "1.6.5-foobar.1",
			toVer:   "v1.7.5_foobar.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			err := validateCoreDNSImageTag(tt.fromVer, tt.toVer)
			if tt.expectErrSubStr != "" {
				g.Expect(err.Error()).To(ContainSubstring(tt.expectErrSubStr))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestUpdateCoreDNSClusterRole(t *testing.T) {
	coreDNS180PolicyRules := []rbacv1.PolicyRule{
		{
			Verbs:     []string{"list", "watch"},
			APIGroups: []string{""},
			Resources: []string{"endpoints", "services", "pods", "namespaces"},
		},
		{
			Verbs:     []string{"get"},
			APIGroups: []string{""},
			Resources: []string{"nodes"},
		},
	}

	tests := []struct {
		name                     string
		kubernetesVersion        semver.Version
		coreDNSVersion           string
		sourceCoreDNSVersion     string
		coreDNSPolicyRules       []rbacv1.PolicyRule
		expectErr                bool
		expectCoreDNSPolicyRules []rbacv1.PolicyRule
	}{
		{
			name:               "does not patch ClusterRole: invalid CoreDNS tag",
			kubernetesVersion:  semver.Version{Major: 1, Minor: 22, Patch: 0},
			coreDNSVersion:     "no-semver",
			coreDNSPolicyRules: coreDNS180PolicyRules,
			expectErr:          true,
		},
		{
			name:                     "does not patch ClusterRole: Kubernetes < 1.22",
			kubernetesVersion:        semver.Version{Major: 1, Minor: 21, Patch: 0},
			coreDNSVersion:           "1.8.4",
			coreDNSPolicyRules:       coreDNS180PolicyRules,
			expectCoreDNSPolicyRules: coreDNS180PolicyRules,
		},
		{
			name:                     "does not patch ClusterRole: Kubernetes > 1.22 && CoreDNS >= 1.8.1",
			kubernetesVersion:        semver.Version{Major: 1, Minor: 23, Patch: 0},
			coreDNSVersion:           "1.8.1",
			sourceCoreDNSVersion:     "1.8.1",
			coreDNSPolicyRules:       coreDNS180PolicyRules,
			expectCoreDNSPolicyRules: coreDNS180PolicyRules,
		},
		{
			name:                     "does not patch ClusterRole: CoreDNS < 1.8.1",
			kubernetesVersion:        semver.Version{Major: 1, Minor: 22, Patch: 0},
			coreDNSVersion:           "1.8.0",
			sourceCoreDNSVersion:     "1.7.0",
			coreDNSPolicyRules:       coreDNS180PolicyRules,
			expectCoreDNSPolicyRules: coreDNS180PolicyRules,
		},
		{
			name:                     "patch ClusterRole: Kubernetes == 1.22 alpha and CoreDNS == 1.8.1",
			kubernetesVersion:        semver.Version{Major: 1, Minor: 22, Patch: 0, Pre: []semver.PRVersion{{VersionStr: "alpha"}}},
			coreDNSVersion:           "1.8.1",
			sourceCoreDNSVersion:     "1.8.1",
			coreDNSPolicyRules:       coreDNS180PolicyRules,
			expectCoreDNSPolicyRules: coreDNS181PolicyRules,
		},
		{
			name:                     "patch ClusterRole: Kubernetes == 1.22 and CoreDNS == 1.8.1",
			kubernetesVersion:        semver.Version{Major: 1, Minor: 22, Patch: 0},
			coreDNSVersion:           "1.8.1",
			sourceCoreDNSVersion:     "1.8.1",
			coreDNSPolicyRules:       coreDNS180PolicyRules,
			expectCoreDNSPolicyRules: coreDNS181PolicyRules,
		},
		{
			name:                     "patch ClusterRole: Kubernetes > 1.22 and CoreDNS > 1.8.1",
			kubernetesVersion:        semver.Version{Major: 1, Minor: 22, Patch: 2},
			coreDNSVersion:           "1.8.5",
			sourceCoreDNSVersion:     "1.8.1",
			coreDNSPolicyRules:       coreDNS180PolicyRules,
			expectCoreDNSPolicyRules: coreDNS181PolicyRules,
		},
		{
			name:                     "patch ClusterRole: Kubernetes > 1.22 and CoreDNS > 1.8.1: no-op",
			kubernetesVersion:        semver.Version{Major: 1, Minor: 22, Patch: 2},
			coreDNSVersion:           "1.8.5",
			sourceCoreDNSVersion:     "1.8.5",
			coreDNSPolicyRules:       coreDNS181PolicyRules,
			expectCoreDNSPolicyRules: coreDNS181PolicyRules,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			cr := &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      coreDNSClusterRoleName,
					Namespace: metav1.NamespaceSystem,
				},
				Rules: tt.coreDNSPolicyRules,
			}
			fakeClient := fake.NewClientBuilder().WithObjects(cr).Build()

			w := &Workload{
				Client: fakeClient,
			}

			err := w.updateCoreDNSClusterRole(ctx, tt.kubernetesVersion, &coreDNSInfo{ToImageTag: tt.coreDNSVersion, FromImageTag: tt.sourceCoreDNSVersion})

			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			var actualClusterRole rbacv1.ClusterRole
			g.Expect(fakeClient.Get(ctx, client.ObjectKey{Name: coreDNSClusterRoleName, Namespace: metav1.NamespaceSystem}, &actualClusterRole)).To(Succeed())

			g.Expect(actualClusterRole.Rules).To(BeComparableTo(tt.expectCoreDNSPolicyRules))
		})
	}
}

func TestSemanticallyDeepEqualPolicyRules(t *testing.T) {
	tests := []struct {
		name string
		r1   []rbacv1.PolicyRule
		r2   []rbacv1.PolicyRule
		want bool
	}{
		{
			name: "equal: identical arrays",
			r1: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"list", "watch"},
					APIGroups: []string{""},
					Resources: []string{"endpoints", "services", "pods", "namespaces"},
				},
				{
					Verbs:     []string{"list", "watch"},
					APIGroups: []string{"discovery.k8s.io"},
					Resources: []string{"endpointslices"},
				},
			},
			r2: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"list", "watch"},
					APIGroups: []string{""},
					Resources: []string{"endpoints", "services", "pods", "namespaces"},
				},
				{
					Verbs:     []string{"list", "watch"},
					APIGroups: []string{"discovery.k8s.io"},
					Resources: []string{"endpointslices"},
				},
			},
			want: true,
		},
		{
			name: "equal: arrays with different order",
			r1: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"list", "watch"},
					APIGroups: []string{""},
					Resources: []string{"endpoints", "services", "pods", "namespaces"},
				},
				{
					Verbs:     []string{"list", "watch"},
					APIGroups: []string{"discovery.k8s.io"},
					Resources: []string{"endpointslices"},
				},
			},
			r2: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"watch", "list"},
					APIGroups: []string{"discovery.k8s.io"},
					Resources: []string{"endpointslices"},
				},
				{
					Verbs:     []string{"list", "watch"},
					APIGroups: []string{""},
					Resources: []string{"endpoints", "pods", "services", "namespaces"},
				},
			},
			want: true,
		},
		{
			name: "equal: separate rules but same semantic",
			r1: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"list", "watch"},
					APIGroups: []string{""},
					Resources: []string{"endpoints", "services", "pods", "namespaces"},
				},
				{
					Verbs:     []string{"list", "watch"},
					APIGroups: []string{"discovery.k8s.io"},
					Resources: []string{"endpointslices"},
				},
			},
			r2: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"watch", "list"},
					APIGroups: []string{"discovery.k8s.io"},
					Resources: []string{"endpointslices"},
				},
				{
					Verbs:     []string{"list", "watch"},
					APIGroups: []string{""},
					Resources: []string{"endpoints", "pods"},
				},
				{
					Verbs:     []string{"list", "watch"},
					APIGroups: []string{""},
					Resources: []string{"services"},
				},
				{
					Verbs:     []string{"list", "watch"},
					APIGroups: []string{""},
					Resources: []string{"namespaces"},
				},
			},
			want: true,
		},
		{
			name: "not equal: one array has additional rules",
			r1: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"list", "watch"},
					APIGroups: []string{""},
					Resources: []string{"endpoints", "services", "pods", "namespaces"},
				},
				{
					Verbs:     []string{"list", "watch"},
					APIGroups: []string{"discovery.k8s.io"},
					Resources: []string{"endpointslices"},
				},
			},
			r2: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"list", "watch"},
					APIGroups: []string{""},
					Resources: []string{"endpoints", "services", "pods", "namespaces"},
				},
				{
					Verbs:     []string{"list", "watch"},
					APIGroups: []string{"discovery.k8s.io"},
					Resources: []string{"endpointslices"},
				},
				{
					Verbs:     []string{"get"},
					APIGroups: []string{""},
					Resources: []string{"nodes"},
				},
			},
			want: false,
		},
		{
			name: "not equal: one array has additional verbs",
			r1: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"list", "watch"},
					APIGroups: []string{""},
					Resources: []string{"endpoints", "services", "pods", "namespaces"},
				},
				{
					Verbs:     []string{"list", "watch"},
					APIGroups: []string{"discovery.k8s.io"},
					Resources: []string{"endpointslices"},
				},
			},
			r2: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"list", "watch"},
					APIGroups: []string{""},
					Resources: []string{"endpoints", "services", "pods", "namespaces"},
				},
				{
					Verbs:     []string{"list", "watch", "get", "update"},
					APIGroups: []string{"discovery.k8s.io"},
					Resources: []string{"endpointslices"},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := semanticDeepEqualPolicyRules(tt.r1, tt.r2); got != tt.want {
				t.Errorf("semanticDeepEqualPolicyRules() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUpdateCoreDNSCorefile(t *testing.T) {
	currentImageTag := "1.6.2"
	originalCorefile := "some-coredns-core-file"
	depl := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      coreDNSKey,
			Namespace: metav1.NamespaceSystem,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: coreDNSKey,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  coreDNSKey,
						Image: "k8s.gcr.io/coredns:" + currentImageTag,
					}},
					Volumes: []corev1.Volume{{
						Name: "config-volume",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: coreDNSKey,
								},
								Items: []corev1.KeyToPath{{
									Key:  "Corefile",
									Path: "Corefile",
								}},
							},
						},
					}},
				},
			},
		},
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      coreDNSKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			"Corefile": originalCorefile,
		},
	}

	t.Run("returns error if migrate failed to update corefile", func(t *testing.T) {
		g := NewWithT(t)
		objs := []client.Object{depl, cm}
		fakeClient := fake.NewClientBuilder().WithObjects(objs...).Build()
		fakeMigrator := &fakeMigrator{
			migrateErr: errors.New("failed to migrate"),
		}

		w := &Workload{
			Client:          fakeClient,
			CoreDNSMigrator: fakeMigrator,
		}

		info := &coreDNSInfo{
			Corefile:               "updated-core-file",
			Deployment:             depl,
			CurrentMajorMinorPatch: "1.6.2",
			TargetMajorMinorPatch:  "1.7.2",
		}

		err := w.updateCoreDNSCorefile(ctx, info)
		g.Expect(err).To(HaveOccurred())
		g.Expect(fakeMigrator.migrateCalled).To(BeTrue())

		var expectedConfigMap corev1.ConfigMap
		g.Expect(fakeClient.Get(ctx, client.ObjectKey{Name: coreDNSKey, Namespace: metav1.NamespaceSystem}, &expectedConfigMap)).To(Succeed())
		g.Expect(expectedConfigMap.Data).To(HaveLen(1))
		g.Expect(expectedConfigMap.Data).To(HaveKeyWithValue("Corefile", originalCorefile))
	})

	t.Run("creates a backup of the corefile", func(t *testing.T) {
		g := NewWithT(t)
		// Not including the deployment so as to fail early and verify that
		// the intermediate config map update occurred
		objs := []client.Object{cm}
		fakeClient := fake.NewClientBuilder().WithObjects(objs...).Build()
		fakeMigrator := &fakeMigrator{
			migratedCorefile: "updated-core-file",
		}

		w := &Workload{
			Client:          fakeClient,
			CoreDNSMigrator: fakeMigrator,
		}

		info := &coreDNSInfo{
			Corefile:               originalCorefile,
			Deployment:             depl,
			CurrentMajorMinorPatch: currentImageTag,
			TargetMajorMinorPatch:  "1.7.2",
		}

		err := w.updateCoreDNSCorefile(ctx, info)
		g.Expect(err).To(HaveOccurred())

		var expectedConfigMap corev1.ConfigMap
		g.Expect(fakeClient.Get(ctx, client.ObjectKey{Name: coreDNSKey, Namespace: metav1.NamespaceSystem}, &expectedConfigMap)).To(Succeed())
		g.Expect(expectedConfigMap.Data).To(HaveLen(2))
		g.Expect(expectedConfigMap.Data).To(HaveKeyWithValue("Corefile", originalCorefile))
		g.Expect(expectedConfigMap.Data).To(HaveKeyWithValue("Corefile-backup", originalCorefile))
	})

	t.Run("patches the core dns deployment to point to the backup corefile before migration", func(t *testing.T) {
		t.Skip("Updating the corefile, after updating controller runtime somehow makes this test fail in a conflict, needs investigation")

		g := NewWithT(t)
		objs := []client.Object{depl, cm}
		fakeClient := fake.NewClientBuilder().WithObjects(objs...).Build()
		fakeMigrator := &fakeMigrator{
			migratedCorefile: "updated-core-file",
		}

		w := &Workload{
			Client:          fakeClient,
			CoreDNSMigrator: fakeMigrator,
		}

		info := &coreDNSInfo{
			Corefile:               originalCorefile,
			Deployment:             depl,
			CurrentMajorMinorPatch: currentImageTag,
			TargetMajorMinorPatch:  "1.7.2",
		}

		err := w.updateCoreDNSCorefile(ctx, info)
		g.Expect(err).ToNot(HaveOccurred())

		expectedVolume := corev1.Volume{
			Name: coreDNSVolumeKey,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: coreDNSKey,
					},
					Items: []corev1.KeyToPath{{
						Key:  "Corefile-backup",
						Path: "Corefile",
					}},
				},
			},
		}

		var actualDeployment appsv1.Deployment
		g.Expect(fakeClient.Get(ctx, client.ObjectKey{Name: coreDNSKey, Namespace: metav1.NamespaceSystem}, &actualDeployment)).To(Succeed())
		g.Expect(actualDeployment.Spec.Template.Spec.Volumes).To(ConsistOf(expectedVolume))

		var expectedConfigMap corev1.ConfigMap
		g.Expect(fakeClient.Get(ctx, client.ObjectKey{Name: coreDNSKey, Namespace: metav1.NamespaceSystem}, &expectedConfigMap)).To(Succeed())
		g.Expect(expectedConfigMap.Data).To(HaveLen(2))
		g.Expect(expectedConfigMap.Data).To(HaveKeyWithValue("Corefile", "updated-core-file"))
		g.Expect(expectedConfigMap.Data).To(HaveKeyWithValue("Corefile-backup", originalCorefile))
	})
}

func TestGetCoreDNSInfo(t *testing.T) {
	t.Run("get coredns info", func(t *testing.T) {
		imageSomeFolder162 := "k8s.gcr.io/some-folder/coredns:1.6.2"
		image162 := "k8s.gcr.io/coredns:1.6.2"

		expectedCorefile := "some-coredns-core-file"
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      coreDNSKey,
				Namespace: metav1.NamespaceSystem,
			},
			Data: map[string]string{
				"Corefile": expectedCorefile,
			},
		}

		emptycm := cm.DeepCopy()
		delete(emptycm.Data, "Corefile")

		emptyDepl := newCoreDNSInfoDeploymentWithimage("")
		emptyDepl.Spec.Template.Spec.Containers = []corev1.Container{}

		clusterConfig := &bootstrapv1.ClusterConfiguration{
			DNS: bootstrapv1.DNS{
				ImageMeta: bootstrapv1.ImageMeta{
					ImageRepository: "myrepo",
					ImageTag:        "1.7.2-foobar.1",
				},
			},
		}
		badImgTagDNS := clusterConfig.DeepCopy()
		badImgTagDNS.DNS.ImageTag = "v1X6.2-foobar.1"

		tests := []struct {
			name              string
			expectErr         bool
			objs              []client.Object
			clusterConfig     *bootstrapv1.ClusterConfiguration
			kubernetesVersion semver.Version
			expectedInfo      coreDNSInfo
		}{
			{
				name:          "returns core dns info",
				objs:          []client.Object{newCoreDNSInfoDeploymentWithimage(imageSomeFolder162), cm},
				clusterConfig: clusterConfig,
				expectedInfo: coreDNSInfo{
					CurrentMajorMinorPatch: "1.6.2",
					FromImageTag:           "1.6.2",
					TargetMajorMinorPatch:  "1.7.2",
					FromImage:              imageSomeFolder162,
					ToImage:                "myrepo/coredns:1.7.2-foobar.1",
					ToImageTag:             "1.7.2-foobar.1",
				},
			},
			{
				name: "uses global config ImageRepository if DNS ImageRepository is not set",
				objs: []client.Object{newCoreDNSInfoDeploymentWithimage(imageSomeFolder162), cm},
				clusterConfig: &bootstrapv1.ClusterConfiguration{
					ImageRepository: "globalRepo/sub-path",
					DNS: bootstrapv1.DNS{
						ImageMeta: bootstrapv1.ImageMeta{
							ImageTag: "1.7.2-foobar.1",
						},
					},
				},
				expectedInfo: coreDNSInfo{
					CurrentMajorMinorPatch: "1.6.2",
					FromImageTag:           "1.6.2",
					TargetMajorMinorPatch:  "1.7.2",
					FromImage:              imageSomeFolder162,
					ToImage:                "globalRepo/sub-path/coredns:1.7.2-foobar.1",
					ToImageTag:             "1.7.2-foobar.1",
				},
			},
			{
				name: "uses DNS ImageRepository config if both global and DNS-level are set",
				objs: []client.Object{newCoreDNSInfoDeploymentWithimage(imageSomeFolder162), cm},
				clusterConfig: &bootstrapv1.ClusterConfiguration{
					ImageRepository: "globalRepo",
					DNS: bootstrapv1.DNS{
						ImageMeta: bootstrapv1.ImageMeta{
							ImageRepository: "dnsRepo",
							ImageTag:        "1.7.2-foobar.1",
						},
					},
				},
				expectedInfo: coreDNSInfo{
					CurrentMajorMinorPatch: "1.6.2",
					FromImageTag:           "1.6.2",
					TargetMajorMinorPatch:  "1.7.2",
					FromImage:              imageSomeFolder162,
					ToImage:                "dnsRepo/coredns:1.7.2-foobar.1",
					ToImageTag:             "1.7.2-foobar.1",
				},
			},
			{
				name: "patches ImageRepository to registry.k8s.io if it's set on neither global nor DNS-level and kubernetesVersion >= v1.25",
				objs: []client.Object{newCoreDNSInfoDeploymentWithimage(imageSomeFolder162), cm},
				clusterConfig: &bootstrapv1.ClusterConfiguration{
					DNS: bootstrapv1.DNS{
						ImageMeta: bootstrapv1.ImageMeta{
							ImageTag: "1.7.2-foobar.1",
						},
					},
				},
				kubernetesVersion: semver.MustParse("1.25.0"),
				expectedInfo: coreDNSInfo{
					CurrentMajorMinorPatch: "1.6.2",
					FromImageTag:           "1.6.2",
					TargetMajorMinorPatch:  "1.7.2",
					FromImage:              imageSomeFolder162,
					ToImage:                "registry.k8s.io/some-folder/coredns:1.7.2-foobar.1",
					ToImageTag:             "1.7.2-foobar.1",
				},
			},
			{
				name: "rename to coredns/coredns when upgrading to coredns=1.8.0 and kubernetesVersion=1.22.16",
				// 1.22.16 uses k8s.gcr.io as default registry. Thus the registry doesn't get changed as
				// FromImage is already using k8s.gcr.io.
				objs: []client.Object{newCoreDNSInfoDeploymentWithimage("k8s.gcr.io/coredns:1.6.2"), cm},
				clusterConfig: &bootstrapv1.ClusterConfiguration{
					DNS: bootstrapv1.DNS{
						ImageMeta: bootstrapv1.ImageMeta{
							ImageTag: "1.8.0",
						},
					},
				},
				kubernetesVersion: semver.MustParse("1.22.16"),
				expectedInfo: coreDNSInfo{
					CurrentMajorMinorPatch: "1.6.2",
					FromImageTag:           "1.6.2",
					TargetMajorMinorPatch:  "1.8.0",
					FromImage:              "k8s.gcr.io/coredns:1.6.2",
					ToImage:                "k8s.gcr.io/coredns/coredns:1.8.0",
					ToImageTag:             "1.8.0",
				},
			},
			{
				name: "rename to coredns/coredns when upgrading to coredns=1.8.0 and kubernetesVersion=1.22.17",
				// 1.22.17 has registry.k8s.io as default registry. Thus the registry gets changed as
				// FromImage is using k8s.gcr.io.
				objs: []client.Object{newCoreDNSInfoDeploymentWithimage("k8s.gcr.io/coredns:1.6.2"), cm},
				clusterConfig: &bootstrapv1.ClusterConfiguration{
					DNS: bootstrapv1.DNS{
						ImageMeta: bootstrapv1.ImageMeta{
							ImageTag: "1.8.0",
						},
					},
				},
				kubernetesVersion: semver.MustParse("1.22.17"),
				expectedInfo: coreDNSInfo{
					CurrentMajorMinorPatch: "1.6.2",
					FromImageTag:           "1.6.2",
					TargetMajorMinorPatch:  "1.8.0",
					FromImage:              "k8s.gcr.io/coredns:1.6.2",
					ToImage:                "registry.k8s.io/coredns/coredns:1.8.0",
					ToImageTag:             "1.8.0",
				},
			},
			{
				name: "rename to coredns/coredns when upgrading to coredns=1.8.0 and kubernetesVersion=1.26.0",
				// 1.26.0 uses registry.k8s.io as default registry. Thus the registry doesn't get changed as
				// FromImage is already using registry.k8s.io.
				objs: []client.Object{newCoreDNSInfoDeploymentWithimage("registry.k8s.io/coredns:1.6.2"), cm},
				clusterConfig: &bootstrapv1.ClusterConfiguration{
					DNS: bootstrapv1.DNS{
						ImageMeta: bootstrapv1.ImageMeta{
							ImageTag: "1.8.0",
						},
					},
				},
				kubernetesVersion: semver.MustParse("1.26.0"),
				expectedInfo: coreDNSInfo{
					CurrentMajorMinorPatch: "1.6.2",
					FromImageTag:           "1.6.2",
					TargetMajorMinorPatch:  "1.8.0",
					FromImage:              "registry.k8s.io/coredns:1.6.2",
					ToImage:                "registry.k8s.io/coredns/coredns:1.8.0",
					ToImageTag:             "1.8.0",
				},
			},
			{
				name: "patches ImageRepository to registry.k8s.io if it's set on neither global nor DNS-level and kubernetesVersion >= v1.22.17 and rename to coredns/coredns",
				// 1.22.17 has registry.k8s.io as default registry. Thus the registry gets changed as
				// FromImage is using k8s.gcr.io.
				objs: []client.Object{newCoreDNSInfoDeploymentWithimage(image162), cm},
				clusterConfig: &bootstrapv1.ClusterConfiguration{
					DNS: bootstrapv1.DNS{
						ImageMeta: bootstrapv1.ImageMeta{
							ImageTag: "1.8.0",
						},
					},
				},
				kubernetesVersion: semver.MustParse("1.22.17"),
				expectedInfo: coreDNSInfo{
					CurrentMajorMinorPatch: "1.6.2",
					FromImageTag:           "1.6.2",
					TargetMajorMinorPatch:  "1.8.0",
					FromImage:              image162,
					ToImage:                "registry.k8s.io/coredns/coredns:1.8.0",
					ToImageTag:             "1.8.0",
				},
			},
			{
				name: "patches ImageRepository to registry.k8s.io if it's set on neither global nor DNS-level and kubernetesVersion >= v1.25 and rename to coredns/coredns",
				objs: []client.Object{newCoreDNSInfoDeploymentWithimage(image162), cm},
				clusterConfig: &bootstrapv1.ClusterConfiguration{
					DNS: bootstrapv1.DNS{
						ImageMeta: bootstrapv1.ImageMeta{
							ImageTag: "1.8.0",
						},
					},
				},
				kubernetesVersion: semver.MustParse("1.25.0"),
				expectedInfo: coreDNSInfo{
					CurrentMajorMinorPatch: "1.6.2",
					FromImageTag:           "1.6.2",
					TargetMajorMinorPatch:  "1.8.0",
					FromImage:              image162,
					ToImage:                "registry.k8s.io/coredns/coredns:1.8.0",
					ToImageTag:             "1.8.0",
				},
			},
			{
				name:          "returns error if unable to find coredns config map",
				objs:          []client.Object{newCoreDNSInfoDeploymentWithimage(imageSomeFolder162)},
				clusterConfig: clusterConfig,
				expectErr:     true,
			},
			{
				name:          "returns error if unable to find coredns deployment",
				objs:          []client.Object{cm},
				clusterConfig: clusterConfig,
				expectErr:     true,
			},
			{
				name:          "returns error if coredns deployment doesn't have coredns container",
				objs:          []client.Object{emptyDepl, cm},
				clusterConfig: clusterConfig,
				expectErr:     true,
			},
			{
				name:          "returns error if unable to find coredns corefile",
				objs:          []client.Object{newCoreDNSInfoDeploymentWithimage(imageSomeFolder162), emptycm},
				clusterConfig: clusterConfig,
				expectErr:     true,
			},
			{
				name:          "returns error if unable to parse the container image",
				objs:          []client.Object{newCoreDNSInfoDeploymentWithimage("k8s.gcr.io/asd:1123/asd:coredns:1.6.1"), cm},
				clusterConfig: clusterConfig,
				expectErr:     true,
			},
			{
				name:          "returns error if container image has not tag",
				objs:          []client.Object{newCoreDNSInfoDeploymentWithimage("k8s.gcr.io/coredns"), cm},
				clusterConfig: clusterConfig,
				expectErr:     true,
			},
			{
				name:          "returns error if unable to semver parse container image",
				objs:          []client.Object{newCoreDNSInfoDeploymentWithimage("k8s.gcr.io/coredns:v1X6.2"), cm},
				clusterConfig: clusterConfig,
				expectErr:     true,
			},
			{
				name:          "returns error if unable to semver parse dns image tag",
				objs:          []client.Object{newCoreDNSInfoDeploymentWithimage(imageSomeFolder162), cm},
				clusterConfig: badImgTagDNS,
				expectErr:     true,
			},
		}
		for i := range tests {
			tt := tests[i]
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)
				fakeClient := fake.NewClientBuilder().WithObjects(tt.objs...).Build()
				w := &Workload{
					Client: fakeClient,
				}

				var actualDepl *appsv1.Deployment
				for _, o := range tt.objs {
					if d, ok := o.(*appsv1.Deployment); ok {
						actualDepl = d
						break
					}
				}

				actualInfo, err := w.getCoreDNSInfo(ctx, tt.clusterConfig, tt.kubernetesVersion)
				if tt.expectErr {
					g.Expect(err).To(HaveOccurred())
					return
				}
				g.Expect(err).ToNot(HaveOccurred())
				tt.expectedInfo.Corefile = expectedCorefile
				tt.expectedInfo.Deployment = actualDepl

				g.Expect(actualInfo).To(BeComparableTo(&tt.expectedInfo))
			})
		}
	})
}

func TestUpdateCoreDNSImageInfoInKubeadmConfigMap(t *testing.T) {
	tests := []struct {
		name                     string
		clusterConfigurationData string
		newDNS                   bootstrapv1.DNS
		wantClusterConfiguration string
	}{
		{
			name: "it should set the DNS image config",
			clusterConfigurationData: utilyaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta2
				kind: ClusterConfiguration
				`),
			newDNS: bootstrapv1.DNS{
				ImageMeta: bootstrapv1.ImageMeta{
					ImageRepository: "example.com/k8s",
					ImageTag:        "v1.2.3",
				},
			},
			wantClusterConfiguration: utilyaml.Raw(`
				apiServer: {}
				apiVersion: kubeadm.k8s.io/v1beta2
				controllerManager: {}
				dns:
				  imageRepository: example.com/k8s
				  imageTag: v1.2.3
				etcd: {}
				kind: ClusterConfiguration
				networking: {}
				scheduler: {}
				`),
		},
	}
	for i := range tests {
		tt := tests[i]
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
			err := w.UpdateClusterConfiguration(ctx, semver.MustParse("1.19.1"), w.updateCoreDNSImageInfoInKubeadmConfigMap(&tt.newDNS))
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

func TestUpdateCoreDNSDeployment(t *testing.T) {
	depl := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      coreDNSKey,
			Namespace: metav1.NamespaceSystem,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: coreDNSKey,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  coreDNSKey,
						Image: "k8s.gcr.io/coredns:1.6.2",
						Args:  []string{"-conf", "/etc/coredns/Corefile"},
					}},
					Volumes: []corev1.Volume{{
						Name: "config-volume",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: coreDNSKey,
								},
								Items: []corev1.KeyToPath{{
									Key:  corefileBackupKey,
									Path: corefileKey,
								}},
							},
						},
					}},
				},
			},
		},
	}

	tests := []struct {
		name      string
		objs      []client.Object
		info      *coreDNSInfo
		expectErr bool
	}{
		{
			name: "patches coredns deployment successfully",
			objs: []client.Object{depl},
			info: &coreDNSInfo{
				Deployment:             depl.DeepCopy(),
				Corefile:               "updated-core-file",
				FromImage:              "k8s.gcr.io/coredns:1.6.2",
				ToImage:                "myrepo/mycoredns:1.7.2-foobar.1",
				CurrentMajorMinorPatch: "1.6.2",
				TargetMajorMinorPatch:  "1.7.2",
			},
		},
		{
			name: "returns error if patch fails",
			objs: []client.Object{},
			info: &coreDNSInfo{
				Deployment:             depl.DeepCopy(),
				Corefile:               "updated-core-file",
				FromImage:              "k8s.gcr.io/coredns:1.6.2",
				ToImage:                "myrepo/mycoredns:1.7.2-foobar.1",
				CurrentMajorMinorPatch: "1.6.2",
				TargetMajorMinorPatch:  "1.7.2",
			},
			expectErr: true,
		},
		{
			name: "deployment is nil for some reason",
			info: &coreDNSInfo{
				Deployment:             nil,
				Corefile:               "updated-core-file",
				FromImage:              "k8s.gcr.io/coredns:1.6.2",
				ToImage:                "myrepo/mycoredns:1.7.2-foobar.1",
				CurrentMajorMinorPatch: "1.6.2",
				TargetMajorMinorPatch:  "1.7.2",
			},
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

			err := w.updateCoreDNSDeployment(ctx, tt.info, semver.Version{Major: 1, Minor: 26, Patch: 0})
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			expectedVolume := corev1.Volume{
				Name: "config-volume",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: coreDNSKey,
						},
						Items: []corev1.KeyToPath{{
							Key:  corefileKey,
							Path: corefileKey,
						}},
					},
				},
			}

			var actualDeployment appsv1.Deployment
			g.Expect(fakeClient.Get(ctx, client.ObjectKey{Name: coreDNSKey, Namespace: metav1.NamespaceSystem}, &actualDeployment)).To(Succeed())
			// ensure the image is updated and the volumes point to the corefile
			g.Expect(actualDeployment.Spec.Template.Spec.Containers[0].Image).To(Equal(tt.info.ToImage))
			g.Expect(actualDeployment.Spec.Template.Spec.Volumes).To(ConsistOf(expectedVolume))
		})
	}
}

func TestPatchCoreDNSDeploymentTolerations(t *testing.T) {
	oldControlPlaneToleration := corev1.Toleration{
		Key:    oldControlPlaneTaint,
		Effect: corev1.TaintEffectNoSchedule,
	}
	controlPlaneToleration := corev1.Toleration{
		Key:    controlPlaneTaint,
		Effect: corev1.TaintEffectNoSchedule,
	}

	tests := []struct {
		name                string
		currentTolerations  []corev1.Toleration
		kubernetesVersion   semver.Version
		expectedTolerations []corev1.Toleration
	}{
		{
			name:               "adds both tolerations for Kubernetes v1.25",
			currentTolerations: []corev1.Toleration{},
			kubernetesVersion:  semver.Version{Major: 1, Minor: 25, Patch: 0},
			expectedTolerations: []corev1.Toleration{
				controlPlaneToleration,
				oldControlPlaneToleration,
			},
		},
		{
			name:               "adds only new toleration for Kubernetes v1.26",
			currentTolerations: []corev1.Toleration{},
			kubernetesVersion:  semver.Version{Major: 1, Minor: 26, Patch: 0},
			expectedTolerations: []corev1.Toleration{
				controlPlaneToleration,
			},
		},
		{
			name: "adds both tolerations for Kubernetes v1.25 and preserves additional tolerations",
			currentTolerations: []corev1.Toleration{
				{
					Key:    "my-special.custom/taint",
					Effect: corev1.TaintEffectNoExecute,
					Value:  "aValue",
				},
			},
			kubernetesVersion: semver.Version{Major: 1, Minor: 25, Patch: 0},
			expectedTolerations: []corev1.Toleration{
				controlPlaneToleration,
				oldControlPlaneToleration,
				{
					Key:    "my-special.custom/taint",
					Effect: corev1.TaintEffectNoExecute,
					Value:  "aValue",
				},
			},
		},
		{
			name: "ensures only new toleration is set for Kubernetes v1.26, drops old toleration and preserves additional tolerations",
			currentTolerations: []corev1.Toleration{
				oldControlPlaneToleration,
				{
					Key:    "my-special.custom/taint",
					Effect: corev1.TaintEffectNoExecute,
					Value:  "aValue",
				},
			},
			kubernetesVersion: semver.Version{Major: 1, Minor: 25, Patch: 0},
			expectedTolerations: []corev1.Toleration{
				controlPlaneToleration,
				oldControlPlaneToleration,
				{
					Key:    "my-special.custom/taint",
					Effect: corev1.TaintEffectNoExecute,
					Value:  "aValue",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			d := &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Tolerations: tt.currentTolerations,
						},
					},
				},
			}

			patchCoreDNSDeploymentTolerations(d, tt.kubernetesVersion)

			g.Expect(d.Spec.Template.Spec.Tolerations).To(BeComparableTo(tt.expectedTolerations))
		})
	}
}

type fakeMigrator struct {
	migrateCalled    bool
	migrateErr       error
	migratedCorefile string
}

func (m *fakeMigrator) Migrate(_, _, _ string, _ bool) (string, error) {
	m.migrateCalled = true
	if m.migrateErr != nil {
		return "", m.migrateErr
	}
	return m.migratedCorefile, nil
}

func newCoreDNSInfoDeploymentWithimage(image string) *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      coreDNSKey,
			Namespace: metav1.NamespaceSystem,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: coreDNSKey,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  coreDNSKey,
						Image: image,
					}},
				},
			},
		},
	}
}
