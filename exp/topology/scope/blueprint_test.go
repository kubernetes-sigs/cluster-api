/*
Copyright 2022 The Kubernetes Authors.

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

package scope

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestIsControlPlaneMachineHealthCheckEnabled(t *testing.T) {
	tests := []struct {
		name      string
		blueprint *ClusterBlueprint
		want      bool
	}{
		{
			name: "should return false if the control plane does not have infrastructure machine",
			blueprint: &ClusterBlueprint{
				ClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "cluster-class").
					Build(),
			},
			want: false,
		},
		{
			name: "should return false if no MachineHealthCheck is defined in ClusterClass or ClusterTopology",
			blueprint: &ClusterBlueprint{
				ClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "cluster-class").
					WithControlPlaneInfrastructureMachineTemplate(&unstructured.Unstructured{}).
					Build(),
				Topology: *builder.ClusterTopology().
					WithClass("cluster-class").
					Build(),
			},
			want: false,
		},
		{
			name: "should return true if MachineHealthCheck is defined in ClusterClass, not defined in cluster topology and enable is not set",
			blueprint: &ClusterBlueprint{
				ClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "cluster-class").
					WithControlPlaneInfrastructureMachineTemplate(builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpinframachinetemplate").Build()).
					WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneClassHealthCheck{
						Checks: clusterv1.ControlPlaneClassHealthCheckChecks{
							UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
								{
									Type:           corev1.NodeReady,
									Status:         corev1.ConditionUnknown,
									TimeoutSeconds: ptr.To(int32(5 * 60)),
								},
							},
						},
					}).
					Build(),
				Topology: *builder.ClusterTopology().
					WithClass("cluster-class").
					Build(),
			},
			want: true,
		},
		{
			name: "should return false if MachineHealthCheck is defined in ClusterClass, not defined in cluster topology and enable is false",
			blueprint: &ClusterBlueprint{
				ClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "cluster-class").
					WithControlPlaneInfrastructureMachineTemplate(&unstructured.Unstructured{}).
					WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneClassHealthCheck{
						Checks: clusterv1.ControlPlaneClassHealthCheckChecks{
							UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
								{
									Type:           corev1.NodeReady,
									Status:         corev1.ConditionUnknown,
									TimeoutSeconds: ptr.To(int32(5 * 60)),
								},
							},
						},
					}).
					Build(),
				Topology: *builder.ClusterTopology().
					WithClass("cluster-class").
					WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneTopologyHealthCheck{
						Enabled: ptr.To(false),
					}).
					Build(),
			},
			want: false,
		},
		{
			name: "should return true if MachineHealthCheck is defined in ClusterClass, not defined in cluster topology and enable is true",
			blueprint: &ClusterBlueprint{
				ClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "cluster-class").
					WithControlPlaneInfrastructureMachineTemplate(builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpinframachinetemplate").Build()).
					WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneClassHealthCheck{
						Checks: clusterv1.ControlPlaneClassHealthCheckChecks{
							UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
								{
									Type:           corev1.NodeReady,
									Status:         corev1.ConditionUnknown,
									TimeoutSeconds: ptr.To(int32(5 * 60)),
								},
							},
						},
					}).
					Build(),
				Topology: *builder.ClusterTopology().
					WithClass("cluster-class").
					WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneTopologyHealthCheck{
						Enabled: ptr.To(true),
					}).
					Build(),
			},
			want: true,
		},
		{
			name: "should return true if MachineHealthCheck is defined in cluster topology, not defined in ClusterClass and enable is not set",
			blueprint: &ClusterBlueprint{
				ClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "cluster-class").
					WithControlPlaneInfrastructureMachineTemplate(builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpinframachinetemplate").Build()).
					Build(),
				Topology: *builder.ClusterTopology().
					WithClass("cluster-class").
					WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneTopologyHealthCheck{
						Checks: clusterv1.ControlPlaneTopologyHealthCheckChecks{
							UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
								{
									Type:           corev1.NodeReady,
									Status:         corev1.ConditionUnknown,
									TimeoutSeconds: ptr.To(int32(5 * 60)),
								},
							},
						},
					}).
					Build(),
			},
			want: true,
		},
		{
			name: "should return false if MachineHealthCheck is defined in cluster topology, not defined in ClusterClass and enable is false",
			blueprint: &ClusterBlueprint{
				ClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "cluster-class").
					WithControlPlaneInfrastructureMachineTemplate(&unstructured.Unstructured{}).
					Build(),
				Topology: *builder.ClusterTopology().
					WithClass("cluster-class").
					WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneTopologyHealthCheck{
						Enabled: ptr.To(false),
						Checks: clusterv1.ControlPlaneTopologyHealthCheckChecks{
							UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
								{
									Type:           corev1.NodeReady,
									Status:         corev1.ConditionUnknown,
									TimeoutSeconds: ptr.To(int32(5 * 60)),
								},
							},
						},
					}).
					Build(),
			},
			want: false,
		},
		{
			name: "should return true if MachineHealthCheck is defined in cluster topology, not defined in ClusterClass and enable is true",
			blueprint: &ClusterBlueprint{
				ClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "cluster-class").
					WithControlPlaneInfrastructureMachineTemplate(builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpinframachinetemplate").Build()).
					Build(),
				Topology: *builder.ClusterTopology().
					WithClass("cluster-class").
					WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneTopologyHealthCheck{
						Enabled: ptr.To(true),
						Checks: clusterv1.ControlPlaneTopologyHealthCheckChecks{
							UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
								{
									Type:           corev1.NodeReady,
									Status:         corev1.ConditionUnknown,
									TimeoutSeconds: ptr.To(int32(5 * 60)),
								},
							},
						},
					}).
					Build(),
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(tt.blueprint.IsControlPlaneMachineHealthCheckEnabled()).To(Equal(tt.want))
		})
	}
}

func TestControlPlaneMachineHealthCheckClass(t *testing.T) {
	tests := []struct {
		name            string
		blueprint       *ClusterBlueprint
		wantChecks      clusterv1.MachineHealthCheckChecks
		wantRemediation clusterv1.MachineHealthCheckRemediation
	}{
		{
			name: "should return the MachineHealthCheck from cluster topology if defined - should take precedence over MachineHealthCheck in ClusterClass",
			blueprint: &ClusterBlueprint{
				Topology: *builder.ClusterTopology().
					WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneTopologyHealthCheck{
						Checks: clusterv1.ControlPlaneTopologyHealthCheckChecks{
							UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
								{
									Type:           corev1.NodeReady,
									Status:         corev1.ConditionFalse,
									TimeoutSeconds: ptr.To(int32(20 * 60)),
								},
							},
						},
						Remediation: clusterv1.ControlPlaneTopologyHealthCheckRemediation{
							TriggerIf: clusterv1.ControlPlaneTopologyHealthCheckRemediationTriggerIf{
								UnhealthyLessThanOrEqualTo: ptr.To(intstr.FromString("50%")),
							},
						},
					}).
					Build(),
				ControlPlane: &ControlPlaneBlueprint{
					HealthCheck: clusterv1.ControlPlaneClassHealthCheck{
						Checks: clusterv1.ControlPlaneClassHealthCheckChecks{
							UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
								{
									Type:           corev1.NodeReady,
									Status:         corev1.ConditionFalse,
									TimeoutSeconds: ptr.To(int32(10 * 60)),
								},
							},
						},
					},
				},
			},
			wantChecks: clusterv1.MachineHealthCheckChecks{
				UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
					{
						Type:           corev1.NodeReady,
						Status:         corev1.ConditionFalse,
						TimeoutSeconds: ptr.To(int32(20 * 60)),
					},
				},
			},
			wantRemediation: clusterv1.MachineHealthCheckRemediation{
				TriggerIf: clusterv1.MachineHealthCheckRemediationTriggerIf{
					UnhealthyLessThanOrEqualTo: ptr.To(intstr.FromString("50%")),
				},
			},
		},
		{
			name: "should return the MachineHealthCheck from ClusterClass if no MachineHealthCheck is defined in cluster topology",
			blueprint: &ClusterBlueprint{
				Topology: *builder.ClusterTopology().
					WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneTopologyHealthCheck{}).
					Build(),
				ControlPlane: &ControlPlaneBlueprint{
					HealthCheck: clusterv1.ControlPlaneClassHealthCheck{
						Checks: clusterv1.ControlPlaneClassHealthCheckChecks{
							UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
								{
									Type:           corev1.NodeReady,
									Status:         corev1.ConditionFalse,
									TimeoutSeconds: ptr.To(int32(10 * 60)),
								},
							},
						},
					},
				},
			},
			wantChecks: clusterv1.MachineHealthCheckChecks{
				UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
					{
						Type:           corev1.NodeReady,
						Status:         corev1.ConditionFalse,
						TimeoutSeconds: ptr.To(int32(10 * 60)),
					},
				},
			},
			wantRemediation: clusterv1.MachineHealthCheckRemediation{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			gotChecks, gotRemediation := tt.blueprint.ControlPlaneMachineHealthCheckClass()
			g.Expect(gotChecks).To(BeComparableTo(tt.wantChecks))
			g.Expect(gotRemediation).To(BeComparableTo(tt.wantRemediation))
		})
	}
}

func TestIsMachineDeploymentMachineHealthCheckEnabled(t *testing.T) {
	tests := []struct {
		name       string
		blueprint  *ClusterBlueprint
		mdTopology *clusterv1.MachineDeploymentTopology
		want       bool
	}{
		{
			name: "should return false if MachineHealthCheck is not defined in ClusterClass and cluster topology",
			blueprint: &ClusterBlueprint{
				MachineDeployments: map[string]*MachineDeploymentBlueprint{
					"worker-class": {},
				},
			},
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Class: "worker-class",
			},
			want: false,
		},
		{
			name: "should return true if MachineHealthCheck is defined in ClusterClass and enable is not set",
			blueprint: &ClusterBlueprint{
				MachineDeployments: map[string]*MachineDeploymentBlueprint{
					"worker-class": {
						HealthCheck: clusterv1.MachineDeploymentClassHealthCheck{
							Checks: clusterv1.MachineDeploymentClassHealthCheckChecks{
								UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
									{
										Type:           corev1.NodeReady,
										Status:         corev1.ConditionUnknown,
										TimeoutSeconds: ptr.To(int32(5 * 60)),
									},
								},
							},
						},
					},
				},
			},
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Class: "worker-class",
			},
			want: true,
		},
		{
			name: "should return false if MachineHealthCheck is defined in ClusterClass and enable is false",
			blueprint: &ClusterBlueprint{
				MachineDeployments: map[string]*MachineDeploymentBlueprint{
					"worker-class": {
						HealthCheck: clusterv1.MachineDeploymentClassHealthCheck{
							Checks: clusterv1.MachineDeploymentClassHealthCheckChecks{
								UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
									{
										Type:           corev1.NodeReady,
										Status:         corev1.ConditionUnknown,
										TimeoutSeconds: ptr.To(int32(5 * 60)),
									},
								},
							},
						},
					},
				},
			},
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Class: "worker-class",
				HealthCheck: clusterv1.MachineDeploymentTopologyHealthCheck{
					Enabled: ptr.To(false),
				},
			},
			want: false,
		},
		{
			name: "should return true if MachineHealthCheck is defined in ClusterClass and enable is true",
			blueprint: &ClusterBlueprint{
				MachineDeployments: map[string]*MachineDeploymentBlueprint{
					"worker-class": {
						HealthCheck: clusterv1.MachineDeploymentClassHealthCheck{
							Checks: clusterv1.MachineDeploymentClassHealthCheckChecks{
								UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
									{
										Type:           corev1.NodeReady,
										Status:         corev1.ConditionUnknown,
										TimeoutSeconds: ptr.To(int32(5 * 60)),
									},
								},
							},
						},
					},
				},
			},
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Class: "worker-class",
				HealthCheck: clusterv1.MachineDeploymentTopologyHealthCheck{
					Enabled: ptr.To(true),
				},
			},
			want: true,
		},
		{
			name: "should return true if MachineHealthCheck is defined in cluster topology and enable is not set",
			blueprint: &ClusterBlueprint{
				MachineDeployments: map[string]*MachineDeploymentBlueprint{
					"worker-class": {},
				},
			},
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Class: "worker-class",
				HealthCheck: clusterv1.MachineDeploymentTopologyHealthCheck{
					Checks: clusterv1.MachineDeploymentTopologyHealthCheckChecks{
						UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
							{
								Type:           corev1.NodeReady,
								Status:         corev1.ConditionUnknown,
								TimeoutSeconds: ptr.To(int32(5 * 60)),
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "should return false if MachineHealthCheck is defined in cluster topology and enable is false",
			blueprint: &ClusterBlueprint{
				MachineDeployments: map[string]*MachineDeploymentBlueprint{
					"worker-class": {},
				},
			},
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Class: "worker-class",
				HealthCheck: clusterv1.MachineDeploymentTopologyHealthCheck{
					Enabled: ptr.To(false),
					Checks: clusterv1.MachineDeploymentTopologyHealthCheckChecks{
						UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
							{
								Type:           corev1.NodeReady,
								Status:         corev1.ConditionUnknown,
								TimeoutSeconds: ptr.To(int32(5 * 60)),
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "should return true if MachineHealthCheck is defined in cluster topology and enable is true",
			blueprint: &ClusterBlueprint{
				MachineDeployments: map[string]*MachineDeploymentBlueprint{
					"worker-class": {},
				},
			},
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Class: "worker-class",
				HealthCheck: clusterv1.MachineDeploymentTopologyHealthCheck{
					Enabled: ptr.To(true),
					Checks: clusterv1.MachineDeploymentTopologyHealthCheckChecks{
						UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
							{
								Type:           corev1.NodeReady,
								Status:         corev1.ConditionUnknown,
								TimeoutSeconds: ptr.To(int32(5 * 60)),
							},
						},
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(tt.blueprint.IsMachineDeploymentMachineHealthCheckEnabled(tt.mdTopology)).To(BeComparableTo(tt.want))
		})
	}
}

func TestMachineDeploymentMachineHealthCheckClass(t *testing.T) {
	tests := []struct {
		name            string
		blueprint       *ClusterBlueprint
		mdTopology      *clusterv1.MachineDeploymentTopology
		wantChecks      clusterv1.MachineHealthCheckChecks
		wantRemediation clusterv1.MachineHealthCheckRemediation
	}{
		{
			name: "should return the MachineHealthCheck from cluster topology if defined - should take precedence over MachineHealthCheck in ClusterClass",
			blueprint: &ClusterBlueprint{
				MachineDeployments: map[string]*MachineDeploymentBlueprint{
					"worker-class": {
						HealthCheck: clusterv1.MachineDeploymentClassHealthCheck{
							Checks: clusterv1.MachineDeploymentClassHealthCheckChecks{
								UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
									{
										Type:           corev1.NodeReady,
										Status:         corev1.ConditionFalse,
										TimeoutSeconds: ptr.To(int32(10 * 60)),
									},
								},
							},
						},
					},
				},
			},
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Class: "worker-class",
				HealthCheck: clusterv1.MachineDeploymentTopologyHealthCheck{
					Checks: clusterv1.MachineDeploymentTopologyHealthCheckChecks{
						UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
							{
								Type:           corev1.NodeReady,
								Status:         corev1.ConditionFalse,
								TimeoutSeconds: ptr.To(int32(20 * 60)),
							},
						},
					},
					Remediation: clusterv1.MachineDeploymentTopologyHealthCheckRemediation{
						TriggerIf: clusterv1.MachineDeploymentTopologyHealthCheckRemediationTriggerIf{
							UnhealthyLessThanOrEqualTo: ptr.To(intstr.FromString("50%")),
						},
					},
				},
			},
			wantChecks: clusterv1.MachineHealthCheckChecks{
				UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
					{
						Type:           corev1.NodeReady,
						Status:         corev1.ConditionFalse,
						TimeoutSeconds: ptr.To(int32(20 * 60)),
					},
				},
			},
			wantRemediation: clusterv1.MachineHealthCheckRemediation{
				TriggerIf: clusterv1.MachineHealthCheckRemediationTriggerIf{
					UnhealthyLessThanOrEqualTo: ptr.To(intstr.FromString("50%")),
				},
			},
		},
		{
			name: "should return the MachineHealthCheck from ClusterClass if no MachineHealthCheck is defined in cluster topology",
			blueprint: &ClusterBlueprint{
				MachineDeployments: map[string]*MachineDeploymentBlueprint{
					"worker-class": {
						HealthCheck: clusterv1.MachineDeploymentClassHealthCheck{
							Checks: clusterv1.MachineDeploymentClassHealthCheckChecks{
								UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
									{
										Type:           corev1.NodeReady,
										Status:         corev1.ConditionFalse,
										TimeoutSeconds: ptr.To(int32(10 * 60)),
									},
								},
							},
						},
					},
				},
			},
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Class:       "worker-class",
				HealthCheck: clusterv1.MachineDeploymentTopologyHealthCheck{},
			},
			wantChecks: clusterv1.MachineHealthCheckChecks{
				UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
					{
						Type:           corev1.NodeReady,
						Status:         corev1.ConditionFalse,
						TimeoutSeconds: ptr.To(int32(10 * 60)),
					},
				},
			},
			wantRemediation: clusterv1.MachineHealthCheckRemediation{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			gotChecks, gotRemediation := tt.blueprint.MachineDeploymentMachineHealthCheckClass(tt.mdTopology)
			g.Expect(gotChecks).To(BeComparableTo(tt.wantChecks))
			g.Expect(gotRemediation).To(BeComparableTo(tt.wantRemediation))
		})
	}
}
