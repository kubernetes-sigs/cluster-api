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
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
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
				Topology: builder.ClusterTopology().
					WithClass("cluster-class").
					Build(),
			},
			want: false,
		},
		{
			name: "should return true if MachineHealthCheck if defined in ClusterClass, not defined in cluster topology and enable is not set",
			blueprint: &ClusterBlueprint{
				ClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "cluster-class").
					WithControlPlaneInfrastructureMachineTemplate(&unstructured.Unstructured{}).
					WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckClass{}).
					Build(),
				Topology: builder.ClusterTopology().
					WithClass("cluster-class").
					Build(),
			},
			want: true,
		},
		{
			name: "should return false if MachineHealthCheck if defined in ClusterClass, not defined in cluster topology and enable is false",
			blueprint: &ClusterBlueprint{
				ClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "cluster-class").
					WithControlPlaneInfrastructureMachineTemplate(&unstructured.Unstructured{}).
					WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckClass{}).
					Build(),
				Topology: builder.ClusterTopology().
					WithClass("cluster-class").
					WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckTopology{
						Enable: ptr.To(false),
					}).
					Build(),
			},
			want: false,
		},
		{
			name: "should return true if MachineHealthCheck if defined in ClusterClass, not defined in cluster topology and enable is true",
			blueprint: &ClusterBlueprint{
				ClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "cluster-class").
					WithControlPlaneInfrastructureMachineTemplate(&unstructured.Unstructured{}).
					WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckClass{}).
					Build(),
				Topology: builder.ClusterTopology().
					WithClass("cluster-class").
					WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckTopology{
						Enable: ptr.To(true),
					}).
					Build(),
			},
			want: true,
		},
		{
			name: "should return true if MachineHealthCheck if defined in cluster topology, not defined in ClusterClass and enable is not set",
			blueprint: &ClusterBlueprint{
				ClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "cluster-class").
					WithControlPlaneInfrastructureMachineTemplate(&unstructured.Unstructured{}).
					Build(),
				Topology: builder.ClusterTopology().
					WithClass("cluster-class").
					WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckTopology{
						MachineHealthCheckClass: clusterv1.MachineHealthCheckClass{
							UnhealthyConditions: []clusterv1.UnhealthyCondition{
								{
									Type:    corev1.NodeReady,
									Status:  corev1.ConditionUnknown,
									Timeout: metav1.Duration{Duration: 5 * time.Minute},
								},
							},
						},
					}).
					Build(),
			},
			want: true,
		},
		{
			name: "should return false if MachineHealthCheck if defined in cluster topology, not defined in ClusterClass and enable is false",
			blueprint: &ClusterBlueprint{
				ClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "cluster-class").
					WithControlPlaneInfrastructureMachineTemplate(&unstructured.Unstructured{}).
					Build(),
				Topology: builder.ClusterTopology().
					WithClass("cluster-class").
					WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckTopology{
						Enable: ptr.To(false),
						MachineHealthCheckClass: clusterv1.MachineHealthCheckClass{
							UnhealthyConditions: []clusterv1.UnhealthyCondition{
								{
									Type:    corev1.NodeReady,
									Status:  corev1.ConditionUnknown,
									Timeout: metav1.Duration{Duration: 5 * time.Minute},
								},
							},
						},
					}).
					Build(),
			},
			want: false,
		},
		{
			name: "should return true if MachineHealthCheck if defined in cluster topology, not defined in ClusterClass and enable is true",
			blueprint: &ClusterBlueprint{
				ClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "cluster-class").
					WithControlPlaneInfrastructureMachineTemplate(&unstructured.Unstructured{}).
					Build(),
				Topology: builder.ClusterTopology().
					WithClass("cluster-class").
					WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckTopology{
						Enable: ptr.To(true),
						MachineHealthCheckClass: clusterv1.MachineHealthCheckClass{
							UnhealthyConditions: []clusterv1.UnhealthyCondition{
								{
									Type:    corev1.NodeReady,
									Status:  corev1.ConditionUnknown,
									Timeout: metav1.Duration{Duration: 5 * time.Minute},
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
	mhcInClusterClass := &clusterv1.MachineHealthCheckClass{
		UnhealthyConditions: []clusterv1.UnhealthyCondition{
			{
				Type:    corev1.NodeReady,
				Status:  corev1.ConditionFalse,
				Timeout: metav1.Duration{Duration: 10 * time.Minute},
			},
		},
	}

	percent50 := intstr.FromString("50%")
	mhcInClusterTopology := &clusterv1.MachineHealthCheckClass{
		UnhealthyConditions: []clusterv1.UnhealthyCondition{
			{
				Type:    corev1.NodeReady,
				Status:  corev1.ConditionFalse,
				Timeout: metav1.Duration{Duration: 20 * time.Minute},
			},
		},
		MaxUnhealthy: &percent50,
	}

	tests := []struct {
		name      string
		blueprint *ClusterBlueprint
		want      *clusterv1.MachineHealthCheckClass
	}{
		{
			name: "should return the MachineHealthCheck from cluster topology if defined - should take precedence over MachineHealthCheck in ClusterClass",
			blueprint: &ClusterBlueprint{
				Topology: builder.ClusterTopology().
					WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckTopology{
						MachineHealthCheckClass: *mhcInClusterTopology,
					}).
					Build(),
				ControlPlane: &ControlPlaneBlueprint{
					MachineHealthCheck: mhcInClusterClass,
				},
			},
			want: mhcInClusterTopology,
		},
		{
			name: "should return the MachineHealthCheck from ClusterClass if no MachineHealthCheck is defined in cluster topology",
			blueprint: &ClusterBlueprint{
				Topology: builder.ClusterTopology().
					WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckTopology{}).
					Build(),
				ControlPlane: &ControlPlaneBlueprint{
					MachineHealthCheck: mhcInClusterClass,
				},
			},
			want: mhcInClusterClass,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(tt.blueprint.ControlPlaneMachineHealthCheckClass()).To(BeComparableTo(tt.want))
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
						MachineHealthCheck: &clusterv1.MachineHealthCheckClass{},
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
						MachineHealthCheck: &clusterv1.MachineHealthCheckClass{},
					},
				},
			},
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Class: "worker-class",
				MachineHealthCheck: &clusterv1.MachineHealthCheckTopology{
					Enable: ptr.To(false),
				},
			},
			want: false,
		},
		{
			name: "should return true if MachineHealthCheck is defined in ClusterClass and enable is true",
			blueprint: &ClusterBlueprint{
				MachineDeployments: map[string]*MachineDeploymentBlueprint{
					"worker-class": {
						MachineHealthCheck: &clusterv1.MachineHealthCheckClass{},
					},
				},
			},
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Class: "worker-class",
				MachineHealthCheck: &clusterv1.MachineHealthCheckTopology{
					Enable: ptr.To(true),
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
				MachineHealthCheck: &clusterv1.MachineHealthCheckTopology{
					MachineHealthCheckClass: clusterv1.MachineHealthCheckClass{
						UnhealthyConditions: []clusterv1.UnhealthyCondition{
							{
								Type:    corev1.NodeReady,
								Status:  corev1.ConditionUnknown,
								Timeout: metav1.Duration{Duration: 5 * time.Minute},
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
				MachineHealthCheck: &clusterv1.MachineHealthCheckTopology{
					Enable: ptr.To(false),
					MachineHealthCheckClass: clusterv1.MachineHealthCheckClass{
						UnhealthyConditions: []clusterv1.UnhealthyCondition{
							{
								Type:    corev1.NodeReady,
								Status:  corev1.ConditionUnknown,
								Timeout: metav1.Duration{Duration: 5 * time.Minute},
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
				MachineHealthCheck: &clusterv1.MachineHealthCheckTopology{
					Enable: ptr.To(true),
					MachineHealthCheckClass: clusterv1.MachineHealthCheckClass{
						UnhealthyConditions: []clusterv1.UnhealthyCondition{
							{
								Type:    corev1.NodeReady,
								Status:  corev1.ConditionUnknown,
								Timeout: metav1.Duration{Duration: 5 * time.Minute},
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
	mhcInClusterClass := &clusterv1.MachineHealthCheckClass{
		UnhealthyConditions: []clusterv1.UnhealthyCondition{
			{
				Type:    corev1.NodeReady,
				Status:  corev1.ConditionFalse,
				Timeout: metav1.Duration{Duration: 10 * time.Minute},
			},
		},
	}

	percent50 := intstr.FromString("50%")
	mhcInClusterTopology := &clusterv1.MachineHealthCheckClass{
		UnhealthyConditions: []clusterv1.UnhealthyCondition{
			{
				Type:    corev1.NodeReady,
				Status:  corev1.ConditionFalse,
				Timeout: metav1.Duration{Duration: 20 * time.Minute},
			},
		},
		MaxUnhealthy: &percent50,
	}

	tests := []struct {
		name       string
		blueprint  *ClusterBlueprint
		mdTopology *clusterv1.MachineDeploymentTopology
		want       *clusterv1.MachineHealthCheckClass
	}{
		{
			name: "should return the MachineHealthCheck from cluster topology if defined - should take precedence over MachineHealthCheck in ClusterClass",
			blueprint: &ClusterBlueprint{
				MachineDeployments: map[string]*MachineDeploymentBlueprint{
					"worker-class": {
						MachineHealthCheck: mhcInClusterClass,
					},
				},
			},
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Class: "worker-class",
				MachineHealthCheck: &clusterv1.MachineHealthCheckTopology{
					MachineHealthCheckClass: *mhcInClusterTopology,
				},
			},
			want: mhcInClusterTopology,
		},
		{
			name: "should return the MachineHealthCheck from ClusterClass if no MachineHealthCheck is defined in cluster topology",
			blueprint: &ClusterBlueprint{
				MachineDeployments: map[string]*MachineDeploymentBlueprint{
					"worker-class": {
						MachineHealthCheck: mhcInClusterClass,
					},
				},
			},
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Class:              "worker-class",
				MachineHealthCheck: &clusterv1.MachineHealthCheckTopology{},
			},
			want: mhcInClusterClass,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(tt.blueprint.MachineDeploymentMachineHealthCheckClass(tt.mdTopology)).To(BeComparableTo(tt.want))
		})
	}
}
