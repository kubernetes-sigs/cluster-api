/*
Copyright 2019 The Kubernetes Authors.

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

package v1alpha2

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1a2 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha2"
	bootstrapv1a3 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func TestFuzzyConversion(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha3.AddToScheme(scheme)).To(Succeed())

	t.Run("for Cluster", utilconversion.FuzzTestFunc(scheme, &v1alpha3.Cluster{}, &Cluster{}))
	t.Run("for Machine", utilconversion.FuzzTestFunc(scheme, &v1alpha3.Machine{}, &Machine{}))
	t.Run("for MachineSet", utilconversion.FuzzTestFunc(scheme, &v1alpha3.MachineSet{}, &MachineSet{}))
	t.Run("for MachineDeployment", utilconversion.FuzzTestFunc(scheme, &v1alpha3.MachineDeployment{}, &MachineDeployment{}))
}

func TestConvertCluster(t *testing.T) {
	g := NewWithT(t)

	t.Run("to hub", func(t *testing.T) {
		t.Run("should convert the first value in Status.APIEndpoints to Spec.ControlPlaneEndpoint", func(t *testing.T) {
			src := &Cluster{
				Status: ClusterStatus{
					APIEndpoints: []APIEndpoint{
						{
							Host: "example.com",
							Port: 6443,
						},
					},
				},
			}
			dst := &v1alpha3.Cluster{}

			g.Expect(src.ConvertTo(dst)).To(Succeed())
			g.Expect(dst.Spec.ControlPlaneEndpoint.Host).To(Equal("example.com"))
			g.Expect(dst.Spec.ControlPlaneEndpoint.Port).To(BeEquivalentTo(6443))
		})
	})

	t.Run("from hub", func(t *testing.T) {
		t.Run("preserves fields from hub version", func(t *testing.T) {
			src := &v1alpha3.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hub",
				},
				Spec: v1alpha3.ClusterSpec{
					ControlPlaneRef: &corev1.ObjectReference{
						Name: "controlplane-1",
					},
				},
				Status: v1alpha3.ClusterStatus{
					ControlPlaneReady: true,
				},
			}
			dst := &Cluster{}

			g.Expect(dst.ConvertFrom(src)).To(Succeed())
			restored := &v1alpha3.Cluster{}
			g.Expect(dst.ConvertTo(restored)).To(Succeed())

			// Test field restored fields.
			g.Expect(restored.Name).To(Equal(src.Name))
			g.Expect(restored.Spec.ControlPlaneRef).To(Equal(src.Spec.ControlPlaneRef))
			g.Expect(restored.Status.ControlPlaneReady).To(Equal(src.Status.ControlPlaneReady))
		})

		t.Run("should convert Spec.ControlPlaneEndpoint to Status.APIEndpoints[0]", func(t *testing.T) {
			src := &v1alpha3.Cluster{
				Spec: v1alpha3.ClusterSpec{
					ControlPlaneEndpoint: v1alpha3.APIEndpoint{
						Host: "example.com",
						Port: 6443,
					},
				},
			}
			dst := &Cluster{}

			g.Expect(dst.ConvertFrom(src)).To(Succeed())
			g.Expect(dst.Status.APIEndpoints[0].Host).To(Equal("example.com"))
			g.Expect(dst.Status.APIEndpoints[0].Port).To(BeEquivalentTo(6443))
		})
	})
}

func TestConvertMachine(t *testing.T) {
	g := NewWithT(t)

	t.Run("to hub", func(t *testing.T) {
		t.Run("should convert all fields correctly", func(t *testing.T) {
			src := &Machine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						MachineClusterLabelName: "test-cluster",
					},
				},
				Spec: MachineSpec{
					Bootstrap: Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							Kind:       kubeadmConfigKind,
							APIVersion: bootstrapv1a2.GroupVersion.String(),
						},
					},
				},
			}
			dst := &v1alpha3.Machine{}

			g.Expect(src.ConvertTo(dst)).To(Succeed())
			g.Expect(dst.Spec.ClusterName).To(Equal("test-cluster"))
			g.Expect(dst.Spec.Bootstrap.ConfigRef.APIVersion).To(Equal(bootstrapv1a3.GroupVersion.String()))
		})
	})

	t.Run("from hub", func(t *testing.T) {
		t.Run("preserves fields from hub version", func(t *testing.T) {
			failureDomain := "my failure domain"
			src := &v1alpha3.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hub",
				},
				Spec: v1alpha3.MachineSpec{
					ClusterName: "test-cluster",
					Bootstrap: v1alpha3.Bootstrap{
						DataSecretName: pointer.StringPtr("secret-data"),
					},
					FailureDomain: &failureDomain,
				},
			}
			dst := &Machine{}

			g.Expect(dst.ConvertFrom(src)).To(Succeed())
			restored := &v1alpha3.Machine{}
			g.Expect(dst.ConvertTo(restored)).To(Succeed())

			// Test field restored fields.
			g.Expect(restored.Name).To(Equal(src.Name))
			g.Expect(restored.Spec.Bootstrap.DataSecretName).To(Equal(src.Spec.Bootstrap.DataSecretName))
			g.Expect(restored.Spec.ClusterName).To(Equal(src.Spec.ClusterName))
			g.Expect(restored.Spec.FailureDomain).To(Equal(src.Spec.FailureDomain))
		})
	})
}

func TestConvertMachineSet(t *testing.T) {
	g := NewWithT(t)

	t.Run("to hub", func(t *testing.T) {
		t.Run("should convert all fields correctly", func(t *testing.T) {
			src := &MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						MachineClusterLabelName: "test-cluster",
					},
				},
				Spec: MachineSetSpec{
					Template: MachineTemplateSpec{
						Spec: MachineSpec{
							Bootstrap: Bootstrap{
								ConfigRef: &corev1.ObjectReference{
									Kind:       kubeadmConfigKind,
									APIVersion: bootstrapv1a2.GroupVersion.String(),
								},
							},
						},
					},
				},
			}
			dst := &v1alpha3.MachineSet{}

			g.Expect(src.ConvertTo(dst)).To(Succeed())
			g.Expect(dst.Spec.ClusterName).To(Equal("test-cluster"))
			g.Expect(dst.Spec.Template.Spec.ClusterName).To(Equal("test-cluster"))
			g.Expect(dst.Spec.Template.Spec.Bootstrap.ConfigRef.APIVersion).To(Equal(bootstrapv1a3.GroupVersion.String()))
		})
	})

	t.Run("from hub", func(t *testing.T) {
		t.Run("preserves field from hub version", func(t *testing.T) {
			src := &v1alpha3.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hub",
				},
				Spec: v1alpha3.MachineSetSpec{
					ClusterName: "test-cluster",
					Template: v1alpha3.MachineTemplateSpec{
						Spec: v1alpha3.MachineSpec{
							ClusterName: "test-cluster",
						},
					},
				},
			}
			dst := &MachineSet{}

			g.Expect(dst.ConvertFrom(src)).To(Succeed())
			restored := &v1alpha3.MachineSet{}
			g.Expect(dst.ConvertTo(restored)).To(Succeed())

			// Test field restored fields.
			g.Expect(restored.Name).To(Equal(src.Name))
			g.Expect(restored.Spec.ClusterName).To(Equal(src.Spec.ClusterName))
			g.Expect(restored.Spec.Template.Spec.ClusterName).To(Equal(src.Spec.Template.Spec.ClusterName))
		})
	})
}

func TestConvertMachineDeployment(t *testing.T) {
	g := NewWithT(t)

	t.Run("to hub", func(t *testing.T) {
		t.Run("should convert all fields correctly", func(t *testing.T) {
			src := &MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						MachineClusterLabelName: "test-cluster",
					},
				},
				Spec: MachineDeploymentSpec{
					Paused: true,
					Template: MachineTemplateSpec{
						Spec: MachineSpec{
							Bootstrap: Bootstrap{
								ConfigRef: &corev1.ObjectReference{
									Kind:       kubeadmConfigTemplateKind,
									APIVersion: bootstrapv1a2.GroupVersion.String(),
								},
							},
						},
					},
				},
			}
			dst := &v1alpha3.MachineDeployment{}

			g.Expect(src.ConvertTo(dst)).To(Succeed())
			g.Expect(dst.Spec.ClusterName).To(Equal("test-cluster"))
			g.Expect(dst.Spec.Template.Spec.ClusterName).To(Equal("test-cluster"))
			g.Expect(dst.Spec.Template.Spec.Bootstrap.ConfigRef.APIVersion).To(Equal(bootstrapv1a3.GroupVersion.String()))
		})
	})

	t.Run("from hub", func(t *testing.T) {
		t.Run("preserves fields from hub version", func(t *testing.T) {
			src := &v1alpha3.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hub",
				},
				Spec: v1alpha3.MachineDeploymentSpec{
					ClusterName: "test-cluster",
					Paused:      true,
					Template: v1alpha3.MachineTemplateSpec{
						Spec: v1alpha3.MachineSpec{
							ClusterName: "test-cluster",
						},
					},
				},
			}
			src.Status.SetTypedPhase(v1alpha3.MachineDeploymentPhaseRunning)
			dst := &MachineDeployment{}
			g.Expect(dst.ConvertFrom(src)).To(Succeed())
			restored := &v1alpha3.MachineDeployment{}
			g.Expect(dst.ConvertTo(restored)).To(Succeed())

			// Test field restored fields.
			g.Expect(restored.Name).To(Equal(src.Name))
			g.Expect(restored.Spec.ClusterName).To(Equal(src.Spec.ClusterName))
			g.Expect(restored.Spec.Paused).To(Equal(src.Spec.Paused))
			g.Expect(restored.Spec.Template.Spec.ClusterName).To(Equal(src.Spec.Template.Spec.ClusterName))
		})
	})
}
