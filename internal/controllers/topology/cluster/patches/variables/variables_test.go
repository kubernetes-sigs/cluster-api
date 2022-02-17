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

package variables

import (
	"bytes"
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/pointer"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/test/builder"
)

func TestGlobal(t *testing.T) {
	tests := []struct {
		name            string
		clusterTopology *clusterv1.Topology
		cluster         *clusterv1.Cluster
		want            VariableMap
	}{
		{
			name: "Should calculate global variables",
			clusterTopology: &clusterv1.Topology{
				Variables: []clusterv1.ClusterVariable{
					{
						Name:  "location",
						Value: toJSON("\"us-central\""),
					},
					{
						Name:  "cpu",
						Value: toJSON("8"),
					},
					{
						// This is blocked by a webhook, but let's make sure that the user-defined
						// variable is overwritten by the builtin variable anyway.
						Name:  "builtin",
						Value: toJSON("8"),
					},
				},
			},
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster1",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{
						Class:   "clusterClass1",
						Version: "v1.21.1",
					},
					ClusterNetwork: &clusterv1.ClusterNetwork{
						Services: &clusterv1.NetworkRanges{
							CIDRBlocks: []string{"10.10.10.1/24"},
						},
						Pods: &clusterv1.NetworkRanges{
							CIDRBlocks: []string{"11.10.10.1/24"},
						},
						ServiceDomain: "cluster.local",
					},
				},
			},
			want: VariableMap{
				"location": toJSON("\"us-central\""),
				"cpu":      toJSON("8"),
				BuiltinsName: toJSONCompact(`{
					"cluster":{
						"name": "cluster1",
  						"namespace": "default",
 						 "topology":{
  						  	"version": "v1.21.1",
 						   	"class": "clusterClass1"
  						},
  						"network":{
							"serviceDomain":"cluster.local",
  						 	"services":["10.10.10.1/24"],
   							"pods":["11.10.10.1/24"],
    						"ipFamily": "IPv4"
						}}}`,
				),
			},
		},
		{
			name: "Should calculate when serviceDomain is not set",
			clusterTopology: &clusterv1.Topology{
				Variables: []clusterv1.ClusterVariable{
					{
						Name:  "location",
						Value: toJSON("\"us-central\""),
					},
					{
						Name:  "cpu",
						Value: toJSON("8"),
					},
					{
						// This is blocked by a webhook, but let's make sure that the user-defined
						// variable is overwritten by the builtin variable anyway.
						Name:  "builtin",
						Value: toJSON("8"),
					},
				},
			},
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster1",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{
						Class:   "clusterClass1",
						Version: "v1.21.1",
					},
					ClusterNetwork: &clusterv1.ClusterNetwork{
						Services: &clusterv1.NetworkRanges{
							CIDRBlocks: []string{"10.10.10.1/24"},
						},
						Pods: &clusterv1.NetworkRanges{
							CIDRBlocks: []string{"11.10.10.1/24"},
						},
					},
				},
			},
			want: VariableMap{
				"location": toJSON("\"us-central\""),
				"cpu":      toJSON("8"),
				BuiltinsName: toJSONCompact(`{
					"cluster":{
						"name": "cluster1",
  						"namespace": "default",
 						 "topology":{
  						  	"version": "v1.21.1",
 						   	"class": "clusterClass1"
  						},
  						"network":{
  						 	"services":["10.10.10.1/24"],
   							"pods":["11.10.10.1/24"],
    						"ipFamily": "IPv4"
						}}}`,
				),
			},
		},
		{
			name: "Should calculate where some variables  are nil",
			clusterTopology: &clusterv1.Topology{
				Variables: []clusterv1.ClusterVariable{
					{
						Name:  "location",
						Value: toJSON("\"us-central\""),
					},
					{
						Name:  "cpu",
						Value: toJSON("8"),
					},
					{
						// This is blocked by a webhook, but let's make sure that the user-defined
						// variable is overwritten by the builtin variable anyway.
						Name:  "builtin",
						Value: toJSON("8"),
					},
				},
			},
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster1",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{
						Class:   "clusterClass1",
						Version: "v1.21.1",
					},
					ClusterNetwork: &clusterv1.ClusterNetwork{
						Services:      nil,
						Pods:          &clusterv1.NetworkRanges{},
						ServiceDomain: "cluster.local",
					},
				},
			},
			want: VariableMap{
				"location": toJSON("\"us-central\""),
				"cpu":      toJSON("8"),
				BuiltinsName: toJSONCompact(`{
					"cluster":{
  						"name": "cluster1",
  						"namespace": "default",
 						"topology":{
    						"version": "v1.21.1",
    						"class": "clusterClass1"
  						},
  						"network":{
    						"serviceDomain":"cluster.local",
    						"ipFamily": "IPv4"
						}}}`),
			},
		},
		{
			name: "Should calculate where ClusterNetwork is nil",
			clusterTopology: &clusterv1.Topology{
				Variables: []clusterv1.ClusterVariable{
					{
						Name:  "location",
						Value: toJSON("\"us-central\""),
					},
					{
						Name:  "cpu",
						Value: toJSON("8"),
					},
					{
						// This is blocked by a webhook, but let's make sure that the user-defined
						// variable is overwritten by the builtin variable anyway.
						Name:  "builtin",
						Value: toJSON("8"),
					},
				},
			},
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster1",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{
						Class:   "clusterClass1",
						Version: "v1.21.1",
					},
					ClusterNetwork: nil,
				},
			},
			want: VariableMap{
				"location": toJSON("\"us-central\""),
				"cpu":      toJSON("8"),
				BuiltinsName: toJSONCompact(`{
					"cluster":{
  						"name": "cluster1",
  						"namespace": "default",
  						"topology":{
						"version": "v1.21.1",
   						 "class": "clusterClass1"
					}
					}}`),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := Global(tt.clusterTopology, tt.cluster)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestControlPlane(t *testing.T) {
	tests := []struct {
		name                                      string
		controlPlaneTopology                      *clusterv1.ControlPlaneTopology
		controlPlane                              *unstructured.Unstructured
		controlPlaneInfrastructureMachineTemplate *unstructured.Unstructured
		want                                      VariableMap
	}{
		{
			name: "Should calculate ControlPlane variables",
			controlPlaneTopology: &clusterv1.ControlPlaneTopology{
				Replicas: pointer.Int32(3),
			},
			controlPlane: builder.ControlPlane(metav1.NamespaceDefault, "controlPlane1").
				WithReplicas(3).
				WithVersion("v1.21.1").
				Build(),
			want: VariableMap{
				BuiltinsName: toJSONCompact(`{
					"controlPlane":{
						"version": "v1.21.1",
						"name":"controlPlane1",
						"replicas":3
					}}`),
			},
		},
		{
			name:                 "Should calculate ControlPlane variables, replicas not set",
			controlPlaneTopology: &clusterv1.ControlPlaneTopology{},
			controlPlane: builder.ControlPlane(metav1.NamespaceDefault, "controlPlane1").
				WithVersion("v1.21.1").
				Build(),
			want: VariableMap{
				BuiltinsName: toJSONCompact(`{
					"controlPlane":{
						"version": "v1.21.1",
						"name":"controlPlane1"
					}}`),
			},
		},
		{
			name: "Should calculate ControlPlane variables with InfrastructureMachineTemplate",
			controlPlaneTopology: &clusterv1.ControlPlaneTopology{
				Replicas: pointer.Int32(3),
			},
			controlPlane: builder.ControlPlane(metav1.NamespaceDefault, "controlPlane1").
				WithReplicas(3).
				WithVersion("v1.21.1").
				Build(),
			controlPlaneInfrastructureMachineTemplate: builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "controlPlaneInfrastructureMachineTemplate1").
				Build(),
			want: VariableMap{
				BuiltinsName: toJSONCompact(`{
					"controlPlane":{
						"version": "v1.21.1",
						"name":"controlPlane1",
						"replicas":3,
						"machineTemplate":{
							"infrastructureRef":{
								"name": "controlPlaneInfrastructureMachineTemplate1"
							}
						}
					}}`),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := ControlPlane(tt.controlPlaneTopology, tt.controlPlane, tt.controlPlaneInfrastructureMachineTemplate)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestMachineDeployment(t *testing.T) {
	tests := []struct {
		name                            string
		mdTopology                      *clusterv1.MachineDeploymentTopology
		md                              *clusterv1.MachineDeployment
		mdBootstrapTemplate             *unstructured.Unstructured
		mdInfrastructureMachineTemplate *unstructured.Unstructured
		want                            VariableMap
	}{
		{
			name: "Should calculate MachineDeployment variables",
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Replicas: pointer.Int32(3),
				Name:     "md-topology",
				Class:    "md-class",
				Variables: &clusterv1.MachineDeploymentVariables{
					Overrides: []clusterv1.ClusterVariable{
						{
							Name:  "location",
							Value: toJSON("\"us-central\""),
						},
						{
							Name:  "cpu",
							Value: toJSON("8"),
						},
					},
				},
			},
			md: builder.MachineDeployment(metav1.NamespaceDefault, "md1").
				WithReplicas(3).
				WithVersion("v1.21.1").
				Build(),
			want: VariableMap{
				"location": toJSON("\"us-central\""),
				"cpu":      toJSON("8"),
				BuiltinsName: toJSONCompact(`{
					"machineDeployment":{
						"version": "v1.21.1",
						"class": "md-class",
						"name": "md1",
						"topologyName": "md-topology",
						"replicas":3
					}}`),
			},
		},
		{
			name: "Should calculate MachineDeployment variables (without overrides)",
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Replicas: pointer.Int32(3),
				Name:     "md-topology",
				Class:    "md-class",
			},
			md: builder.MachineDeployment(metav1.NamespaceDefault, "md1").
				WithReplicas(3).
				WithVersion("v1.21.1").
				Build(),
			want: VariableMap{
				BuiltinsName: toJSONCompact(`{
					"machineDeployment":{
						"version": "v1.21.1",
						"class": "md-class",
						"name": "md1",
						"topologyName": "md-topology",
						"replicas":3
					}}`),
			},
		},
		{
			name: "Should calculate MachineDeployment variables, replicas not set",
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Name:  "md-topology",
				Class: "md-class",
				Variables: &clusterv1.MachineDeploymentVariables{
					Overrides: []clusterv1.ClusterVariable{
						{
							Name:  "location",
							Value: toJSON("\"us-central\""),
						},
						{
							Name:  "cpu",
							Value: toJSON("8"),
						},
					},
				},
			},
			md: builder.MachineDeployment(metav1.NamespaceDefault, "md1").
				WithVersion("v1.21.1").
				Build(),
			want: VariableMap{
				"location": toJSON("\"us-central\""),
				"cpu":      toJSON("8"),
				BuiltinsName: toJSONCompact(`{
					"machineDeployment":{
						"version": "v1.21.1",
						"class": "md-class",
						"name": "md1",
						"topologyName": "md-topology"
					}}`),
			},
		},
		{
			name: "Should calculate MachineDeployment variables with BoostrapTemplate",
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Replicas: pointer.Int32(3),
				Name:     "md-topology",
				Class:    "md-class",
				Variables: &clusterv1.MachineDeploymentVariables{
					Overrides: []clusterv1.ClusterVariable{
						{
							Name:  "location",
							Value: toJSON("\"us-central\""),
						},
						{
							Name:  "cpu",
							Value: toJSON("8"),
						},
					},
				},
			},
			md: builder.MachineDeployment(metav1.NamespaceDefault, "md1").
				WithReplicas(3).
				WithVersion("v1.21.1").
				Build(),
			mdBootstrapTemplate: builder.BootstrapTemplate(metav1.NamespaceDefault, "mdBT1").Build(),
			want: VariableMap{
				"location": toJSON("\"us-central\""),
				"cpu":      toJSON("8"),
				BuiltinsName: toJSONCompact(`{
					"machineDeployment":{
						"version": "v1.21.1",
						"class": "md-class",
						"name": "md1",
						"topologyName": "md-topology",
						"replicas":3,
						"bootstrap":{
							"configRef":{
								"name": "mdBT1"
							}
						}
					}}`),
			},
		},
		{
			name: "Should calculate MachineDeployment variables with InfrastructureMachineTemplate",
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Replicas: pointer.Int32(3),
				Name:     "md-topology",
				Class:    "md-class",
				Variables: &clusterv1.MachineDeploymentVariables{
					Overrides: []clusterv1.ClusterVariable{
						{
							Name:  "location",
							Value: toJSON("\"us-central\""),
						},
						{
							Name:  "cpu",
							Value: toJSON("8"),
						},
					},
				},
			},
			md: builder.MachineDeployment(metav1.NamespaceDefault, "md1").
				WithReplicas(3).
				WithVersion("v1.21.1").
				Build(),
			mdInfrastructureMachineTemplate: builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "mdIMT1").Build(),
			want: VariableMap{
				"location": toJSON("\"us-central\""),
				"cpu":      toJSON("8"),
				BuiltinsName: toJSONCompact(`{
					"machineDeployment":{
						"version": "v1.21.1",
						"class": "md-class",
						"name": "md1",
						"topologyName": "md-topology",
						"replicas":3,
						"infrastructureRef":{
							"name": "mdIMT1"
						}
					}}`),
			},
		},
		{
			name: "Should calculate MachineDeployment variables with BootstrapTemplate and InfrastructureMachineTemplate",
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Replicas: pointer.Int32(3),
				Name:     "md-topology",
				Class:    "md-class",
				Variables: &clusterv1.MachineDeploymentVariables{
					Overrides: []clusterv1.ClusterVariable{
						{
							Name:  "location",
							Value: toJSON("\"us-central\""),
						},
						{
							Name:  "cpu",
							Value: toJSON("8"),
						},
					},
				},
			},
			md: builder.MachineDeployment(metav1.NamespaceDefault, "md1").
				WithReplicas(3).
				WithVersion("v1.21.1").
				Build(),
			mdBootstrapTemplate:             builder.BootstrapTemplate(metav1.NamespaceDefault, "mdBT1").Build(),
			mdInfrastructureMachineTemplate: builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "mdIMT1").Build(),
			want: VariableMap{
				"location": toJSON("\"us-central\""),
				"cpu":      toJSON("8"),
				BuiltinsName: toJSONCompact(`{
					"machineDeployment":{
						"version": "v1.21.1",
						"class": "md-class",
						"name": "md1",
						"topologyName": "md-topology",
						"replicas":3,
						"bootstrap":{
							"configRef":{
								"name": "mdBT1"
							}
						},
						"infrastructureRef":{
							"name": "mdIMT1"
						}
					}}`),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := MachineDeployment(tt.mdTopology, tt.md, tt.mdBootstrapTemplate, tt.mdInfrastructureMachineTemplate)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func toJSON(value string) apiextensionsv1.JSON {
	return apiextensionsv1.JSON{Raw: []byte(value)}
}

func toJSONCompact(value string) apiextensionsv1.JSON {
	var compactValue bytes.Buffer
	if err := json.Compact(&compactValue, []byte(value)); err != nil {
		panic(err)
	}
	return apiextensionsv1.JSON{Raw: compactValue.Bytes()}
}
