/*
Copyright 2025 The Kubernetes Authors.

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

package machinepool

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/contract"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestMachinePoolReconciler_runPreflightChecks(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.MachinePoolPreflightChecks, true)
	ns := "ns1"

	controlPlaneWithNoVersion := builder.ControlPlane(ns, "cp1").Build()

	controlPlaneWithInvalidVersion := builder.ControlPlane(ns, "cp1").
		WithVersion("v1.25.6.0").Build()

	controlPlaneProvisioning := builder.ControlPlane(ns, "cp1").
		WithVersion("v1.25.6").Build()

	controlPlaneUpgrading := builder.ControlPlane(ns, "cp1").
		WithVersion("v1.26.2").
		WithStatusFields(map[string]any{
			"status.version": "v1.25.2",
		}).
		Build()

	controlPlaneStable := builder.ControlPlane(ns, "cp1").
		WithVersion("v1.26.2").
		WithStatusFields(map[string]any{
			"status.version": "v1.26.2",
		}).
		Build()

	controlPlaneStable128 := builder.ControlPlane(ns, "cp1").
		WithVersion("v1.28.0").
		WithStatusFields(map[string]any{
			"status.version": "v1.28.0",
		}).
		Build()

	t.Run("should run preflight checks if the feature gate is enabled", func(t *testing.T) {
		tests := []struct {
			name                  string
			cluster               *clusterv1.Cluster
			controlPlane          *unstructured.Unstructured
			machinePool           *clusterv1.MachinePool
			kubeadmConfigTemplate *bootstrapv1.KubeadmConfigTemplate
			wantMessages          []string
			wantErr               bool
		}{
			{
				name:         "should pass if cluster has no control plane",
				cluster:      &clusterv1.Cluster{},
				machinePool:  &clusterv1.MachinePool{},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "should pass if the control plane version is not defined",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneWithNoVersion),
					},
				},
				controlPlane: controlPlaneWithNoVersion,
				machinePool:  &clusterv1.MachinePool{},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "should error if the control plane version is invalid",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneWithInvalidVersion),
					},
				},
				controlPlane: controlPlaneWithInvalidVersion,
				machinePool:  &clusterv1.MachinePool{},
				wantMessages: nil,
				wantErr:      true,
			},
			{
				name: "should pass if all preflight checks are skipped",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneUpgrading),
					},
				},
				controlPlane: controlPlaneUpgrading,
				machinePool: &clusterv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Annotations: map[string]string{
							clusterv1.MachinePoolSkipPreflightChecksAnnotation: string(clusterv1.MachinePoolPreflightCheckAll),
						},
					},
				},
				kubeadmConfigTemplate: &bootstrapv1.KubeadmConfigTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      "kubeadmconfigtemplate-1",
						Annotations: map[string]string{
							// Note: Disabling this check is not enough, so this test verifies that this annotation is overwritten
							// by the one from the MachinePool.
							clusterv1.MachinePoolSkipPreflightChecksAnnotation: string(clusterv1.MachinePoolPreflightCheckKubeadmVersionSkew),
						},
					},
				},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "should pass if all preflight checks are skipped via KubeadmConfigTemplate",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneUpgrading),
					},
				},
				controlPlane: controlPlaneUpgrading,
				machinePool: &clusterv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.MachinePoolSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: "v1.26.2",
								Bootstrap: clusterv1.Bootstrap{
									ConfigRef: clusterv1.ContractVersionedObjectReference{
										APIGroup: bootstrapv1.GroupVersion.Group,
										Kind:     "KubeadmConfigTemplate",
										Name:     "kubeadmconfigtemplate-1",
									},
								},
							},
						},
					},
				},
				kubeadmConfigTemplate: &bootstrapv1.KubeadmConfigTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      "kubeadmconfigtemplate-1",
						Annotations: map[string]string{
							clusterv1.MachinePoolSkipPreflightChecksAnnotation: string(clusterv1.MachinePoolPreflightCheckAll),
						},
					},
				},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "control plane preflight check: should fail if the control plane is provisioning",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneProvisioning),
					},
				},
				controlPlane: controlPlaneProvisioning,
				machinePool:  &clusterv1.MachinePool{},
				wantMessages: []string{
					"GenericControlPlane ns1/cp1 is provisioning (\"ControlPlaneIsStable\" preflight check failed)",
				},
				wantErr: false,
			},
			{
				name: "control plane preflight check: should fail if the control plane is upgrading",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneUpgrading),
					},
				},
				controlPlane: controlPlaneUpgrading,
				machinePool:  &clusterv1.MachinePool{},
				wantMessages: []string{
					"GenericControlPlane ns1/cp1 is upgrading (\"ControlPlaneIsStable\" preflight check failed)",
				},
				wantErr: false,
			},
			{
				name: "control plane preflight check: should fail if the cluster defines a different version than the control plane",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
						Topology: clusterv1.Topology{
							Version: "v1.27.2",
						},
					},
				},
				controlPlane: controlPlaneStable,
				machinePool:  &clusterv1.MachinePool{},
				wantMessages: []string{
					"GenericControlPlane ns1/cp1 has a pending version upgrade to v1.27.2 (\"ControlPlaneIsStable\" preflight check failed)",
				},
				wantErr: false,
			},
			{
				name: "control plane preflight check: should fail if the cluster defines a different version than the control plane, and the control plane is not yet at the current step of the upgrade plan",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Annotations: map[string]string{
							clusterv1.ClusterTopologyUpgradeStepAnnotation: "v1.27.0",
						},
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
						Topology: clusterv1.Topology{
							Version: "v1.27.2",
						},
					},
				},
				controlPlane: controlPlaneStable,
				machinePool:  &clusterv1.MachinePool{},
				wantMessages: []string{
					"GenericControlPlane ns1/cp1 has a pending version upgrade to v1.27.0 (\"ControlPlaneIsStable\" preflight check failed)",
				},
				wantErr: false,
			},
			{
				name: "control plane preflight check: should pass if the control plane is upgrading but the preflight check is skipped",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneUpgrading),
					},
				},
				controlPlane: controlPlaneUpgrading,
				machinePool: &clusterv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Annotations: map[string]string{
							clusterv1.MachinePoolSkipPreflightChecksAnnotation: string(clusterv1.MachinePoolPreflightCheckControlPlaneIsStable),
						},
					},
					Spec: clusterv1.MachinePoolSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: "v1.26.2",
								Bootstrap: clusterv1.Bootstrap{
									ConfigRef: clusterv1.ContractVersionedObjectReference{
										APIGroup: bootstrapv1.GroupVersion.Group,
										Kind:     "KubeadmConfigTemplate",
										Name:     "kubeadmconfigtemplate-1",
									},
								},
							},
						},
					},
				},
				kubeadmConfigTemplate: &bootstrapv1.KubeadmConfigTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      "kubeadmconfigtemplate-1",
					},
				},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "control plane preflight check: should pass if the control plane is stable",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machinePool:  &clusterv1.MachinePool{},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "control plane preflight check: should pass if the control plane is stable, and the control plane is at the current step of the upgrade plan",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Annotations: map[string]string{
							clusterv1.ClusterTopologyUpgradeStepAnnotation: "v1.28.0",
						},
					},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable128),
						Topology: clusterv1.Topology{
							Version: "v1.27.2",
						},
					},
				},
				controlPlane: controlPlaneStable128,
				machinePool:  &clusterv1.MachinePool{},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "should pass if the machine pool version is not defined",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machinePool: &clusterv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec:       clusterv1.MachinePoolSpec{},
				},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "should error if the machine pool version is invalid",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machinePool: &clusterv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.MachinePoolSpec{
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
				name: "kubernetes version preflight check: should fail if the machine pool minor version is greater than control plane minor version",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machinePool: &clusterv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.MachinePoolSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: "v1.27.0",
							},
						},
					},
				},
				wantMessages: []string{
					"MachinePool version (1.27.0) and ControlPlane version (1.26.2) do not conform to the kubernetes version skew policy as MachinePool version is higher than ControlPlane version (\"KubernetesVersionSkew\" preflight check failed)",
				},
				wantErr: false,
			},
			{
				name: "kubernetes version preflight check: should fail if the machine pool minor version is 4 older than control plane minor version",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable128),
					},
				},
				controlPlane: controlPlaneStable128,
				machinePool: &clusterv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.MachinePoolSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: "v1.24.0",
							},
						},
					},
				},
				wantMessages: []string{
					"MachinePool version (1.24.0) and ControlPlane version (1.28.0) do not conform to the kubernetes version skew policy as MachinePool version is more than 3 minor versions older than the ControlPlane version (\"KubernetesVersionSkew\" preflight check failed)",
				},
				wantErr: false,
			},
			{
				name: "kubernetes version preflight check: should pass if the machine pool minor version is greater than control plane minor version but the preflight check is skipped",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machinePool: &clusterv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Annotations: map[string]string{
							clusterv1.MachinePoolSkipPreflightChecksAnnotation: string(clusterv1.MachinePoolPreflightCheckKubernetesVersionSkew),
						},
					},
					Spec: clusterv1.MachinePoolSpec{
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
				name: "kubernetes version preflight check: should pass if the machine pool minor version and control plane version conform to kubernetes version skew policy",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable128),
					},
				},
				controlPlane: controlPlaneStable128,
				machinePool: &clusterv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.MachinePoolSpec{
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
				name: "kubeadm version preflight check: should fail if the machine pool version is not equal (major+minor) to control plane version when using kubeadm bootstrap provider",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machinePool: &clusterv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.MachinePoolSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: "v1.25.5",
								Bootstrap: clusterv1.Bootstrap{
									ConfigRef: clusterv1.ContractVersionedObjectReference{
										APIGroup: bootstrapv1.GroupVersion.Group,
										Kind:     "KubeadmConfigTemplate",
										Name:     "kubeadmconfigtemplate-1",
									},
								},
							},
						},
					},
				},
				kubeadmConfigTemplate: &bootstrapv1.KubeadmConfigTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      "kubeadmconfigtemplate-1",
					},
				},
				wantMessages: []string{
					"MachinePool version (1.25.5) and ControlPlane version (1.26.2) do not conform to kubeadm version skew policy as kubeadm only supports joining with the same major+minor version as the control plane (\"KubeadmVersionSkew\" preflight check failed)",
				},
				wantErr: false,
			},
			{
				name: "kubeadm version preflight check: should pass if the machine pool is not using kubeadm bootstrap provider",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machinePool: &clusterv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.MachinePoolSpec{
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
				name: "kubeadm version preflight check: should pass if the machine pool version and control plane version do not conform to kubeadm version skew policy but the preflight check is skipped",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machinePool: &clusterv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Annotations: map[string]string{
							clusterv1.MachinePoolSkipPreflightChecksAnnotation: "foobar," + string(clusterv1.MachinePoolPreflightCheckKubeadmVersionSkew),
						},
					},
					Spec: clusterv1.MachinePoolSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: "v1.25.0",
								Bootstrap: clusterv1.Bootstrap{
									ConfigRef: clusterv1.ContractVersionedObjectReference{
										APIGroup: bootstrapv1.GroupVersion.Group,
										Kind:     "KubeadmConfigTemplate",
										Name:     "kubeadmconfigtemplate-1",
									},
								},
							},
						},
					},
				},
				kubeadmConfigTemplate: &bootstrapv1.KubeadmConfigTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      "kubeadmconfigtemplate-1",
					},
				},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "kubeadm version preflight check: should pass if the machine pool version and control plane version conform to kubeadm version skew when using kubeadm bootstrap provider",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machinePool: &clusterv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.MachinePoolSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: "v1.26.2",
								Bootstrap: clusterv1.Bootstrap{
									ConfigRef: clusterv1.ContractVersionedObjectReference{
										APIGroup: bootstrapv1.GroupVersion.Group,
										Kind:     "KubeadmConfigTemplate",
										Name:     "kubeadmconfigtemplate-1",
									},
								},
							},
						},
					},
				},
				kubeadmConfigTemplate: &bootstrapv1.KubeadmConfigTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      "kubeadmconfigtemplate-1",
					},
				},
				wantMessages: nil,
				wantErr:      false,
			},
			{
				name: "control plane version preflight check: should pass if the machine pool version and control plane version are not the same but the preflight check is skipped",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
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
				machinePool: &clusterv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Annotations: map[string]string{
							clusterv1.MachinePoolSkipPreflightChecksAnnotation: "foobar," + string(clusterv1.MachinePoolPreflightCheckControlPlaneVersionSkew) + "," + string(clusterv1.MachinePoolPreflightCheckControlPlaneIsStable),
						},
					},
					Spec: clusterv1.MachinePoolSpec{
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
				name: "control plane version preflight check: should pass if the machine pool version and control plane version are not the same but the Cluster does not have a managed topology",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
					Spec: clusterv1.ClusterSpec{
						// No Topology
						ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
					},
				},
				controlPlane: controlPlaneStable,
				machinePool: &clusterv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Annotations: map[string]string{
							clusterv1.MachinePoolSkipPreflightChecksAnnotation: string(clusterv1.MachinePoolPreflightCheckControlPlaneIsStable),
						},
					},
					Spec: clusterv1.MachinePoolSpec{
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
				name: "control plane version preflight check: should fail if the machine pool version and control plane version are not the same",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
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
				machinePool: &clusterv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Annotations: map[string]string{
							clusterv1.MachinePoolSkipPreflightChecksAnnotation: string(clusterv1.MachinePoolPreflightCheckControlPlaneIsStable),
						},
					},
					Spec: clusterv1.MachinePoolSpec{
						Template: clusterv1.MachineTemplateSpec{
							Spec: clusterv1.MachineSpec{
								Version: "v1.26.0",
							},
						},
					},
				},
				wantMessages: []string{"MachinePool version (v1.26.0) is not yet the same as the ControlPlane version (v1.26.2), waiting for version to be propagated to the MachinePool (\"ControlPlaneVersionSkew\" preflight check failed)"},
				wantErr:      false,
			},
			{
				name: "control plane version preflight check: should pass if the machine pool version and control plane version are the same",
				cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns},
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
				machinePool: &clusterv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Annotations: map[string]string{
							clusterv1.MachinePoolSkipPreflightChecksAnnotation: string(clusterv1.MachinePoolPreflightCheckControlPlaneIsStable),
						},
					},
					Spec: clusterv1.MachinePoolSpec{
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
				if tt.kubeadmConfigTemplate != nil {
					objs = append(objs, tt.kubeadmConfigTemplate, &apiextensionsv1.CustomResourceDefinition{
						ObjectMeta: metav1.ObjectMeta{
							Name: "kubeadmconfigtemplates.bootstrap.cluster.x-k8s.io",
							Labels: map[string]string{
								clusterv1.GroupVersion.String(): bootstrapv1.GroupVersion.Version,
							},
						},
					})
				}
				fakeClient := fake.NewClientBuilder().WithObjects(objs...).Build()
				r := &Reconciler{
					Client:          fakeClient,
					PreflightChecks: sets.Set[clusterv1.MachinePoolPreflightCheck]{}.Insert(clusterv1.MachinePoolPreflightCheckAll),
				}
				preflightCheckErrMessage, err := r.runPreflightChecks(ctx, tt.cluster, tt.machinePool, "")
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
		utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.MachinePoolPreflightChecks, false)

		g := NewWithT(t)
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneUpgrading),
			},
		}
		machinePool := &clusterv1.MachinePool{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns},
			Spec: clusterv1.MachinePoolSpec{
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
		fakeClient := fake.NewClientBuilder().WithObjects(controlPlaneUpgrading).Build()
		r := &Reconciler{
			Client:          fakeClient,
			PreflightChecks: sets.Set[clusterv1.MachinePoolPreflightCheck]{}.Insert(clusterv1.MachinePoolPreflightCheckAll),
		}
		messages, err := r.runPreflightChecks(ctx, cluster, machinePool, "")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(messages).To(BeNil())
	})
}

// TestMachinePoolReconciler_reconcilePreflightChecks verifies two behaviors:
//   - A check failure (e.g. control plane upgrading) is non-disruptive: it surfaces the PreflightCheckSucceeded
//     condition and an event, and requeues, matching the MachineSet behavior.
//   - An evaluation error (e.g. the control plane cannot be read) is a genuine controller error and is propagated so it
//     is logged and retried with backoff, matching the MachineSet behavior.
func TestMachinePoolReconciler_reconcilePreflightChecks(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.MachinePoolPreflightChecks, true)
	ns := "ns1"

	controlPlaneUpgrading := builder.ControlPlane(ns, "cp1").
		WithVersion("v1.26.2").
		WithStatusFields(map[string]any{
			"status.version": "v1.25.2",
		}).
		Build()

	controlPlaneStable := builder.ControlPlane(ns, "cp1").
		WithVersion("v1.26.2").
		WithStatusFields(map[string]any{
			"status.version": "v1.26.2",
		}).
		Build()

	t.Run("should surface a failing condition and event without blocking", func(t *testing.T) {
		g := NewWithT(t)
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneUpgrading),
			},
		}
		mp := &clusterv1.MachinePool{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "mp1"}}

		fakeClient := fake.NewClientBuilder().WithObjects(controlPlaneUpgrading, builder.GenericControlPlaneCRD).Build()
		recorder := record.NewFakeRecorder(10)
		r := &Reconciler{
			Client:          fakeClient,
			recorder:        recorder,
			PreflightChecks: sets.Set[clusterv1.MachinePoolPreflightCheck]{}.Insert(clusterv1.MachinePoolPreflightCheckAll),
		}

		res, err := r.reconcilePreflightChecks(ctx, &scope{cluster: cluster, machinePool: mp})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.RequeueAfter).To(Equal(preflightFailedRequeueAfter))

		cond := v1beta1conditions.Get(mp, clusterv1.PreflightCheckSucceededV1Beta1Condition)
		g.Expect(cond).ToNot(BeNil())
		g.Expect(cond.Status).To(Equal(corev1.ConditionFalse))
		g.Expect(cond.Reason).To(Equal(clusterv1.PreflightCheckFailedV1Beta1Reason))

		g.Expect(recorder.Events).To(Receive())
	})

	t.Run("should return an error when the checks cannot be evaluated", func(t *testing.T) {
		g := NewWithT(t)
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneUpgrading),
			},
		}
		mp := &clusterv1.MachinePool{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "mp1"}}

		// The control plane object is deliberately absent from the client, so resolving it fails.
		fakeClient := fake.NewClientBuilder().WithObjects(builder.GenericControlPlaneCRD).Build()
		recorder := record.NewFakeRecorder(10)
		r := &Reconciler{
			Client:          fakeClient,
			recorder:        recorder,
			PreflightChecks: sets.Set[clusterv1.MachinePoolPreflightCheck]{}.Insert(clusterv1.MachinePoolPreflightCheckAll),
		}

		res, err := r.reconcilePreflightChecks(ctx, &scope{cluster: cluster, machinePool: mp})
		g.Expect(err).To(HaveOccurred())
		g.Expect(res.IsZero()).To(BeTrue())

		g.Expect(v1beta1conditions.Get(mp, clusterv1.PreflightCheckSucceededV1Beta1Condition)).To(BeNil())

		g.Expect(recorder.Events).ToNot(Receive())
	})

	t.Run("should surface a passing condition when all checks pass", func(t *testing.T) {
		g := NewWithT(t)
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
			},
		}
		mp := &clusterv1.MachinePool{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "mp1"},
			Spec: clusterv1.MachinePoolSpec{
				Template: clusterv1.MachineTemplateSpec{
					Spec: clusterv1.MachineSpec{Version: "v1.26.2"},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithObjects(controlPlaneStable, builder.GenericControlPlaneCRD).Build()
		recorder := record.NewFakeRecorder(10)
		r := &Reconciler{
			Client:          fakeClient,
			recorder:        recorder,
			PreflightChecks: sets.Set[clusterv1.MachinePoolPreflightCheck]{}.Insert(clusterv1.MachinePoolPreflightCheckAll),
		}

		res, err := r.reconcilePreflightChecks(ctx, &scope{cluster: cluster, machinePool: mp})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeTrue())

		cond := v1beta1conditions.Get(mp, clusterv1.PreflightCheckSucceededV1Beta1Condition)
		g.Expect(cond).ToNot(BeNil())
		g.Expect(cond.Status).To(Equal(corev1.ConditionTrue))
	})

	t.Run("should not surface a condition if the feature gate is disabled", func(t *testing.T) {
		utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.MachinePoolPreflightChecks, false)

		g := NewWithT(t)
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneUpgrading),
			},
		}
		mp := &clusterv1.MachinePool{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "mp1"}}

		fakeClient := fake.NewClientBuilder().WithObjects(controlPlaneUpgrading, builder.GenericControlPlaneCRD).Build()
		recorder := record.NewFakeRecorder(10)
		r := &Reconciler{
			Client:          fakeClient,
			recorder:        recorder,
			PreflightChecks: sets.Set[clusterv1.MachinePoolPreflightCheck]{}.Insert(clusterv1.MachinePoolPreflightCheckAll),
		}

		res, err := r.reconcilePreflightChecks(ctx, &scope{cluster: cluster, machinePool: mp})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeTrue())
		g.Expect(v1beta1conditions.Get(mp, clusterv1.PreflightCheckSucceededV1Beta1Condition)).To(BeNil())
	})
}

func TestMachinePoolReconciler_shouldRun(t *testing.T) {
	tests := []struct {
		name                   string
		preflightChecks        sets.Set[clusterv1.MachinePoolPreflightCheck]
		skippedPreflightChecks sets.Set[clusterv1.MachinePoolPreflightCheck]
		preflightCheck         clusterv1.MachinePoolPreflightCheck
		expected               bool
	}{
		{
			name: "Should run all",
			preflightChecks: sets.Set[clusterv1.MachinePoolPreflightCheck]{}.Insert(
				clusterv1.MachinePoolPreflightCheckAll,
			),
			skippedPreflightChecks: nil,
			preflightCheck:         clusterv1.MachinePoolPreflightCheckControlPlaneIsStable,
			expected:               true,
		},
		{
			name: "Should run ControlPlaneIsStable",
			preflightChecks: sets.Set[clusterv1.MachinePoolPreflightCheck]{}.Insert(
				clusterv1.MachinePoolPreflightCheckControlPlaneIsStable,
			),
			skippedPreflightChecks: nil,
			preflightCheck:         clusterv1.MachinePoolPreflightCheckControlPlaneIsStable,
			expected:               true,
		},
		{
			name: "Should skip all when All is skipped",
			preflightChecks: sets.Set[clusterv1.MachinePoolPreflightCheck]{}.Insert(
				clusterv1.MachinePoolPreflightCheckAll,
			),
			skippedPreflightChecks: sets.Set[clusterv1.MachinePoolPreflightCheck]{}.Insert(
				clusterv1.MachinePoolPreflightCheckAll,
			),
			preflightCheck: clusterv1.MachinePoolPreflightCheckControlPlaneIsStable,
			expected:       false,
		},
		{
			name: "Should skip ControlPlaneIsStable when the specific check is skipped",
			preflightChecks: sets.Set[clusterv1.MachinePoolPreflightCheck]{}.Insert(
				clusterv1.MachinePoolPreflightCheckControlPlaneIsStable,
			),
			skippedPreflightChecks: sets.Set[clusterv1.MachinePoolPreflightCheck]{}.Insert(
				clusterv1.MachinePoolPreflightCheckControlPlaneIsStable,
			),
			preflightCheck: clusterv1.MachinePoolPreflightCheckControlPlaneIsStable,
			expected:       false,
		},
		{
			name: "Should skip ControlPlaneIsStable when All is enabled but the specific check is skipped",
			preflightChecks: sets.Set[clusterv1.MachinePoolPreflightCheck]{}.Insert(
				clusterv1.MachinePoolPreflightCheckAll,
			),
			skippedPreflightChecks: sets.Set[clusterv1.MachinePoolPreflightCheck]{}.Insert(
				clusterv1.MachinePoolPreflightCheckControlPlaneIsStable,
			),
			preflightCheck: clusterv1.MachinePoolPreflightCheckControlPlaneIsStable,
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
