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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestMachineSetReconciler_runPreflightChecks(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
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
			wantMessages []string
			wantErr      bool
		}{
			{
				name:         "should pass if cluster has no control plane",
				cluster:      &clusterv1.Cluster{},
				machineSet:   &clusterv1.MachineSet{},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "should pass if the control plane version is not defined",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneWithNoVersion),
					},
				},
				controlPlane: controlPlaneWithNoVersion,
				machineSet:   &clusterv1.MachineSet{},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "should error if the control plane version is invalid",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneWithInvalidVersion),
					},
				},
				controlPlane: controlPlaneWithInvalidVersion,
				machineSet:   &clusterv1.MachineSet{},
				wantMessages: nil,
				wantErr:      true,
			},
			{
				name: "should pass if all preflight checks are skipped",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneUpgrading),
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
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "control plane preflight check: should fail if the control plane is provisioning",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneProvisioning),
					},
				},
				controlPlane: controlPlaneProvisioning,
				machineSet:   &clusterv1.MachineSet{},
				wantMessages: []string{
					"GenericControlPlane ns1/cp1 is provisioning (\"ControlPlaneIsStable\" preflight check failed)",
				},
				wantErr: false,
			},
			{
				name: "control plane preflight check: should fail if the control plane is upgrading",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneUpgrading),
					},
				},
				controlPlane: controlPlaneUpgrading,
				machineSet:   &clusterv1.MachineSet{},
				wantMessages: []string{
					"GenericControlPlane ns1/cp1 is upgrading (\"ControlPlaneIsStable\" preflight check failed)",
				},
				wantErr: false,
			},
			{
				name: "control plane preflight check: should fail if the cluster defines a different version than the control plane",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
						Topology: clusterv1.Topology{
							Version: "v1.27.2",
						},
					},
				},
				controlPlane: controlPlaneStable,
				machineSet:   &clusterv1.MachineSet{},
				wantMessages: []string{
					"GenericControlPlane ns1/cp1 has a pending version upgrade to v1.27.2 (\"ControlPlaneIsStable\" preflight check failed)",
				},
				wantErr: false,
			},
			{
				name: "control plane preflight check: should pass if the control plane is upgrading but the preflight check is skipped",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneUpgrading),
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
								Version:   "v1.26.2",
								Bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{Kind: "KubeadmConfigTemplate"}},
							},
						},
					},
				},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "control plane preflight check: should pass if the control plane is stable",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machineSet:   &clusterv1.MachineSet{},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "should pass if the machine set version is not defined",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machineSet: &clusterv1.MachineSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.MachineSetSpec{},
				},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "should error if the machine set version is invalid",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
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
								Version: "v1.27.0.0",
							},
						},
					},
				},
				wantMessages: nil,
				wantErr:      true,
			},
			{
				name: "kubernetes version preflight check: should fail if the machine set minor version is greater than control plane minor version",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
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
								Version: "v1.27.0",
							},
						},
					},
				},
				wantMessages: []string{
					"MachineSet version (1.27.0) and ControlPlane version (1.26.2) do not conform to the kubernetes version skew policy as MachineSet version is higher than ControlPlane version (\"KubernetesVersionSkew\" preflight check failed)",
				},
				wantErr: false,
			},
			{
				name: "kubernetes version preflight check: should fail if the machine set minor version is 4 older than control plane minor version",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable128),
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
								Version: "v1.24.0",
							},
						},
					},
				},
				wantMessages: []string{
					"MachineSet version (1.24.0) and ControlPlane version (1.28.0) do not conform to the kubernetes version skew policy as MachineSet version is more than 3 minor versions older than the ControlPlane version (\"KubernetesVersionSkew\" preflight check failed)",
				},
				wantErr: false,
			},
			{
				name: "kubernetes version preflight check: should pass if the machine set minor version is greater than control plane minor version but the preflight check is skipped",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
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
								Version: "v1.27.0",
							},
						},
					},
				},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "kubernetes version preflight check: should pass if the machine set minor version and control plane version conform to kubernetes version skew policy",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable128),
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
								Version: "v1.25.0",
							},
						},
					},
				},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "kubeadm version preflight check: should fail if the machine set version is not equal (major+minor) to control plane version when using kubeadm bootstrap provider",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
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
								Version: "v1.25.5",
								Bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{
									APIGroup: bootstrapv1.GroupVersion.Group,
									Kind:     "KubeadmConfigTemplate",
								}},
							},
						},
					},
				},
				wantMessages: []string{
					"MachineSet version (1.25.5) and ControlPlane version (1.26.2) do not conform to kubeadm version skew policy as kubeadm only supports joining with the same major+minor version as the control plane (\"KubeadmVersionSkew\" preflight check failed)",
				},
				wantErr: false,
			},
			{
				name: "kubeadm version preflight check: should pass if the machine set is not using kubeadm bootstrap provider",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
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
								Version: "v1.25.0",
							},
						},
					},
				},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "kubeadm version preflight check: should pass if the machine set version and control plane version do not conform to kubeadm version skew policy but the preflight check is skipped",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
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
								Version: "v1.25.0",
								Bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{
									APIGroup: bootstrapv1.GroupVersion.Group,
									Kind:     "KubeadmConfigTemplate",
								}},
							},
						},
					},
				},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "kubeadm version preflight check: should pass if the machine set version and control plane version conform to kubeadm version skew when using kubeadm bootstrap provider",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
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
								Version: "v1.26.2",
								Bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{
									APIGroup: bootstrapv1.GroupVersion.Group,
									Kind:     "KubeadmConfigTemplate",
								}},
							},
						},
					},
				},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "control plane version preflight check: should pass if the machine set version and control plane version are not the same but the preflight check is skipped",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						Topology: clusterv1.Topology{
							ClassRef: clusterv1.ClusterClassRef{
								Name: "class",
							},
						},
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machineSet: &clusterv1.MachineSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Annotations: map[string]string{
							clusterv1.MachineSetSkipPreflightChecksAnnotation: "foobar," + string(clusterv1.MachineSetPreflightCheckControlPlaneVersionSkew) + "," + string(clusterv1.MachineSetPreflightCheckControlPlaneIsStable),
						},
					},
					Spec: clusterv1.MachineSetSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: "v1.26.0",
							},
						},
					},
				},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "control plane version preflight check: should pass if the machine set version and control plane version are not the same but the Cluster does not have a managed topology",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						// No Topology
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
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
								Version: "v1.26.0",
							},
						},
					},
				},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "control plane version preflight check: should fail if the machine set version and control plane version are not the same",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						Topology: clusterv1.Topology{
							ClassRef: clusterv1.ClusterClassRef{
								Name: "class",
							},
						},
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
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
								Version: "v1.26.0",
							},
						},
					},
				},
				wantMessages: []string{"MachineSet version (v1.26.0) is not yet the same as the ControlPlane version (v1.26.2), waiting for version to be propagated to the MachineSet (\"ControlPlaneVersionSkew\" preflight check failed)"},
				wantErr:      false,
			},
			{
				name: "control plane version preflight check: should pass if the machine set version and control plane version are the same",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
					},
					Spec: clusterv1.ClusterSpec{
						Topology: clusterv1.Topology{
							ClassRef: clusterv1.ClusterClassRef{
								Name: "class",
							},
						},
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
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
								Version: "v1.26.2",
							},
						},
					},
				},
				wantMessages: nil,
				wantErr:      false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)
				objs := []client.Object{}
				if tt.controlPlane != nil {
					objs = append(objs, tt.controlPlane, builder.GenericControlPlaneCRD)
				}
				fakeClient := fake.NewClientBuilder().WithObjects(objs...).Build()
				r := &Reconciler{
					Client:          fakeClient,
					PreflightChecks: sets.Set[clusterv1.MachineSetPreflightCheck]{}.Insert(clusterv1.MachineSetPreflightCheckAll),
				}
				preflightCheckErrMessage, err := r.runPreflightChecks(ctx, tt.cluster, tt.machineSet, "")
				if tt.wantErr {
					g.Expect(err).To(HaveOccurred())
				} else {
					g.Expect(err).ToNot(HaveOccurred())
				}
				g.Expect(preflightCheckErrMessage).To(BeComparableTo(tt.wantMessages))
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
				ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneUpgrading),
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
						Version: "v1.26.0",
						Bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: bootstrapv1.GroupVersion.Group,
							Kind:     "KubeadmConfigTemplate",
						}},
					},
				},
			},
		}
		fakeClient := fake.NewClientBuilder().WithObjects(controlPlane).Build()
		r := &Reconciler{Client: fakeClient}
		messages, err := r.runPreflightChecks(ctx, cluster, machineSet, "")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(messages).To(BeNil())
	})
}

func TestMachineSetReconciler_shouldRun(t *testing.T) {
	tests := []struct {
		name                   string
		preflightChecks        sets.Set[clusterv1.MachineSetPreflightCheck]
		skippedPreflightChecks sets.Set[clusterv1.MachineSetPreflightCheck]
		preflightCheck         clusterv1.MachineSetPreflightCheck
		expected               bool
	}{
		{
			name: "Should run all",
			preflightChecks: sets.Set[clusterv1.MachineSetPreflightCheck]{}.Insert(
				clusterv1.MachineSetPreflightCheckAll,
			),
			skippedPreflightChecks: nil,
			preflightCheck:         clusterv1.MachineSetPreflightCheckControlPlaneIsStable,
			expected:               true,
		},
		{
			name: "Should run ControlPlaneIsStable",
			preflightChecks: sets.Set[clusterv1.MachineSetPreflightCheck]{}.Insert(
				clusterv1.MachineSetPreflightCheckControlPlaneIsStable,
			),
			skippedPreflightChecks: nil,
			preflightCheck:         clusterv1.MachineSetPreflightCheckControlPlaneIsStable,
			expected:               true,
		},
		{
			name: "Should skip all",
			preflightChecks: sets.Set[clusterv1.MachineSetPreflightCheck]{}.Insert(
				clusterv1.MachineSetPreflightCheckAll,
			),
			skippedPreflightChecks: sets.Set[clusterv1.MachineSetPreflightCheck]{}.Insert(
				clusterv1.MachineSetPreflightCheckAll,
			),
			preflightCheck: clusterv1.MachineSetPreflightCheckControlPlaneIsStable,
			expected:       false,
		},
		{
			name: "Should skip all",
			preflightChecks: sets.Set[clusterv1.MachineSetPreflightCheck]{}.Insert(
				clusterv1.MachineSetPreflightCheckControlPlaneIsStable,
			),
			skippedPreflightChecks: sets.Set[clusterv1.MachineSetPreflightCheck]{}.Insert(
				clusterv1.MachineSetPreflightCheckAll,
			),
			preflightCheck: clusterv1.MachineSetPreflightCheckControlPlaneIsStable,
			expected:       false,
		},
		{
			name: "Should skip ControlPlaneIsStable",
			preflightChecks: sets.Set[clusterv1.MachineSetPreflightCheck]{}.Insert(
				clusterv1.MachineSetPreflightCheckControlPlaneIsStable,
			),
			skippedPreflightChecks: sets.Set[clusterv1.MachineSetPreflightCheck]{}.Insert(
				clusterv1.MachineSetPreflightCheckControlPlaneIsStable,
			),
			preflightCheck: clusterv1.MachineSetPreflightCheckControlPlaneIsStable,
			expected:       false,
		},
		{
			name: "Should skip ControlPlaneIsStable",
			preflightChecks: sets.Set[clusterv1.MachineSetPreflightCheck]{}.Insert(
				clusterv1.MachineSetPreflightCheckAll,
			),
			skippedPreflightChecks: sets.Set[clusterv1.MachineSetPreflightCheck]{}.Insert(
				clusterv1.MachineSetPreflightCheckControlPlaneIsStable,
			),
			preflightCheck: clusterv1.MachineSetPreflightCheckControlPlaneIsStable,
			expected:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			actual := shouldRun(tt.preflightChecks, tt.skippedPreflightChecks, tt.preflightCheck)
			g.Expect(actual).To(Equal(tt.expected))
		})
	}
}
