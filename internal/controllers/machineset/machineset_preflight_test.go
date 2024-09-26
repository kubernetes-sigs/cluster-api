/*
Copyright 2023 The Kubernetes Authors.

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

package machineset

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/test/builder"
)

func TestMachineSetReconciler_runPreflightChecks(t *testing.T) {
	ns := "ns1"

	controlPlaneWithNoVersion := builder.ControlPlane(ns, "cp1").Build()

	controlPlaneWithInvalidVersion := builder.ControlPlane(ns, "cp1").
		WithVersion("v1.25.6.0").Build()

	controlPlaneProvisioning := builder.ControlPlane(ns, "cp1").
		WithVersion("v1.25.6").Build()

	controlPlaneUpgrading := builder.ControlPlane(ns, "cp1").
		WithVersion("v1.26.2").
		WithStatusFields(map[string]interface{}{
			"status.version": "v1.25.2",
		}).
		Build()

	controlPlaneStable := builder.ControlPlane(ns, "cp1").
		WithVersion("v1.26.2").
		WithStatusFields(map[string]interface{}{
			"status.version": "v1.26.2",
		}).
		Build()

	controlPlaneStable128 := builder.ControlPlane(ns, "cp1").
		WithVersion("v1.28.0").
		WithStatusFields(map[string]interface{}{
			"status.version": "v1.28.0",
		}).
		Build()

	t.Run("should run preflight checks if the feature gate is enabled", func(t *testing.T) {
		tests := []struct {
			name         string
			cluster      *clusterv1.Cluster
			controlPlane *unstructured.Unstructured
			machineSet   *clusterv1.MachineSet
			wantPass     bool
			wantErr      bool
		}{
			{
				name:       "should pass if cluster has no control plane",
				cluster:    &clusterv1.Cluster{},
				machineSet: &clusterv1.MachineSet{},
				wantPass:   true,
			},
			{
				name: "should pass if the control plane version is not defined",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToRef(controlPlaneWithNoVersion),
					},
				},
				controlPlane: controlPlaneWithNoVersion,
				machineSet:   &clusterv1.MachineSet{},
				wantPass:     true,
			},
			{
				name: "should error if the control plane version is invalid",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToRef(controlPlaneWithInvalidVersion),
					},
				},
				controlPlane: controlPlaneWithInvalidVersion,
				machineSet:   &clusterv1.MachineSet{},
				wantErr:      true,
			},
			{
				name: "should pass if all preflight checks are skipped",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToRef(controlPlaneUpgrading),
					},
				},
				controlPlane: controlPlaneUpgrading,
				machineSet: &clusterv1.MachineSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Annotations: map[string]string{
							clusterv1.MachineSetSkipPreflightChecksAnnotation: string(clusterv1.MachineSetPreflightCheckAll),
						},
					},
				},
				wantPass: true,
			},
			{
				name: "control plane preflight check: should fail if the control plane is provisioning",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToRef(controlPlaneProvisioning),
					},
				},
				controlPlane: controlPlaneProvisioning,
				machineSet:   &clusterv1.MachineSet{},
				wantPass:     false,
			},
			{
				name: "control plane preflight check: should fail if the control plane is upgrading",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToRef(controlPlaneUpgrading),
					},
				},
				controlPlane: controlPlaneUpgrading,
				machineSet:   &clusterv1.MachineSet{},
				wantPass:     false,
			},
			{
				name: "control plane preflight check: should pass if the control plane is upgrading but the preflight check is skipped",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToRef(controlPlaneUpgrading),
					},
				},
				controlPlane: controlPlaneUpgrading,
				machineSet: &clusterv1.MachineSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Annotations: map[string]string{
							clusterv1.MachineSetSkipPreflightChecksAnnotation: string(clusterv1.MachineSetPreflightCheckControlPlaneIsStable),
						},
					},
					Spec: clusterv1.MachineSetSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version:   ptr.To("v1.26.2"),
								Bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{Kind: "KubeadmConfigTemplate"}},
							},
						},
					},
				},
				wantPass: true,
			},
			{
				name: "control plane preflight check: should pass if the control plane is stable",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToRef(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machineSet:   &clusterv1.MachineSet{},
				wantPass:     true,
			},
			{
				name: "should pass if the machine set version is not defined",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToRef(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machineSet: &clusterv1.MachineSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.MachineSetSpec{},
				},
				wantPass: true,
			},
			{
				name: "should error if the machine set version is invalid",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToRef(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machineSet: &clusterv1.MachineSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.MachineSetSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: ptr.To("v1.27.0.0"),
							},
						},
					},
				},
				wantErr: true,
			},
			{
				name: "kubernetes version preflight check: should fail if the machine set minor version is greater than control plane minor version",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToRef(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machineSet: &clusterv1.MachineSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.MachineSetSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: ptr.To("v1.27.0"),
							},
						},
					},
				},
				wantPass: false,
			},
			{
				name: "kubernetes version preflight check: should fail if the machine set minor version is 4 older than control plane minor version for >= v1.28",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToRef(controlPlaneStable128),
					},
				},
				controlPlane: controlPlaneStable128,
				machineSet: &clusterv1.MachineSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.MachineSetSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: ptr.To("v1.24.0"),
							},
						},
					},
				},
				wantPass: false,
			},
			{
				name: "kubernetes version preflight check: should fail if the machine set minor version is 3 older than control plane minor version for < v1.28",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToRef(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machineSet: &clusterv1.MachineSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.MachineSetSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: ptr.To("v1.23.0"),
							},
						},
					},
				},
				wantPass: false,
			},
			{
				name: "kubernetes version preflight check: should pass if the machine set minor version is greater than control plane minor version but the preflight check is skipped",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToRef(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machineSet: &clusterv1.MachineSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Annotations: map[string]string{
							clusterv1.MachineSetSkipPreflightChecksAnnotation: string(clusterv1.MachineSetPreflightCheckKubernetesVersionSkew),
						},
					},
					Spec: clusterv1.MachineSetSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: ptr.To("v1.27.0"),
							},
						},
					},
				},
				wantPass: true,
			},
			{
				name: "kubernetes version preflight check: should pass if the machine set minor version and control plane version conform to kubernetes version skew policy >= v1.28",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToRef(controlPlaneStable128),
					},
				},
				controlPlane: controlPlaneStable128,
				machineSet: &clusterv1.MachineSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.MachineSetSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: ptr.To("v1.25.0"),
							},
						},
					},
				},
				wantPass: true,
			},
			{
				name: "kubernetes version preflight check: should pass if the machine set minor version and control plane version conform to kubernetes version skew policy < v1.28",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToRef(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machineSet: &clusterv1.MachineSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.MachineSetSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: ptr.To("v1.24.0"),
							},
						},
					},
				},
				wantPass: true,
			},
			{
				name: "kubeadm version preflight check: should fail if the machine set version is not equal (major+minor) to control plane version when using kubeadm bootstrap provider",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToRef(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machineSet: &clusterv1.MachineSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.MachineSetSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: ptr.To("v1.25.5"),
								Bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{
									APIVersion: bootstrapv1.GroupVersion.String(),
									Kind:       "KubeadmConfigTemplate",
								}},
							},
						},
					},
				},
				wantPass: false,
			},
			{
				name: "kubeadm version preflight check: should pass if the machine set is not using kubeadm bootstrap provider",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToRef(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machineSet: &clusterv1.MachineSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.MachineSetSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: ptr.To("v1.25.0"),
							},
						},
					},
				},
				wantPass: true,
			},
			{
				name: "kubeadm version preflight check: should pass if the machine set version and control plane version do not conform to kubeadm version skew policy but the preflight check is skipped",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToRef(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machineSet: &clusterv1.MachineSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Annotations: map[string]string{
							clusterv1.MachineSetSkipPreflightChecksAnnotation: "foobar," + string(clusterv1.MachineSetPreflightCheckKubeadmVersionSkew),
						},
					},
					Spec: clusterv1.MachineSetSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: ptr.To("v1.25.0"),
								Bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{
									APIVersion: bootstrapv1.GroupVersion.String(),
									Kind:       "KubeadmConfigTemplate",
								}},
							},
						},
					},
				},
				wantPass: true,
			},
			{
				name: "kubeadm version preflight check: should pass if the machine set version and control plane version conform to kubeadm version skew when using kubeadm bootstrap provider",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToRef(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machineSet: &clusterv1.MachineSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.MachineSetSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: ptr.To("v1.26.2"),
								Bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{
									APIVersion: bootstrapv1.GroupVersion.String(),
									Kind:       "KubeadmConfigTemplate",
								}},
							},
						},
					},
				},
				wantPass: true,
			},
			{
				name: "kubeadm version preflight check: should error if the bootstrap ref APIVersion is invalid",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToRef(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machineSet: &clusterv1.MachineSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.MachineSetSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: ptr.To("v1.26.2"),
								Bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{
									APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1/invalid",
									Kind:       "KubeadmConfigTemplate",
								}},
							},
						},
					},
				},
				wantErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)
				objs := []client.Object{}
				if tt.controlPlane != nil {
					objs = append(objs, tt.controlPlane)
				}
				fakeClient := fake.NewClientBuilder().WithObjects(objs...).Build()
				r := &Reconciler{
					Client: fakeClient,
				}
				result, _, err := r.runPreflightChecks(ctx, tt.cluster, tt.machineSet, "")
				if tt.wantErr {
					g.Expect(err).To(HaveOccurred())
				} else {
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(result.IsZero()).To(Equal(tt.wantPass))
				}
			})
		}
	})

	t.Run("should not run the preflight checks if the feature gate is disabled", func(t *testing.T) {
		utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.MachineSetPreflightChecks, false)

		g := NewWithT(t)
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: contract.ObjToRef(controlPlaneUpgrading),
			},
		}
		controlPlane := controlPlaneUpgrading
		machineSet := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
			},
			Spec: clusterv1.MachineSetSpec{
				Template: clusterv1.MachineTemplateSpec{
					Spec: clusterv1.MachineSpec{
						Version: ptr.To("v1.26.0"),
						Bootstrap: clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{
							APIVersion: bootstrapv1.GroupVersion.String(),
							Kind:       "KubeadmConfigTemplate",
						}},
					},
				},
			},
		}
		fakeClient := fake.NewClientBuilder().WithObjects(controlPlane).Build()
		r := &Reconciler{Client: fakeClient}
		result, _, err := r.runPreflightChecks(ctx, cluster, machineSet, "")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.IsZero()).To(BeTrue())
	})
}
