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
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	cabpkv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	kubeadmv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestUpdateCoreDNS(t *testing.T) {
	validKCP := &controlplanev1.KubeadmControlPlane{
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			KubeadmConfigSpec: cabpkv1.KubeadmConfigSpec{
				ClusterConfiguration: &kubeadmv1.ClusterConfiguration{
					DNS: kubeadmv1.DNS{
						Type: "",
						ImageMeta: kubeadmv1.ImageMeta{
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
		ObjectMeta: v1.ObjectMeta{
			Name:      coreDNSKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			"BadCoreFileKey": "",
		},
	}
	depl := &appsv1.Deployment{
		TypeMeta: v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      coreDNSKey,
			Namespace: metav1.NamespaceSystem,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Name: coreDNSKey,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  coreDNSKey,
						Image: "k8s.gcr.io/some-folder/coredns:1.6.2",
					}},
				},
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
		ObjectMeta: v1.ObjectMeta{
			Name:      coreDNSKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			"Corefile": expectedCorefile,
		},
	}
	kubeadmCM := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      kubeadmConfigKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			"ClusterConfiguration": `apiServer:
apiVersion: kubeadm.k8s.io/v1beta2
dns:
  type: CoreDNS
imageRepository: k8s.gcr.io
kind: ClusterConfiguration
`,
		},
	}

	tests := []struct {
		name          string
		kcp           *controlplanev1.KubeadmControlPlane
		migrator      coreDNSMigrator
		objs          []runtime.Object
		expectErr     bool
		expectUpdates bool
		expectImage   string
	}{
		{
			name: "returns early without error if skip core dns annotation is present",
			kcp: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{
						controlplanev1.SkipCoreDNSAnnotation: "",
					},
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: cabpkv1.KubeadmConfigSpec{
						ClusterConfiguration: &kubeadmv1.ClusterConfiguration{
							DNS: kubeadmv1.DNS{
								Type: "",
							},
						},
					},
				},
			},
			objs:      []runtime.Object{badCM},
			expectErr: false,
		},
		{
			name: "returns early without error if KCP ClusterConfiguration is nil",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: cabpkv1.KubeadmConfigSpec{},
				},
			},
			objs:      []runtime.Object{badCM},
			expectErr: false,
		},
		{
			name: "returns early without error if KCP Cluster config DNS is not empty && not CoreDNS",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: cabpkv1.KubeadmConfigSpec{
						ClusterConfiguration: &kubeadmv1.ClusterConfiguration{
							DNS: kubeadmv1.DNS{
								Type: "foobarDNS",
							},
						},
					},
				},
			},
			objs:      []runtime.Object{badCM},
			expectErr: false,
		},
		{
			name:      "returns early without error if CoreDNS info is not found",
			kcp:       validKCP,
			expectErr: false,
		},
		{
			name:      "returns error if there was a problem retrieving CoreDNS info",
			kcp:       validKCP,
			objs:      []runtime.Object{badCM},
			expectErr: true,
		},
		{
			name:      "returns early without error if CoreDNS fromImage == ToImage",
			kcp:       validKCP,
			objs:      []runtime.Object{depl, cm},
			expectErr: false,
		},
		{
			name: "returns error if validation of CoreDNS image tag fails",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: cabpkv1.KubeadmConfigSpec{
						ClusterConfiguration: &kubeadmv1.ClusterConfiguration{
							DNS: kubeadmv1.DNS{
								Type: kubeadmv1.CoreDNS,
								ImageMeta: kubeadmv1.ImageMeta{
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
			objs:      []runtime.Object{depl, cm},
			expectErr: true,
		},
		{
			name: "returns error if unable to update CoreDNS image info in kubeadm config map",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: cabpkv1.KubeadmConfigSpec{
						ClusterConfiguration: &kubeadmv1.ClusterConfiguration{
							DNS: kubeadmv1.DNS{
								Type: kubeadmv1.CoreDNS,
								ImageMeta: kubeadmv1.ImageMeta{
									// provide an newer image to update to
									ImageRepository: "k8s.gcr.io/some-folder/coredns",
									ImageTag:        "1.7.2",
								},
							},
						},
					},
				},
			},
			// no kubeadmConfigMap available so it will trigger an error
			objs:      []runtime.Object{depl, cm},
			expectErr: true,
		},
		{
			name: "returns error if unable to update CoreDNS corefile",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: cabpkv1.KubeadmConfigSpec{
						ClusterConfiguration: &kubeadmv1.ClusterConfiguration{
							DNS: kubeadmv1.DNS{
								Type: kubeadmv1.CoreDNS,
								ImageMeta: kubeadmv1.ImageMeta{
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
			objs:      []runtime.Object{depl, cm, kubeadmCM},
			expectErr: true,
		},
		{
			name: "updates everything successfully",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: cabpkv1.KubeadmConfigSpec{
						ClusterConfiguration: &kubeadmv1.ClusterConfiguration{
							DNS: kubeadmv1.DNS{
								Type: kubeadmv1.CoreDNS,
								ImageMeta: kubeadmv1.ImageMeta{
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
			objs:          []runtime.Object{depl, cm, kubeadmCM},
			expectErr:     false,
			expectUpdates: true,
			expectImage:   "k8s.gcr.io/some-repo/coredns:1.7.2",
		},
		{
			name: "updates everything successfully to v1.8.0 with a custom repo should not change the image name",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: cabpkv1.KubeadmConfigSpec{
						ClusterConfiguration: &kubeadmv1.ClusterConfiguration{
							DNS: kubeadmv1.DNS{
								Type: kubeadmv1.CoreDNS,
								ImageMeta: kubeadmv1.ImageMeta{
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
			objs:          []runtime.Object{deplWithImage("k8s.gcr.io/some-repo/coredns:1.7.0"), cm, kubeadmCM},
			expectErr:     false,
			expectUpdates: true,
			expectImage:   "k8s.gcr.io/some-repo/coredns:1.8.0",
		},
		{
			name: "kubeadm defaults, upgrade from Kubernetes v1.18.x to v1.19.y (from k8s.gcr.io/coredns:1.6.7 to k8s.gcr.io/coredns:1.7.0)",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: cabpkv1.KubeadmConfigSpec{
						ClusterConfiguration: &kubeadmv1.ClusterConfiguration{
							DNS: kubeadmv1.DNS{
								Type: kubeadmv1.CoreDNS,
								ImageMeta: kubeadmv1.ImageMeta{
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
			objs:          []runtime.Object{deplWithImage("k8s.gcr.io/coredns:1.6.7"), cm, kubeadmCM},
			expectErr:     false,
			expectUpdates: true,
			expectImage:   "k8s.gcr.io/coredns:1.7.0",
		},
		{
			name: "kubeadm defaults, upgrade from Kubernetes v1.19.x to v1.20.y (stay on k8s.gcr.io/coredns:1.7.0)",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: cabpkv1.KubeadmConfigSpec{
						ClusterConfiguration: &kubeadmv1.ClusterConfiguration{
							DNS: kubeadmv1.DNS{
								Type: kubeadmv1.CoreDNS,
								ImageMeta: kubeadmv1.ImageMeta{
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
			objs:          []runtime.Object{deplWithImage("k8s.gcr.io/coredns:1.7.0"), cm, kubeadmCM},
			expectErr:     false,
			expectUpdates: false,
		},
		{
			name: "kubeadm defaults, upgrade from Kubernetes v1.20.x to v1.21.y (from k8s.gcr.io/coredns:1.7.0 to k8s.gcr.io/coredns/coredns:v1.8.0)",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: cabpkv1.KubeadmConfigSpec{
						ClusterConfiguration: &kubeadmv1.ClusterConfiguration{
							DNS: kubeadmv1.DNS{
								Type: kubeadmv1.CoreDNS,
								ImageMeta: kubeadmv1.ImageMeta{
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
			objs:          []runtime.Object{deplWithImage("k8s.gcr.io/coredns:1.7.0"), cm, kubeadmCM},
			expectErr:     false,
			expectUpdates: true,
			expectImage:   "k8s.gcr.io/coredns/coredns:v1.8.0", // NOTE: ImageName has coredns/coredns
		},
		{
			name: "kubeadm defaults, upgrade from Kubernetes v1.21.x to v1.22.y (stay on k8s.gcr.io/coredns/coredns:v1.8.0)",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: cabpkv1.KubeadmConfigSpec{
						ClusterConfiguration: &kubeadmv1.ClusterConfiguration{
							DNS: kubeadmv1.DNS{
								Type: kubeadmv1.CoreDNS,
								ImageMeta: kubeadmv1.ImageMeta{
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
			objs:          []runtime.Object{deplWithImage("k8s.gcr.io/coredns/coredns:v1.8.0"), cm, kubeadmCM},
			expectErr:     false,
			expectUpdates: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, tt.objs...)
			w := &Workload{
				Client:          fakeClient,
				CoreDNSMigrator: tt.migrator,
			}
			err := w.UpdateCoreDNS(context.Background(), tt.kcp)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			// Assert that CoreDNS updates have been made
			if tt.expectUpdates {
				// assert kubeadmConfigMap
				var expectedKubeadmConfigMap corev1.ConfigMap
				g.Expect(fakeClient.Get(context.TODO(), ctrlclient.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem}, &expectedKubeadmConfigMap)).To(Succeed())
				g.Expect(expectedKubeadmConfigMap.Data).To(HaveKeyWithValue("ClusterConfiguration", ContainSubstring(tt.kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS.ImageTag)))
				g.Expect(expectedKubeadmConfigMap.Data).To(HaveKeyWithValue("ClusterConfiguration", ContainSubstring(tt.kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS.ImageRepository)))

				// assert CoreDNS corefile
				var expectedConfigMap corev1.ConfigMap
				g.Expect(fakeClient.Get(context.TODO(), ctrlclient.ObjectKey{Name: coreDNSKey, Namespace: metav1.NamespaceSystem}, &expectedConfigMap)).To(Succeed())
				g.Expect(expectedConfigMap.Data).To(HaveLen(2))
				g.Expect(expectedConfigMap.Data).To(HaveKeyWithValue("Corefile", "updated-core-file"))
				g.Expect(expectedConfigMap.Data).To(HaveKeyWithValue("Corefile-backup", expectedCorefile))

				// assert CoreDNS deployment
				var actualDeployment appsv1.Deployment
				g.Eventually(func() string {
					g.Expect(fakeClient.Get(context.TODO(), ctrlclient.ObjectKey{Name: coreDNSKey, Namespace: metav1.NamespaceSystem}, &actualDeployment)).To(Succeed())
					return actualDeployment.Spec.Template.Spec.Containers[0].Image
				}, "5s").Should(Equal(tt.expectImage))
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
			name:            "fromVer is equal to toVer",
			fromVer:         "1.6.5_foobar.1",
			toVer:           "1.6.5_foobar.1",
			expectErrSubStr: "must be greater",
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

func TestUpdateCoreDNSCorefile(t *testing.T) {
	currentImageTag := "1.6.2"
	originalCorefile := "some-coredns-core-file"
	depl := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      coreDNSKey,
			Namespace: metav1.NamespaceSystem,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
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
		ObjectMeta: v1.ObjectMeta{
			Name:      coreDNSKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			"Corefile": originalCorefile,
		},
	}

	t.Run("returns error if migrate failed to update corefile", func(t *testing.T) {
		g := NewWithT(t)
		objs := []runtime.Object{depl, cm}
		fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, objs...)
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

		err := w.updateCoreDNSCorefile(context.TODO(), info)
		g.Expect(err).To(HaveOccurred())
		g.Expect(fakeMigrator.migrateCalled).To(BeTrue())

		var expectedConfigMap corev1.ConfigMap
		g.Expect(fakeClient.Get(context.TODO(), ctrlclient.ObjectKey{Name: coreDNSKey, Namespace: metav1.NamespaceSystem}, &expectedConfigMap)).To(Succeed())
		g.Expect(expectedConfigMap.Data).To(HaveLen(1))
		g.Expect(expectedConfigMap.Data).To(HaveKeyWithValue("Corefile", originalCorefile))
	})

	t.Run("creates a backup of the corefile", func(t *testing.T) {
		g := NewWithT(t)
		// Not including the deployment so as to fail early and verify that
		// the intermediate config map update occurred
		objs := []runtime.Object{cm}
		fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, objs...)
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

		err := w.updateCoreDNSCorefile(context.TODO(), info)
		g.Expect(err).To(HaveOccurred())

		var expectedConfigMap corev1.ConfigMap
		g.Expect(fakeClient.Get(context.TODO(), ctrlclient.ObjectKey{Name: coreDNSKey, Namespace: metav1.NamespaceSystem}, &expectedConfigMap)).To(Succeed())
		g.Expect(expectedConfigMap.Data).To(HaveLen(2))
		g.Expect(expectedConfigMap.Data).To(HaveKeyWithValue("Corefile", originalCorefile))
		g.Expect(expectedConfigMap.Data).To(HaveKeyWithValue("Corefile-backup", originalCorefile))
	})

	t.Run("patches the core dns deployment to point to the backup corefile before migration", func(t *testing.T) {
		g := NewWithT(t)
		objs := []runtime.Object{depl, cm}
		fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, objs...)
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

		err := w.updateCoreDNSCorefile(context.TODO(), info)
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
		g.Expect(fakeClient.Get(context.TODO(), ctrlclient.ObjectKey{Name: coreDNSKey, Namespace: metav1.NamespaceSystem}, &actualDeployment)).To(Succeed())
		g.Expect(actualDeployment.Spec.Template.Spec.Volumes).To(ConsistOf(expectedVolume))

		var expectedConfigMap corev1.ConfigMap
		g.Expect(fakeClient.Get(context.TODO(), ctrlclient.ObjectKey{Name: coreDNSKey, Namespace: metav1.NamespaceSystem}, &expectedConfigMap)).To(Succeed())
		g.Expect(expectedConfigMap.Data).To(HaveLen(2))
		g.Expect(expectedConfigMap.Data).To(HaveKeyWithValue("Corefile", "updated-core-file"))
		g.Expect(expectedConfigMap.Data).To(HaveKeyWithValue("Corefile-backup", originalCorefile))
	})
}

func TestGetCoreDNSInfo(t *testing.T) {
	t.Run("get coredns info", func(t *testing.T) {
		expectedImage := "k8s.gcr.io/some-folder/coredns:1.6.2"
		depl := &appsv1.Deployment{
			TypeMeta: v1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      coreDNSKey,
				Namespace: metav1.NamespaceSystem,
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Name: coreDNSKey,
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  coreDNSKey,
							Image: expectedImage,
						}},
					},
				},
			},
		}

		expectedCorefile := "some-coredns-core-file"
		cm := &corev1.ConfigMap{
			ObjectMeta: v1.ObjectMeta{
				Name:      coreDNSKey,
				Namespace: metav1.NamespaceSystem,
			},
			Data: map[string]string{
				"Corefile": expectedCorefile,
			},
		}

		emptycm := cm.DeepCopy()
		delete(emptycm.Data, "Corefile")

		emptyDepl := depl.DeepCopy()
		emptyDepl.Spec.Template.Spec.Containers = []corev1.Container{}

		badContainerDepl := depl.DeepCopy()
		badContainerDepl.Spec.Template.Spec.Containers[0].Image = "k8s.gcr.io/asd:1123/asd:coredns:1.6.1"

		noTagContainerDepl := depl.DeepCopy()
		noTagContainerDepl.Spec.Template.Spec.Containers[0].Image = "k8s.gcr.io/coredns"

		badSemverContainerDepl := depl.DeepCopy()
		badSemverContainerDepl.Spec.Template.Spec.Containers[0].Image = "k8s.gcr.io/coredns:v1X6.2"

		clusterConfig := &kubeadmv1.ClusterConfiguration{
			DNS: kubeadmv1.DNS{
				ImageMeta: kubeadmv1.ImageMeta{
					ImageRepository: "myrepo",
					ImageTag:        "1.7.2-foobar.1",
				},
			},
		}
		badImgTagDNS := clusterConfig.DeepCopy()
		badImgTagDNS.DNS.ImageTag = "v1X6.2-foobar.1"

		tests := []struct {
			name          string
			expectErr     bool
			objs          []runtime.Object
			clusterConfig *kubeadmv1.ClusterConfiguration
			toImage       string
		}{
			{
				name:          "returns core dns info",
				objs:          []runtime.Object{depl, cm},
				clusterConfig: clusterConfig,
				toImage:       "myrepo/coredns:1.7.2-foobar.1",
			},
			{
				name: "uses global config ImageRepository if DNS ImageRepository is not set",
				objs: []runtime.Object{depl, cm},
				clusterConfig: &kubeadmv1.ClusterConfiguration{
					ImageRepository: "globalRepo/sub-path",
					DNS: kubeadmv1.DNS{
						ImageMeta: kubeadmv1.ImageMeta{
							ImageTag: "1.7.2-foobar.1",
						},
					},
				},
				toImage: "globalRepo/sub-path/coredns:1.7.2-foobar.1",
			},
			{
				name: "uses DNS ImageRepository config if both global and DNS-level are set",
				objs: []runtime.Object{depl, cm},
				clusterConfig: &kubeadmv1.ClusterConfiguration{
					ImageRepository: "globalRepo",
					DNS: kubeadmv1.DNS{
						ImageMeta: kubeadmv1.ImageMeta{
							ImageRepository: "dnsRepo",
							ImageTag:        "1.7.2-foobar.1",
						},
					},
				},
				toImage: "dnsRepo/coredns:1.7.2-foobar.1",
			},
			{
				name:          "returns error if unable to find coredns config map",
				objs:          []runtime.Object{depl},
				clusterConfig: clusterConfig,
				expectErr:     true,
			},
			{
				name:          "returns error if unable to find coredns deployment",
				objs:          []runtime.Object{cm},
				clusterConfig: clusterConfig,
				expectErr:     true,
			},
			{
				name:          "returns error if coredns deployment doesn't have coredns container",
				objs:          []runtime.Object{emptyDepl, cm},
				clusterConfig: clusterConfig,
				expectErr:     true,
			},
			{
				name:          "returns error if unable to find coredns corefile",
				objs:          []runtime.Object{depl, emptycm},
				clusterConfig: clusterConfig,
				expectErr:     true,
			},
			{
				name:          "returns error if unable to parse the container image",
				objs:          []runtime.Object{badContainerDepl, cm},
				clusterConfig: clusterConfig,
				expectErr:     true,
			},
			{
				name:          "returns error if container image has not tag",
				objs:          []runtime.Object{noTagContainerDepl, cm},
				clusterConfig: clusterConfig,
				expectErr:     true,
			},
			{
				name:          "returns error if unable to semver parse container image",
				objs:          []runtime.Object{badSemverContainerDepl, cm},
				clusterConfig: clusterConfig,
				expectErr:     true,
			},
			{
				name:          "returns error if unable to semver parse dns image tag",
				objs:          []runtime.Object{depl, cm},
				clusterConfig: badImgTagDNS,
				expectErr:     true,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)
				fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, tt.objs...)
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

				actualInfo, err := w.getCoreDNSInfo(context.TODO(), tt.clusterConfig)
				if tt.expectErr {
					g.Expect(err).To(HaveOccurred())
					return
				}
				g.Expect(err).ToNot(HaveOccurred())
				expectedInfo := &coreDNSInfo{
					Corefile:               expectedCorefile,
					Deployment:             actualDepl,
					CurrentMajorMinorPatch: "1.6.2",
					TargetMajorMinorPatch:  "1.7.2",
					FromImage:              expectedImage,
					ToImage:                tt.toImage,
					FromImageTag:           "1.6.2",
					ToImageTag:             "1.7.2-foobar.1",
				}

				g.Expect(actualInfo).To(Equal(expectedInfo))
			})
		}
	})
}

func TestUpdateCoreDNSImageInfoInKubeadmConfigMap(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      kubeadmConfigKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			"ClusterConfiguration": `apiServer:
  extraArgs:
    authorization-mode: Node,RBAC
    cloud-provider: aws
  timeoutForControlPlane: 4m0s
apiVersion: kubeadm.k8s.io/v1beta2
certificatesDir: /etc/kubernetes/pki
clusterName: foobar
controlPlaneEndpoint: foobar.us-east-2.elb.amazonaws.com
controllerManager:
  extraArgs:
    cloud-provider: aws
dns:
  type: CoreDNS
etcd:
  local:
    dataDir: /var/lib/etcd
imageRepository: k8s.gcr.io
kind: ClusterConfiguration
kubernetesVersion: v1.16.1
networking:
  dnsDomain: cluster.local
  podSubnet: 192.168.0.0/16
  serviceSubnet: 10.96.0.0/12
scheduler: {}`,
		},
	}

	emptyCM := cm.DeepCopy()
	delete(emptyCM.Data, "ClusterConfiguration")

	dns := &kubeadmv1.DNS{
		Type: kubeadmv1.CoreDNS,
		ImageMeta: kubeadmv1.ImageMeta{
			ImageRepository: "gcr.io/example",
			ImageTag:        "1.0.1-somever.1",
		},
	}

	tests := []struct {
		name      string
		dns       *kubeadmv1.DNS
		objs      []runtime.Object
		expectErr bool
	}{
		{
			name:      "returns error if unable to find config map",
			dns:       dns,
			expectErr: true,
		},
		{
			name:      "returns error if config map is empty",
			objs:      []runtime.Object{emptyCM},
			dns:       dns,
			expectErr: true,
		},
		{
			name:      "succeeds if updates correctly",
			dns:       dns,
			objs:      []runtime.Object{cm},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, tt.objs...)
			w := &Workload{
				Client: fakeClient,
			}

			err := w.updateCoreDNSImageInfoInKubeadmConfigMap(context.TODO(), tt.dns)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			var expectedConfigMap corev1.ConfigMap
			g.Expect(fakeClient.Get(context.TODO(), ctrlclient.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem}, &expectedConfigMap)).To(Succeed())
			g.Expect(expectedConfigMap.Data).To(HaveKeyWithValue("ClusterConfiguration", ContainSubstring("1.0.1-somever.1")))
			g.Expect(expectedConfigMap.Data).To(HaveKeyWithValue("ClusterConfiguration", ContainSubstring("gcr.io/example")))
		})
	}
}

func TestUpdateCoreDNSDeployment(t *testing.T) {
	depl := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      coreDNSKey,
			Namespace: metav1.NamespaceSystem,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
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
		objs      []runtime.Object
		info      *coreDNSInfo
		expectErr bool
	}{
		{
			name: "patches coredns deployment successfully",
			objs: []runtime.Object{depl},
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
			objs: []runtime.Object{},
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
			fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, tt.objs...)

			w := &Workload{
				Client: fakeClient,
			}

			err := w.updateCoreDNSDeployment(context.TODO(), tt.info)
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
			g.Expect(fakeClient.Get(context.TODO(), ctrlclient.ObjectKey{Name: coreDNSKey, Namespace: metav1.NamespaceSystem}, &actualDeployment)).To(Succeed())
			// ensure the image is updated and the volumes point to the corefile
			g.Expect(actualDeployment.Spec.Template.Spec.Containers[0].Image).To(Equal(tt.info.ToImage))
			g.Expect(actualDeployment.Spec.Template.Spec.Volumes).To(ConsistOf(expectedVolume))
		})
	}
}

type fakeMigrator struct {
	migrateCalled    bool
	migrateErr       error
	migratedCorefile string
}

func (m *fakeMigrator) Migrate(current, to, corefile string, deprecations bool) (string, error) {
	m.migrateCalled = true
	if m.migrateErr != nil {
		return "", m.migrateErr
	}
	return m.migratedCorefile, nil
}
