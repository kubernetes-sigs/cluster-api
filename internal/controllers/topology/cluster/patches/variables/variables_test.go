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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestGlobal(t *testing.T) {
	clusterUID := "8a35f406-6b9b-4b78-8c93-a7f878d90623"
	tests := []struct {
		name                        string
		clusterTopology             *clusterv1.Topology
		cluster                     *clusterv1.Cluster
		variableDefinitionsForPatch map[string]bool
		want                        []runtimehooksv1.Variable
	}{
		{
			name:                        "Should calculate global variables",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
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
					UID:       types.UID(clusterUID),
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
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
					"cluster":{
						"name": "cluster1",
  						"namespace": "default",
						"uid": "8a35f406-6b9b-4b78-8c93-a7f878d90623",
 						 "topology":{
  						  	"version": "v1.21.1",
 						   	"class": "clusterClass1"
  						},
  						"network":{
							"serviceDomain":"cluster.local",
  						 	"services":["10.10.10.1/24"],
   							"pods":["11.10.10.1/24"],
    						"ipFamily": "IPv4"
						}
					}}`),
				},
			},
		},
		{
			name:                        "Should calculate global variables based on the variables defined for the patch",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
			clusterTopology: &clusterv1.Topology{
				Variables: []clusterv1.ClusterVariable{
					{
						Name:  "location",
						Value: toJSON("\"us-central\""),
					},
					{
						Name:  "https-proxy",
						Value: toJSON("\"internal.proxy.com\""),
						// This variable should be excluded because it is not among variableDefinitionsForPatch.
					},
					{
						Name:  "cpu",
						Value: toJSON("8"),
					},
				},
			},
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster1",
					Namespace: metav1.NamespaceDefault,
					UID:       types.UID(clusterUID),
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
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
					"cluster":{
						"name": "cluster1",
  						"namespace": "default",
						"uid": "8a35f406-6b9b-4b78-8c93-a7f878d90623",
 						 "topology":{
  						  	"version": "v1.21.1",
 						   	"class": "clusterClass1"
  						},
  						"network":{
							"serviceDomain":"cluster.local",
  						 	"services":["10.10.10.1/24"],
   							"pods":["11.10.10.1/24"],
    						"ipFamily": "IPv4"
						}
					}}`),
				},
			},
		},
		{
			name:                        "Should calculate when serviceDomain is not set",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
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
					UID:       types.UID(clusterUID),
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
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
					"cluster":{
						"name": "cluster1",
  						"namespace": "default",
						"uid": "8a35f406-6b9b-4b78-8c93-a7f878d90623",
 						 "topology":{
  						  	"version": "v1.21.1",
 						   	"class": "clusterClass1"
  						},
  						"network":{
  						 	"services":["10.10.10.1/24"],
   							"pods":["11.10.10.1/24"],
    						"ipFamily": "IPv4"
						}
					}}`),
				},
			},
		},
		{
			name:                        "Should calculate where some variables are nil",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
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
					UID:       types.UID(clusterUID),
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
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
					"cluster":{
  						"name": "cluster1",
  						"namespace": "default",
						"uid": "8a35f406-6b9b-4b78-8c93-a7f878d90623",
 						"topology":{
    						"version": "v1.21.1",
    						"class": "clusterClass1"
  						},
  						"network":{
    						"serviceDomain":"cluster.local",
    						"ipFamily": "IPv4"
						}
					}}`),
				},
			},
		},
		{
			name:                        "Should calculate where ClusterNetwork is nil",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
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
					UID:       types.UID(clusterUID),
				},
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{
						Class:   "clusterClass1",
						Version: "v1.21.1",
					},
					ClusterNetwork: nil,
				},
			},
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
					"cluster":{
  						"name": "cluster1",
  						"namespace": "default",
						"uid": "8a35f406-6b9b-4b78-8c93-a7f878d90623",
  						"topology":{
							"version": "v1.21.1",
   						 	"class": "clusterClass1"
						}
					}}`),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := Global(tt.clusterTopology, tt.cluster, tt.variableDefinitionsForPatch)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(BeComparableTo(tt.want))
		})
	}
}

func TestControlPlane(t *testing.T) {
	tests := []struct {
		name                                      string
		controlPlaneTopology                      *clusterv1.ControlPlaneTopology
		variableDefinitionsForPatch               map[string]bool
		controlPlane                              *unstructured.Unstructured
		controlPlaneInfrastructureMachineTemplate *unstructured.Unstructured
		want                                      []runtimehooksv1.Variable
	}{
		{
			name:                        "Should calculate ControlPlane variables",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
			controlPlaneTopology: &clusterv1.ControlPlaneTopology{
				Replicas: ptr.To[int32](3),
				Variables: &clusterv1.ControlPlaneVariables{
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
			controlPlane: builder.ControlPlane(metav1.NamespaceDefault, "controlPlane1").
				WithReplicas(3).
				WithVersion("v1.21.1").
				WithLabels(map[string]string{"foo": "bar"}).
				WithAnnotations(map[string]string{"fizz": "buzz"}).
				Build(),
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
					"controlPlane":{
						"version": "v1.21.1",
						"metadata": {"labels":{"foo":"bar"}, "annotations":{"fizz":"buzz"}},
						"name":"controlPlane1",
						"replicas":3
					}}`),
				},
			},
		},
		{
			name:                        "Should calculate ControlPlane variables based on the variables defined for the patch",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
			controlPlaneTopology: &clusterv1.ControlPlaneTopology{
				Replicas: ptr.To[int32](3),
				Variables: &clusterv1.ControlPlaneVariables{
					Overrides: []clusterv1.ClusterVariable{
						{
							Name:  "location",
							Value: toJSON("\"us-central\""),
						},
						{
							Name:  "http-proxy",
							Value: toJSON("\"internal.proxy.com\""),
							// This variable should be excluded because it is not in variableDefinitionsForPatch.
						},
						{
							Name:  "cpu",
							Value: toJSON("8"),
						},
					},
				},
			},
			controlPlane: builder.ControlPlane(metav1.NamespaceDefault, "controlPlane1").
				WithReplicas(3).
				WithVersion("v1.21.1").
				WithLabels(map[string]string{"foo": "bar"}).
				Build(),
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
					"controlPlane":{
						"version": "v1.21.1",
						"metadata": {"labels":{"foo":"bar"}},
						"name":"controlPlane1",
						"replicas":3
					}}`),
				},
			},
		},
		{
			name:                        "Should calculate ControlPlane variables (without overrides)",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
			controlPlaneTopology: &clusterv1.ControlPlaneTopology{
				Replicas: ptr.To[int32](3),
			},
			controlPlane: builder.ControlPlane(metav1.NamespaceDefault, "controlPlane1").
				WithReplicas(3).
				WithVersion("v1.21.1").
				Build(),
			want: []runtimehooksv1.Variable{
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
					"controlPlane":{
						"version": "v1.21.1",
						"name":"controlPlane1",
						"replicas":3
					}}`),
				},
			},
		},
		{
			name:                        "Should calculate ControlPlane variables, replicas not set",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
			controlPlaneTopology: &clusterv1.ControlPlaneTopology{
				Variables: &clusterv1.ControlPlaneVariables{
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
			controlPlane: builder.ControlPlane(metav1.NamespaceDefault, "controlPlane1").
				WithVersion("v1.21.1").
				Build(),
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
					"controlPlane":{
						"version": "v1.21.1",
						"name":"controlPlane1"
					}}`),
				},
			},
		},
		{
			name:                        "Should calculate ControlPlane variables with InfrastructureMachineTemplate",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
			controlPlaneTopology: &clusterv1.ControlPlaneTopology{
				Replicas: ptr.To[int32](3),
				Variables: &clusterv1.ControlPlaneVariables{
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
			controlPlane: builder.ControlPlane(metav1.NamespaceDefault, "controlPlane1").
				WithReplicas(3).
				WithVersion("v1.21.1").
				Build(),
			controlPlaneInfrastructureMachineTemplate: builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "controlPlaneInfrastructureMachineTemplate1").
				Build(),
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
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
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := ControlPlane(tt.controlPlaneTopology, tt.controlPlane, tt.controlPlaneInfrastructureMachineTemplate, tt.variableDefinitionsForPatch)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(BeComparableTo(tt.want))
		})
	}
}

func TestMachineDeployment(t *testing.T) {
	tests := []struct {
		name                            string
		mdTopology                      *clusterv1.MachineDeploymentTopology
		variableDefinitionsForPatch     map[string]bool
		md                              *clusterv1.MachineDeployment
		mdBootstrapTemplate             *unstructured.Unstructured
		mdInfrastructureMachineTemplate *unstructured.Unstructured
		want                            []runtimehooksv1.Variable
	}{
		{
			name:                        "Should calculate MachineDeployment variables",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Replicas: ptr.To[int32](3),
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
				WithLabels(map[string]string{"foo": "bar"}).
				WithAnnotations(map[string]string{"fizz": "buzz"}).
				Build(),
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
					"machineDeployment":{
						"version": "v1.21.1",
						"metadata": {"labels":{"foo":"bar"}, "annotations":{"fizz":"buzz"}},
						"class": "md-class",
						"name": "md1",
						"topologyName": "md-topology",
						"replicas":3
					}}`),
				},
			},
		},
		{
			name: "Should calculate MachineDeployment variables based on the variables defined for the patch",
			variableDefinitionsForPatch: map[string]bool{
				"location": true,
				"cpu":      true,
			},
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Replicas: ptr.To[int32](3),
				Name:     "md-topology",
				Class:    "md-class",
				Variables: &clusterv1.MachineDeploymentVariables{
					Overrides: []clusterv1.ClusterVariable{
						{
							Name:  "location",
							Value: toJSON("\"us-central\""),
						},
						{
							Name:  "https-proxy",
							Value: toJSON("\"internal.proxy.com\""),
							// This variable should be excluded because it is not in variableDefinitionsForPatch.
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
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
					"machineDeployment":{
						"version": "v1.21.1",
						"class": "md-class",
						"name": "md1",
						"topologyName": "md-topology",
						"replicas":3
					}}`),
				},
			},
		},
		{
			name:                        "Should calculate MachineDeployment variables (without overrides)",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Replicas: ptr.To[int32](3),
				Name:     "md-topology",
				Class:    "md-class",
			},
			md: builder.MachineDeployment(metav1.NamespaceDefault, "md1").
				WithReplicas(3).
				WithVersion("v1.21.1").
				Build(),
			want: []runtimehooksv1.Variable{
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
					"machineDeployment":{
						"version": "v1.21.1",
						"class": "md-class",
						"name": "md1",
						"topologyName": "md-topology",
						"replicas":3
					}}`),
				},
			},
		},
		{
			name:                        "Should calculate MachineDeployment variables, replicas not set",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
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
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
					"machineDeployment":{
						"version": "v1.21.1",
						"class": "md-class",
						"name": "md1",
						"topologyName": "md-topology"
					}}`),
				},
			},
		},
		{
			name:                        "Should calculate MachineDeployment variables with BoostrapTemplate",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Replicas: ptr.To[int32](3),
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
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
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
		},
		{
			name:                        "Should calculate MachineDeployment variables with InfrastructureMachineTemplate",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Replicas: ptr.To[int32](3),
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
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
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
		},
		{
			name:                        "Should calculate MachineDeployment variables with BootstrapTemplate and InfrastructureMachineTemplate",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Replicas: ptr.To[int32](3),
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
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
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
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := MachineDeployment(tt.mdTopology, tt.md, tt.mdBootstrapTemplate, tt.mdInfrastructureMachineTemplate, tt.variableDefinitionsForPatch)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(BeComparableTo(tt.want))
		})
	}
}

func TestMachinePool(t *testing.T) {
	tests := []struct {
		name                        string
		mpTopology                  *clusterv1.MachinePoolTopology
		variableDefinitionsForPatch map[string]bool
		mp                          *expv1.MachinePool
		mpBootstrapConfig           *unstructured.Unstructured
		mpInfrastructureMachinePool *unstructured.Unstructured
		want                        []runtimehooksv1.Variable
	}{
		{
			name:                        "Should calculate MachinePool variables",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
			mpTopology: &clusterv1.MachinePoolTopology{
				Replicas: ptr.To[int32](3),
				Name:     "mp-topology",
				Class:    "mp-class",
				Variables: &clusterv1.MachinePoolVariables{
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
			mp: builder.MachinePool(metav1.NamespaceDefault, "mp1").
				WithReplicas(3).
				WithVersion("v1.21.1").
				WithLabels(map[string]string{"foo": "bar"}).
				WithAnnotations(map[string]string{"fizz": "buzz"}).
				Build(),
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
					"machinePool":{
						"version": "v1.21.1",
						"metadata": {"labels":{"foo":"bar"}, "annotations":{"fizz":"buzz"}},
						"class": "mp-class",
						"name": "mp1",
						"topologyName": "mp-topology",
						"replicas":3
					}}`),
				},
			},
		},
		{
			name: "Should calculate MachinePool variables based on the variables defined for the patch",
			variableDefinitionsForPatch: map[string]bool{
				"location": true,
				"cpu":      true,
			},
			mpTopology: &clusterv1.MachinePoolTopology{
				Replicas: ptr.To[int32](3),
				Name:     "mp-topology",
				Class:    "mp-class",
				Variables: &clusterv1.MachinePoolVariables{
					Overrides: []clusterv1.ClusterVariable{
						{
							Name:  "location",
							Value: toJSON("\"us-central\""),
						},
						{
							Name:  "https-proxy",
							Value: toJSON("\"internal.proxy.com\""),
							// This variable should be excluded because it is not in variableDefinitionsForPatch.
						},
						{
							Name:  "cpu",
							Value: toJSON("8"),
						},
					},
				},
			},
			mp: builder.MachinePool(metav1.NamespaceDefault, "mp1").
				WithReplicas(3).
				WithVersion("v1.21.1").
				Build(),
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
					"machinePool":{
						"version": "v1.21.1",
						"class": "mp-class",
						"name": "mp1",
						"topologyName": "mp-topology",
						"replicas":3
					}}`),
				},
			},
		},
		{
			name:                        "Should calculate MachinePool variables (without overrides)",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
			mpTopology: &clusterv1.MachinePoolTopology{
				Replicas: ptr.To[int32](3),
				Name:     "mp-topology",
				Class:    "mp-class",
			},
			mp: builder.MachinePool(metav1.NamespaceDefault, "mp1").
				WithReplicas(3).
				WithVersion("v1.21.1").
				Build(),
			want: []runtimehooksv1.Variable{
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
					"machinePool":{
						"version": "v1.21.1",
						"class": "mp-class",
						"name": "mp1",
						"topologyName": "mp-topology",
						"replicas":3
					}}`),
				},
			},
		},
		{
			name:                        "Should calculate MachinePool variables, replicas not set",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
			mpTopology: &clusterv1.MachinePoolTopology{
				Name:  "mp-topology",
				Class: "mp-class",
				Variables: &clusterv1.MachinePoolVariables{
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
			mp: builder.MachinePool(metav1.NamespaceDefault, "mp1").
				WithVersion("v1.21.1").
				Build(),
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
					"machinePool":{
						"version": "v1.21.1",
						"class": "mp-class",
						"name": "mp1",
						"topologyName": "mp-topology"
					}}`),
				},
			},
		},
		{
			name:                        "Should calculate MachinePool variables with BoostrapConfig",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
			mpTopology: &clusterv1.MachinePoolTopology{
				Replicas: ptr.To[int32](3),
				Name:     "mp-topology",
				Class:    "mp-class",
				Variables: &clusterv1.MachinePoolVariables{
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
			mp: builder.MachinePool(metav1.NamespaceDefault, "mp1").
				WithReplicas(3).
				WithVersion("v1.21.1").
				Build(),
			mpBootstrapConfig: builder.BootstrapConfig(metav1.NamespaceDefault, "mpBC1").Build(),
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
					"machinePool":{
						"version": "v1.21.1",
						"class": "mp-class",
						"name": "mp1",
						"topologyName": "mp-topology",
						"replicas":3,
						"bootstrap":{
							"configRef":{
								"name": "mpBC1"
							}
						}
					}}`),
				},
			},
		},
		{
			name:                        "Should calculate MachinePool variables with InfrastructureMachinePool",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
			mpTopology: &clusterv1.MachinePoolTopology{
				Replicas: ptr.To[int32](3),
				Name:     "mp-topology",
				Class:    "mp-class",
				Variables: &clusterv1.MachinePoolVariables{
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
			mp: builder.MachinePool(metav1.NamespaceDefault, "mp1").
				WithReplicas(3).
				WithVersion("v1.21.1").
				Build(),
			mpInfrastructureMachinePool: builder.InfrastructureMachinePool(metav1.NamespaceDefault, "mpIMP1").Build(),
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
					"machinePool":{
						"version": "v1.21.1",
						"class": "mp-class",
						"name": "mp1",
						"topologyName": "mp-topology",
						"replicas":3,
						"infrastructureRef":{
							"name": "mpIMP1"
						}
					}}`),
				},
			},
		},
		{
			name:                        "Should calculate MachinePool variables with BootstrapConfig and InfrastructureMachinePool",
			variableDefinitionsForPatch: map[string]bool{"location": true, "cpu": true},
			mpTopology: &clusterv1.MachinePoolTopology{
				Replicas: ptr.To[int32](3),
				Name:     "mp-topology",
				Class:    "mp-class",
				Variables: &clusterv1.MachinePoolVariables{
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
			mp: builder.MachinePool(metav1.NamespaceDefault, "mp1").
				WithReplicas(3).
				WithVersion("v1.21.1").
				Build(),
			mpBootstrapConfig:           builder.BootstrapConfig(metav1.NamespaceDefault, "mpBC1").Build(),
			mpInfrastructureMachinePool: builder.InfrastructureMachinePool(metav1.NamespaceDefault, "mpIMP1").Build(),
			want: []runtimehooksv1.Variable{
				{
					Name:  "location",
					Value: toJSON("\"us-central\""),
				},
				{
					Name:  "cpu",
					Value: toJSON("8"),
				},
				{
					Name: runtimehooksv1.BuiltinsName,
					Value: toJSONCompact(`{
					"machinePool":{
						"version": "v1.21.1",
						"class": "mp-class",
						"name": "mp1",
						"topologyName": "mp-topology",
						"replicas":3,
						"bootstrap":{
							"configRef":{
								"name": "mpBC1"
							}
						},
						"infrastructureRef":{
							"name": "mpIMP1"
						}
					}}`),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := MachinePool(tt.mpTopology, tt.mp, tt.mpBootstrapConfig, tt.mpInfrastructureMachinePool, tt.variableDefinitionsForPatch)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(BeComparableTo(tt.want))
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
