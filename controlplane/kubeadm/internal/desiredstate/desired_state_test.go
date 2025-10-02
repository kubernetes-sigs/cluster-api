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

package desiredstate

import (
	"fmt"
	"testing"

	"github.com/blang/semver/v4"
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func Test_ComputeDesiredMachine(t *testing.T) {
	namingTemplateKey := "-kcp"
	kcpName := "testControlPlane"
	clusterName := "testCluster"

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: metav1.NamespaceDefault,
		},
	}
	duration5s := ptr.To(int32(5))
	duration10s := ptr.To(int32(10))
	kcpMachineTemplateObjectMeta := clusterv1.ObjectMeta{
		Labels: map[string]string{
			"machineTemplateLabel": "machineTemplateLabelValue",
		},
		Annotations: map[string]string{
			"machineTemplateAnnotation": "machineTemplateAnnotationValue",
		},
	}
	kcpMachineTemplateObjectMetaCopy := kcpMachineTemplateObjectMeta.DeepCopy()

	infraRef := &clusterv1.ContractVersionedObjectReference{
		Kind:     "InfraKind",
		APIGroup: clusterv1.GroupVersionInfrastructure.Group,
		Name:     "infra",
	}
	bootstrapRef := clusterv1.ContractVersionedObjectReference{
		Kind:     "BootstrapKind",
		APIGroup: clusterv1.GroupVersionBootstrap.Group,
		Name:     "bootstrap",
	}

	tests := []struct {
		name                      string
		kcp                       *controlplanev1.KubeadmControlPlane
		isUpdatingExistingMachine bool
		want                      []gomegatypes.GomegaMatcher
		wantErr                   bool
	}{
		{
			name: "should return the correct Machine object when creating a new Machine",
			kcp: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kcpName,
					Namespace: cluster.Namespace,
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.16.6",
					MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
						ObjectMeta: kcpMachineTemplateObjectMeta,
						Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
							ReadinessGates: []clusterv1.MachineReadinessGate{
								{
									ConditionType: "Foo",
								},
							},
							Deletion: controlplanev1.KubeadmControlPlaneMachineTemplateDeletionSpec{
								NodeDrainTimeoutSeconds:        duration5s,
								NodeDeletionTimeoutSeconds:     duration5s,
								NodeVolumeDetachTimeoutSeconds: duration5s,
							},
						},
					},
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: bootstrapv1.ClusterConfiguration{
							CertificatesDir: "foo",
						},
					},
					MachineNaming: controlplanev1.MachineNamingSpec{
						Template: "{{ .kubeadmControlPlane.name }}" + namingTemplateKey + "-{{ .random }}",
					},
				},
			},
			isUpdatingExistingMachine: false,
			want: []gomegatypes.GomegaMatcher{
				HavePrefix(kcpName + namingTemplateKey),
				Not(HaveSuffix("00000")),
			},
			wantErr: false,
		},
		{
			name: "should return error when creating a new Machine when '.random' is not added in template",
			kcp: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kcpName,
					Namespace: cluster.Namespace,
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.16.6",
					MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
						ObjectMeta: kcpMachineTemplateObjectMeta,
						Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
							Deletion: controlplanev1.KubeadmControlPlaneMachineTemplateDeletionSpec{
								NodeDrainTimeoutSeconds:        duration5s,
								NodeDeletionTimeoutSeconds:     duration5s,
								NodeVolumeDetachTimeoutSeconds: duration5s,
							},
						},
					},
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: bootstrapv1.ClusterConfiguration{
							CertificatesDir: "foo",
						},
					},
					MachineNaming: controlplanev1.MachineNamingSpec{
						Template: "{{ .kubeadmControlPlane.name }}" + namingTemplateKey,
					},
				},
			},
			isUpdatingExistingMachine: false,
			wantErr:                   true,
		},
		{
			name: "should not return error when creating a new Machine when the generated name exceeds 63",
			kcp: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kcpName,
					Namespace: cluster.Namespace,
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.16.6",
					MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
						ObjectMeta: kcpMachineTemplateObjectMeta,
						Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
							Deletion: controlplanev1.KubeadmControlPlaneMachineTemplateDeletionSpec{
								NodeDrainTimeoutSeconds:        duration5s,
								NodeDeletionTimeoutSeconds:     duration5s,
								NodeVolumeDetachTimeoutSeconds: duration5s,
							},
						},
					},
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: bootstrapv1.ClusterConfiguration{
							CertificatesDir: "foo",
						},
					},
					MachineNaming: controlplanev1.MachineNamingSpec{
						Template: "{{ .random }}" + fmt.Sprintf("%059d", 0),
					},
				},
			},
			isUpdatingExistingMachine: false,
			want: []gomegatypes.GomegaMatcher{
				ContainSubstring(fmt.Sprintf("%053d", 0)),
				Not(HaveSuffix("00000")),
			},
			wantErr: false,
		},
		{
			name: "should return error when creating a new Machine with invalid template",
			kcp: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kcpName,
					Namespace: cluster.Namespace,
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.16.6",
					MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
						ObjectMeta: kcpMachineTemplateObjectMeta,
						Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
							Deletion: controlplanev1.KubeadmControlPlaneMachineTemplateDeletionSpec{
								NodeDrainTimeoutSeconds:        duration5s,
								NodeDeletionTimeoutSeconds:     duration5s,
								NodeVolumeDetachTimeoutSeconds: duration5s,
							},
						},
					},
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: bootstrapv1.ClusterConfiguration{
							CertificatesDir: "foo",
						},
					},
					MachineNaming: controlplanev1.MachineNamingSpec{
						Template: "some-hardcoded-name-{{ .doesnotexistindata }}-{{ .random }}", // invalid template
					},
				},
			},
			isUpdatingExistingMachine: false,
			wantErr:                   true,
		},
		{
			name: "should return the correct Machine object when creating a new Machine with default templated name",
			kcp: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kcpName,
					Namespace: cluster.Namespace,
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.16.6",
					MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
						ObjectMeta: kcpMachineTemplateObjectMeta,
						Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
							Deletion: controlplanev1.KubeadmControlPlaneMachineTemplateDeletionSpec{
								NodeDrainTimeoutSeconds:        duration5s,
								NodeDeletionTimeoutSeconds:     duration5s,
								NodeVolumeDetachTimeoutSeconds: duration5s,
							},
						},
					},
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: bootstrapv1.ClusterConfiguration{
							CertificatesDir: "foo",
						},
					},
				},
			},
			isUpdatingExistingMachine: false,
			wantErr:                   false,
			want: []gomegatypes.GomegaMatcher{
				HavePrefix(kcpName),
				Not(HaveSuffix("00000")),
			},
		},
		{
			name: "should return the correct Machine object when creating a new Machine with additional kcp readinessGates",
			kcp: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kcpName,
					Namespace: cluster.Namespace,
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.16.6",
					MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
						ObjectMeta: kcpMachineTemplateObjectMeta,
						Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
							ReadinessGates: []clusterv1.MachineReadinessGate{
								{
									ConditionType: "Bar",
								},
							},
							Deletion: controlplanev1.KubeadmControlPlaneMachineTemplateDeletionSpec{
								NodeDrainTimeoutSeconds:        duration5s,
								NodeDeletionTimeoutSeconds:     duration5s,
								NodeVolumeDetachTimeoutSeconds: duration5s,
							},
						},
					},
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: bootstrapv1.ClusterConfiguration{
							CertificatesDir: "foo",
						},
					},
				},
			},
			isUpdatingExistingMachine: false,
			wantErr:                   false,
		},
		{
			name: "should return the correct Machine object when updating an existing Machine (empty ClusterConfiguration annotation)",
			kcp: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kcpName,
					Namespace: cluster.Namespace,
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.16.6",
					MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
						ObjectMeta: kcpMachineTemplateObjectMeta,
						Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
							Deletion: controlplanev1.KubeadmControlPlaneMachineTemplateDeletionSpec{
								NodeDrainTimeoutSeconds:        duration5s,
								NodeDeletionTimeoutSeconds:     duration5s,
								NodeVolumeDetachTimeoutSeconds: duration5s,
							},
							ReadinessGates: []clusterv1.MachineReadinessGate{
								{
									ConditionType: "Foo",
								},
							},
						},
					},
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: bootstrapv1.ClusterConfiguration{
							CertificatesDir: "foo",
						},
					},
					MachineNaming: controlplanev1.MachineNamingSpec{
						Template: "{{ .kubeadmControlPlane.name }}" + namingTemplateKey + "-{{ .random }}",
					},
				},
			},
			isUpdatingExistingMachine: true,
			wantErr:                   false,
		},
		{
			name: "should return the correct Machine object when updating an existing Machine (outdated ClusterConfiguration annotation)",
			kcp: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kcpName,
					Namespace: cluster.Namespace,
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.16.6",
					MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
						ObjectMeta: kcpMachineTemplateObjectMeta,
						Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
							Deletion: controlplanev1.KubeadmControlPlaneMachineTemplateDeletionSpec{
								NodeDrainTimeoutSeconds:        duration5s,
								NodeDeletionTimeoutSeconds:     duration5s,
								NodeVolumeDetachTimeoutSeconds: duration5s,
							},
							ReadinessGates: []clusterv1.MachineReadinessGate{
								{
									ConditionType: "Foo",
								},
							},
						},
					},
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: bootstrapv1.ClusterConfiguration{
							CertificatesDir: "foo",
						},
					},
					MachineNaming: controlplanev1.MachineNamingSpec{
						Template: "{{ .kubeadmControlPlane.name }}" + namingTemplateKey + "-{{ .random }}",
					},
				},
			},
			isUpdatingExistingMachine: true,
			wantErr:                   false,
		},
		{
			name: "should return the correct Machine object when updating an existing Machine (up to date ClusterConfiguration annotation)",
			kcp: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kcpName,
					Namespace: cluster.Namespace,
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.16.6",
					MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
						ObjectMeta: kcpMachineTemplateObjectMeta,
						Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
							Deletion: controlplanev1.KubeadmControlPlaneMachineTemplateDeletionSpec{
								NodeDrainTimeoutSeconds:        duration5s,
								NodeDeletionTimeoutSeconds:     duration5s,
								NodeVolumeDetachTimeoutSeconds: duration5s,
							},
							ReadinessGates: []clusterv1.MachineReadinessGate{
								{
									ConditionType: "Foo",
								},
							},
						},
					},
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: bootstrapv1.ClusterConfiguration{
							CertificatesDir: "foo",
						},
					},
					MachineNaming: controlplanev1.MachineNamingSpec{
						Template: "{{ .kubeadmControlPlane.name }}" + namingTemplateKey + "-{{ .random }}",
					},
				},
			},
			isUpdatingExistingMachine: true,
			wantErr:                   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var desiredMachine *clusterv1.Machine
			failureDomain := "fd-1"
			var expectedMachineSpec clusterv1.MachineSpec
			var err error

			if tt.isUpdatingExistingMachine {
				machineName := "existing-machine"
				machineUID := types.UID("abc-123-existing-machine")
				// Use different ClusterConfiguration string than the information present in KCP
				// to verify that for an existing machine we do not override this information.
				remediationData := "remediation-data"
				machineVersion := "v1.25.3"
				existingMachine := &clusterv1.Machine{
					ObjectMeta: metav1.ObjectMeta{
						Name: machineName,
						UID:  machineUID,
						Annotations: map[string]string{
							controlplanev1.RemediationForAnnotation: remediationData,
						},
					},
					Spec: clusterv1.MachineSpec{
						Version:       machineVersion,
						FailureDomain: failureDomain,
						Deletion: clusterv1.MachineDeletionSpec{
							NodeDrainTimeoutSeconds:        duration10s,
							NodeDeletionTimeoutSeconds:     duration10s,
							NodeVolumeDetachTimeoutSeconds: duration10s,
						},
						Bootstrap: clusterv1.Bootstrap{
							ConfigRef: bootstrapRef,
						},
						InfrastructureRef: *infraRef,
						ReadinessGates:    []clusterv1.MachineReadinessGate{{ConditionType: "Foo"}},
					},
				}

				desiredMachine, err = ComputeDesiredMachine(
					tt.kcp, cluster,
					existingMachine.Spec.FailureDomain, existingMachine,
				)
				if tt.wantErr {
					g.Expect(err).To(HaveOccurred())
					return
				}
				g.Expect(err).ToNot(HaveOccurred())
				expectedMachineSpec = clusterv1.MachineSpec{
					ClusterName: cluster.Name,
					Version:     machineVersion, // Should use the Machine version and not the version from KCP.
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: bootstrapRef,
					},
					InfrastructureRef: *infraRef,
					FailureDomain:     failureDomain,
					Deletion: clusterv1.MachineDeletionSpec{
						NodeDrainTimeoutSeconds:        tt.kcp.Spec.MachineTemplate.Spec.Deletion.NodeDrainTimeoutSeconds,
						NodeDeletionTimeoutSeconds:     tt.kcp.Spec.MachineTemplate.Spec.Deletion.NodeDeletionTimeoutSeconds,
						NodeVolumeDetachTimeoutSeconds: tt.kcp.Spec.MachineTemplate.Spec.Deletion.NodeVolumeDetachTimeoutSeconds,
					},
					ReadinessGates: append(append(mandatoryMachineReadinessGates, etcdMandatoryMachineReadinessGates...), tt.kcp.Spec.MachineTemplate.Spec.ReadinessGates...),
				}

				// Verify the Name and UID of the Machine remain unchanged
				g.Expect(desiredMachine.Name).To(Equal(machineName))
				g.Expect(desiredMachine.UID).To(Equal(machineUID))
				// Verify annotations.
				expectedAnnotations := map[string]string{}
				for k, v := range kcpMachineTemplateObjectMeta.Annotations {
					expectedAnnotations[k] = v
				}
				expectedAnnotations[controlplanev1.RemediationForAnnotation] = remediationData
				// The pre-terminate annotation should always be added
				expectedAnnotations[controlplanev1.PreTerminateHookCleanupAnnotation] = ""
				g.Expect(desiredMachine.Annotations).To(Equal(expectedAnnotations))
			} else {
				desiredMachine, err = ComputeDesiredMachine(
					tt.kcp, cluster,
					failureDomain, nil,
				)
				if tt.wantErr {
					g.Expect(err).To(HaveOccurred())
					return
				}
				g.Expect(err).ToNot(HaveOccurred())

				expectedMachineSpec = clusterv1.MachineSpec{
					ClusterName:   cluster.Name,
					Version:       tt.kcp.Spec.Version,
					FailureDomain: failureDomain,
					Deletion: clusterv1.MachineDeletionSpec{
						NodeDrainTimeoutSeconds:        tt.kcp.Spec.MachineTemplate.Spec.Deletion.NodeDrainTimeoutSeconds,
						NodeDeletionTimeoutSeconds:     tt.kcp.Spec.MachineTemplate.Spec.Deletion.NodeDeletionTimeoutSeconds,
						NodeVolumeDetachTimeoutSeconds: tt.kcp.Spec.MachineTemplate.Spec.Deletion.NodeVolumeDetachTimeoutSeconds,
					},
					ReadinessGates: append(append(mandatoryMachineReadinessGates, etcdMandatoryMachineReadinessGates...), tt.kcp.Spec.MachineTemplate.Spec.ReadinessGates...),
				}
				// Verify Name.
				for _, matcher := range tt.want {
					g.Expect(desiredMachine.Name).To(matcher)
				}
				// Verify annotations.
				expectedAnnotations := map[string]string{}
				for k, v := range kcpMachineTemplateObjectMeta.Annotations {
					expectedAnnotations[k] = v
				}
				// The pre-terminate annotation should always be added
				expectedAnnotations[controlplanev1.PreTerminateHookCleanupAnnotation] = ""
				g.Expect(desiredMachine.Annotations).To(Equal(expectedAnnotations))
			}

			g.Expect(desiredMachine.Namespace).To(Equal(tt.kcp.Namespace))
			g.Expect(desiredMachine.OwnerReferences).To(HaveLen(1))
			g.Expect(desiredMachine.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(tt.kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))))
			g.Expect(desiredMachine.Spec).To(BeComparableTo(expectedMachineSpec))

			// Verify that the machineTemplate.ObjectMeta has been propagated to the Machine.
			// Verify labels.
			expectedLabels := map[string]string{}
			for k, v := range kcpMachineTemplateObjectMeta.Labels {
				expectedLabels[k] = v
			}
			expectedLabels[clusterv1.ClusterNameLabel] = cluster.Name
			expectedLabels[clusterv1.MachineControlPlaneLabel] = ""
			expectedLabels[clusterv1.MachineControlPlaneNameLabel] = tt.kcp.Name
			g.Expect(desiredMachine.Labels).To(Equal(expectedLabels))

			// Verify that machineTemplate.ObjectMeta in KCP has not been modified.
			g.Expect(tt.kcp.Spec.MachineTemplate.ObjectMeta.Labels).To(Equal(kcpMachineTemplateObjectMetaCopy.Labels))
			g.Expect(tt.kcp.Spec.MachineTemplate.ObjectMeta.Annotations).To(Equal(kcpMachineTemplateObjectMetaCopy.Annotations))
		})
	}
}

func TestDefaultFeatureGates(t *testing.T) {
	tests := []struct {
		name                  string
		kubernetesVersion     semver.Version
		kubeadmConfigSpec     *bootstrapv1.KubeadmConfigSpec
		wantKubeadmConfigSpec *bootstrapv1.KubeadmConfigSpec
	}{
		{
			name:              "don't default ControlPlaneKubeletLocalMode for 1.30",
			kubernetesVersion: semver.MustParse("1.30.99"),
			kubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{
						"EtcdLearnerMode": true,
					},
				},
			},
			wantKubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{
						"EtcdLearnerMode": true,
					},
				},
			},
		},
		{
			name:              "default ControlPlaneKubeletLocalMode for 1.31",
			kubernetesVersion: semver.MustParse("1.31.0"),
			kubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{},
			},
			wantKubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{
						ControlPlaneKubeletLocalMode: true,
					},
				},
			},
		},
		{
			name:              "default ControlPlaneKubeletLocalMode for 1.31",
			kubernetesVersion: semver.MustParse("1.31.0"),
			kubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{
					FeatureGates: nil,
				},
			},
			wantKubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{
						ControlPlaneKubeletLocalMode: true,
					},
				},
			},
		},
		{
			name:              "default ControlPlaneKubeletLocalMode for 1.31",
			kubernetesVersion: semver.MustParse("1.31.0"),
			kubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{},
				},
			},
			wantKubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{
						ControlPlaneKubeletLocalMode: true,
					},
				},
			},
		},
		{
			name:              "default ControlPlaneKubeletLocalMode for 1.31",
			kubernetesVersion: semver.MustParse("1.31.0"),
			kubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{
						"EtcdLearnerMode": true,
					},
				},
			},
			wantKubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{
						ControlPlaneKubeletLocalMode: true,
						"EtcdLearnerMode":            true,
					},
				},
			},
		},
		{
			name:              "don't default ControlPlaneKubeletLocalMode for 1.31 if already set to false",
			kubernetesVersion: semver.MustParse("1.31.0"),
			kubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{
						ControlPlaneKubeletLocalMode: false,
					},
				},
			},
			wantKubeadmConfigSpec: &bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{
					FeatureGates: map[string]bool{
						ControlPlaneKubeletLocalMode: false,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			DefaultFeatureGates(tt.kubeadmConfigSpec, tt.kubernetesVersion)
			g.Expect(tt.wantKubeadmConfigSpec).Should(BeComparableTo(tt.kubeadmConfigSpec))
		})
	}
}
